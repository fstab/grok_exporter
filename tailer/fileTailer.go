package tailer

import (
	"bytes"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type fileTailer2 struct {
	lines   chan string
	errors  chan error
	done    chan bool
	watcher *fsnotify.Watcher
}

func (f *fileTailer2) Close() {
	f.done <- true
	f.watcher.Close()
	close(f.lines)
	close(f.errors)
}

func (f *fileTailer2) LineChan() chan string {
	return f.lines
}

func (f *fileTailer2) ErrorChan() chan error {
	return f.errors
}

func RunFileTailer2(path string, readall bool) (Tailer, error) {
	abspath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("%v: %v", path, err.Error())
	}
	watcher, err := newWatcher(abspath)
	if err != nil {
		return nil, fmt.Errorf("%v: %v", path, err.Error())
	}
	file, err := os.Open(abspath)
	if err != nil {
		watcher.Close()
		return nil, fmt.Errorf("%v: %v", path, err.Error())
	}
	if !readall {
		_, err := file.Seek(0, os.SEEK_END)
		if err != nil {
			watcher.Close()
			file.Close()
			return nil, fmt.Errorf("Error while seeking to the end of the end of file %v: %v", path, err.Error())
		}
	}
	linesChannel, doneChannel, errorChannel := runWatcher(watcher, abspath, file)
	return &fileTailer2{
		lines:   linesChannel,
		errors:  errorChannel,
		done:    doneChannel,
		watcher: watcher,
	}, nil
}

func newWatcher(abspath string) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize tail process: %v", err.Error())
	}
	dir := path.Dir(abspath)
	err = watcher.Add(dir)
	if err != nil {
		return nil, fmt.Errorf("Failed to watch files in %v: %v", dir, err.Error())
	}
	return watcher, nil
}

func runWatcher(watcher *fsnotify.Watcher, abspath string, file *os.File) (chan string, chan bool, chan error) {
	linesChannel := make(chan string) // TODO: Add capacity, so that we can handle a few fsnotify events in advance, while lines are still processed.
	doneChannel := make(chan bool)
	errorChannel := make(chan error)
	go func() {
		buf := make([]byte, 0) // Buffer for Bytes read from the logfile, but no newline yet, so we need to wait until we can send it to linesChannel.
		var err error
		for {
			if file != nil {
				buf, err = processAvailableLines(file, buf, linesChannel)
				if err != nil {
					errorChannel <- err
					return
				}
			}
			select {
			case event := <-watcher.Events:
				if isRelevant(event, abspath) {
					debug("processing event %v\n", event)
					err := processEvent(event, abspath, &file)
					if err != nil {
						errorChannel <- err
						return
					}
				} else {
					debug("ignoring event %v\n", event)
				}
			case err := <-watcher.Errors:
				errorChannel <- fmt.Errorf("Error while watching files in %v: %v", path.Dir(abspath), err.Error())
				return
			case <-doneChannel:
				debug("Shutting down file watcher loop.\n")
				return
			}
		}
	}()
	return linesChannel, doneChannel, errorChannel
}

func processAvailableLines(file *os.File, bytesFromLastRead []byte, linesChannel chan string) ([]byte, error) {
	availableBytes, err := read2EOF(file, bytesFromLastRead)
	if err != nil {
		return nil, err
	}
	remainingBytes, lines := stripLines(availableBytes)
	for _, line := range lines {
		linesChannel <- line
	}
	return remainingBytes, nil
}

func processEvent(event fsnotify.Event, abspath string, filep **os.File) error {
	var err error
	switch {
	case event.Op&fsnotify.Remove == fsnotify.Remove:
		err = (*filep).Close()
		if err != nil {
			return fmt.Errorf("Error while closing logfile after logrotate: %v", err.Error())
		}
		*filep = nil
	case event.Op&fsnotify.Create == fsnotify.Create:
		if *filep != nil {
			return fmt.Errorf("Error processing logrotate: New logfile was created, but old logfile was not removed. This is a bug.")
		}
		*filep, err = os.Open(abspath)
		if err != nil {
			return fmt.Errorf("Error while re-opening logfile after logrotate: %v", err.Error())
		}
	case event.Op&fsnotify.Chmod == fsnotify.Chmod:
		currentPos, err := (*filep).Seek(0, os.SEEK_CUR)
		if err != nil {
			return fmt.Errorf("Failed to get current read position in logfile: %v", err.Error())
		}
		fileInfo, err := (*filep).Stat()
		if err != nil {
			return fmt.Errorf("Failed to get file info for logfile: %v", err.Error())
		}
		if currentPos != fileInfo.Size() {
			// this happens if file is truncated, like with ':> /path/to/file'
			_, err := (*filep).Seek(0, os.SEEK_SET)
			if err != nil {
				return fmt.Errorf("Failed to seek to the beginning of the logfile: %v", err.Error())
			}
		}
	}
	return nil
}

func isRelevant(event fsnotify.Event, abspath string) bool {
	return strings.HasSuffix(event.Name, abspath)
}

func read2EOF(file *os.File, bytesFromLastRead []byte) ([]byte, error) {
	result := bytesFromLastRead
	buf := make([]byte, 512)
	for {
		n, err := file.Read(buf)
		if err != nil {
			if err == io.EOF {
				return result, nil
			} else {
				return nil, fmt.Errorf("Error reading from logfile: %v", err.Error())
			}
		}
		result = append(result, buf[0:n]...)
	}
}

func stripLines(data []byte) ([]byte, []string) {
	newline := []byte("\n")
	result := make([]string, 0)
	lines := bytes.SplitAfter(data, newline)
	for i, line := range lines {
		if bytes.HasSuffix(line, newline) {
			line = bytes.TrimSuffix(line, newline)
			line = bytes.TrimSuffix(line, []byte("\r")) // needed for Windows?
			result = append(result, string(line))
		} else {
			if i != len(lines)-1 {
				fmt.Fprintf(os.Stderr, "Unexpected error while splitting log data into lines. This is a bug.\n")
				os.Exit(-1)
			}
			return line, result
		}
	}
	return make([]byte, 0), result
}

func debug(format string, a ...interface{}) {
	fmt.Printf(format, a...)
}
