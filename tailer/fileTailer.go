// Copyright 2016-2017 The grok_exporter Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tailer

import (
	"fmt"
	"io"
	"path/filepath"
	"time"
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

func RunFseventFileTailer(path string, readall bool, logger simpleLogger) Tailer {
	return runFileTailer(path, readall, logger, NewFseventWatcher)
}

func RunPollingFileTailer(path string, readall bool, pollIntervall time.Duration, logger simpleLogger) Tailer {
	makeWatcher := func(abspath string, _ *File) (Watcher, error) {
		return NewPollingWatcher(abspath, pollIntervall)
	}
	return runFileTailer(path, readall, logger, makeWatcher)
}

func runFileTailer(path string, readall bool, logger simpleLogger, makeWatcher func(abspath string, file *File) (Watcher, error)) Tailer {
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
			_, err = file.Seek(0, io.SeekEnd)
			if err != nil {
				writeError(errors, done, "Failed to initialize file system watcher for %v: %v", path, err.Error())
				return
			}
		}
		if err != nil {
			writeError(errors, done, "Failed to initialize file system watcher for %v: %v", path, err.Error())
			return
		}
		watcher, err := makeWatcher(abspath, file)
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

		eventLoop := watcher.StartEventLoop()
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
				file, freshLines, err = evnts.Process(file, reader, abspath, logger)
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
