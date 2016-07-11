package tailer

import (
	"bytes"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"os"
	"path/filepath"
	"strings"
)

type fileTailer struct {
	lines  chan string
	errors chan error
	done   chan bool
}

func (f *fileTailer) Close() {
	// Will panic if Close() is called multiple times.
	f.done <- true
	close(f.done)
	close(f.lines)
	close(f.errors)
}

func (f *fileTailer) LineChan() chan string {
	return f.lines
}

func (f *fileTailer) ErrorChan() chan error {
	return f.errors
}

func RunFileTailer(path string, readall bool, log simpleLogger) Tailer {
	linesChannel := make(chan string)
	doneChannel := make(chan bool)
	errorChannel := make(chan error)
	go func() {
		abspath, err := filepath.Abs(path)
		if err != nil {
			errorChannel <- fmt.Errorf("%v: %v", path, err.Error())
			return
		}
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			errorChannel <- fmt.Errorf("Failed to initialize tail process: %v", err.Error())
			return
		}
		defer watcher.Close()
		dir := filepath.Dir(abspath)
		log.Debug("Watching file system notifications in '%v'.\n", dir)
		err = watcher.Add(dir)
		if err != nil {
			errorChannel <- fmt.Errorf("Failed to watch files in %v: %v", dir, err.Error())
			return
		}
		file, err := NewTailedFile(abspath)
		if err != nil {
			errorChannel <- err
			return
		}
		defer func() {
			if !file.IsClosed() {
				file.Close()
			}
		}()
		if !readall {
			err = file.SeekEnd()
			if err != nil {
				errorChannel <- err
				return
			}
		}
		buf := make([]byte, 0) // Buffer for Bytes read from the logfile, but no newline yet, so we need to wait until we can send it to linesChannel.
		bufferedEvents := runBufferingEventForwarder(watcher.Events)
		for {
			if file.IsOpen() {
				buf, err = processAvailableLines(file, buf, linesChannel)
				if err != nil {
					errorChannel <- err
					return
				}
			}
			select {
			case event := <-bufferedEvents:
				if isRelevant(event, abspath) {
					log.Debug("Processing file system event %v\n", event)
					err := processEvent(event, file)
					if err != nil {
						errorChannel <- err
						return
					}
				} else {
					log.Debug("Ignoring file system event %v\n", event)
				}
			case err := <-watcher.Errors:
				errorChannel <- fmt.Errorf("Error while watching files in %v: %v", dir, err.Error())
				return
			case <-doneChannel:
				log.Debug("Shutting down file system notification watcher.\n")
				return
			}
		}
	}()
	return &fileTailer{
		lines:  linesChannel,
		errors: errorChannel,
		done:   doneChannel,
	}
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
	case event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename:
		return file.Close()
	case event.Op&fsnotify.Create == fsnotify.Create:
		return file.Open()
	case event.Op&fsnotify.Chmod == fsnotify.Chmod || event.Op&fsnotify.Write == fsnotify.Write:
		// When the file is truncated on Linux, we get CHMOD.
		// On Windows we get no event directly, but check for truncation with each write.
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

// fsnotify.Event is an unbuffered, synchronous, blocking channel.
// If we take too long to read an event from it, subsequent events may be lost.
// We reduce this problem by reading continuously from fsnotify.Events
// and forwarding the events to a buffered channel.
// However, this only makes lost events less likely but does not really solve the problem.
func runBufferingEventForwarder(events chan fsnotify.Event) chan fsnotify.Event {
	result := make(chan fsnotify.Event, 100)
	go func() {
		for event := range events {
			result <- event
		}
		close(result)
	}()
	return result
}

type simpleLogger interface {
	Debug(format string, a ...interface{})
}
