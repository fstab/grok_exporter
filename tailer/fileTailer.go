package tailer

import (
	"bytes"
	"fmt"
	"github.com/fsnotify/fsnotify"
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
	file, err := NewTailedFile(abspath)
	if err != nil {
		watcher.Close()
		return nil, err
	}
	if !readall {
		err = file.SeekEnd()
		if err != nil {
			watcher.Close()
			file.Close()
			return nil, err
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
	dir := filepath.Dir(abspath)
	debug("Adding watcher for %v\n", dir)
	err = watcher.Add(dir)
	if err != nil {
		return nil, fmt.Errorf("Failed to watch files in %v: %v", dir, err.Error())
	}
	return watcher, nil
}

func runWatcher(watcher *fsnotify.Watcher, abspath string, file *tailedFile) (chan string, chan bool, chan error) {
	linesChannel := make(chan string) // TODO: Add capacity, so that we can handle a few fsnotify events in advance, while lines are still processed.
	doneChannel := make(chan bool)
	errorChannel := make(chan error)
	go func() {
		buf := make([]byte, 0) // Buffer for Bytes read from the logfile, but no newline yet, so we need to wait until we can send it to linesChannel.
		var err error
		for {
			if file.IsOpen() {
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
					err := processEvent(event, file)
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

func processAvailableLines(file *tailedFile, bytesFromLastRead []byte, linesChannel chan string) ([]byte, error) {
	newBytes, err := file.Read2EOF()
	if err != nil {
		return nil, err
	}
	remainingBytes, lines := stripLines(append(bytesFromLastRead, newBytes...))
	for _, line := range lines {
		linesChannel <- line
	}
	return remainingBytes, nil
}

func processEvent(event fsnotify.Event, file *tailedFile) error {
	switch {
	case event.Op&fsnotify.Remove == fsnotify.Remove:
		return file.Close()
	case event.Op&fsnotify.Create == fsnotify.Create:
		return file.Open()
	case event.Op&fsnotify.Chmod == fsnotify.Chmod:
		trunkated, err := file.IsTruncated()
		if err != nil {
			return err
		}
		if trunkated {
			return file.SeekStart()
		}
	}
	return nil
}

func isRelevant(event fsnotify.Event, abspath string) bool {
	return strings.HasSuffix(event.Name, abspath)
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
