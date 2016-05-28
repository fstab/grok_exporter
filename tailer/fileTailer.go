package tailer

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// TODO: Read byte by byte to channel, then have other process to compose lines
// TODO: On CHMOD event (and any other non-WRITE event): Check if file size changed, if so, start reading from beginning of file.

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
	watcher, err := newWatcher(path)
	if err != nil {
		return nil, err
	}
	done := runWatcher(watcher, path)
	<-done
	return nil, nil
}

// See fsnotify's example_test.go
func newWatcher(logfile string) (*fsnotify.Watcher, error) {
	abspath, err := filepath.Abs(logfile)
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize tail process for file %v: %v", logfile, err.Error())
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize tail process for file %v: %v", logfile, err.Error())
	}
	err = watcher.Add(path.Dir(abspath))
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize tail process for file %v: %v", logfile, err.Error())
	}
	return watcher, nil
}

func runWatcher(watcher *fsnotify.Watcher, logfile string) chan bool {
	abspath, err := filepath.Abs(logfile)
	if err != nil {
		// This should not happen, as we successfully called Abs() in newWatcher()
		fmt.Fprintf(os.Stderr, "Failed to get absolute path for %v: %v\n", logfile, err.Error())
		os.Exit(-1)
	}
	done := make(chan bool)
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				fmt.Println("event:", event)
				if isRelevant(event, abspath) {
					fmt.Println("Event is relevant for the logfile.")
				} else {
					fmt.Println("Event is not relevant for the logfile.")
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					fmt.Println("modified file:", event.Name)
				}
			case err := <-watcher.Errors:
				fmt.Println("error:", err)
			case <-done:
				return
			}
		}
	}()
	return done
}

func isRelevant(event fsnotify.Event, logfile string) bool {
	return strings.HasSuffix(event.Name, logfile)
}
