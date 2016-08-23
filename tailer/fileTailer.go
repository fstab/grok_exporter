package tailer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type fileTailer struct {
	lines  chan string
	errors chan error
	done   chan struct{}
	closed bool
}

func (f *fileTailer) Close() {
	if !f.closed {
		f.closed = true
		close(f.done)
	}
}

func (f *fileTailer) Lines() chan string {
	return f.lines
}

func (f *fileTailer) Errors() chan error {
	return f.errors
}

func RunFileTailer(path string, readall bool, logger simpleLogger) Tailer {
	if logger == nil {
		logger = &nilLogger{}
	}
	lines := make(chan string)
	done := make(chan struct{})
	errors := make(chan error)
	go func() {
		defer func() {
			close(lines)
			close(errors)
		}()
		abspath, err := filepath.Abs(path)
		if err != nil {
			writeError(errors, done, "Failed to initialize file system watcher for %v: %v", path, err.Error())
			return
		}
		file, err := open(abspath)
		defer closeUnlessNil(file)
		if err != nil {
			writeError(errors, done, "Failed to initialize file system watcher for %v: %v", path, err.Error())
			return
		}
		if !readall {
			_, err = file.Seek(0, os.SEEK_END)
			if err != nil {
				writeError(errors, done, "Failed to initialize file system watcher for %v: %v", path, err.Error())
				return
			}
		}
		if err != nil {
			writeError(errors, done, "Failed to initialize file system watcher for %v: %v", path, err.Error())
			return
		}
		watcher, err := initWatcher(abspath, file)
		defer closeUnlessNil(watcher)
		if err != nil {
			writeError(errors, done, "Failed to initialize file system watcher for %v: %v", path, err.Error())
			return
		}
		reader := NewBufferedLineReader()
		freshLines, err := reader.ReadAvailableLines(file)
		if err != nil {
			writeError(errors, done, "Failed to initialize file system watcher for %v: %v", path, err.Error())
			return
		}
		for _, line := range freshLines {
			select {
			case <-done:
				return
			case lines <- line:
			}
		}

		eventLoop := startEventLoop(watcher)
		defer closeUnlessNil(eventLoop)

		for {
			select {
			case <-done:
				return
			case err = <-eventLoop.Errors():
				if err == nil {
					select {
					case <-done:
						// The tailer is shutting down and closed the 'done' and 'errors' channels. This is ok.
					default:
						// 'done' is still open, the tailer is not shutting down. This is a bug.
						writeError(errors, done, "failed to watch %v: unknown error", abspath)
					}
				} else {
					writeError(errors, done, "failed to watch %v: %v", abspath, err)
				}
				return
			case evnts := <-eventLoop.Events():
				var freshLines []string
				file, freshLines, err = processEvents(evnts, watcher, file, reader, abspath, logger)
				if err != nil {
					writeError(errors, done, "failed to watch %v: %v", abspath, err)
					return
				}
				for _, line := range freshLines {
					select {
					case <-done:
						return
					case lines <- line:
					}
				}
			}
		}
	}()
	return &fileTailer{
		lines:  lines,
		errors: errors,
		done:   done,
		closed: false,
	}
}

func closeUnlessNil(c io.Closer) {
	if c != nil {
		c.Close()
	}
}

func writeError(errors chan error, done chan struct{}, format string, a ...interface{}) {
	select {
	case errors <- fmt.Errorf(format, a...):
	case <-done:
	}
}

type simpleLogger interface {
	Debug(format string, a ...interface{})
}

type nilLogger struct{}

func (_ *nilLogger) Debug(_ string, _ ...interface{}) {}
