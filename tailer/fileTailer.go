// Copyright 2016-2018 The grok_exporter Authors
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
	"os"
	"path/filepath"
	"time"
)

type fileTailer struct {
	lines  chan string
	errors chan Error
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

func (f *fileTailer) Errors() chan Error {
	return f.errors
}

func RunFseventFileTailer(path string, readall bool, failOnMissingFile bool, logger simpleLogger) Tailer {
	return runFileTailer(path, readall, failOnMissingFile, logger, NewFseventWatcher)
}

func RunPollingFileTailer(path string, readall bool, failOnMissingFile bool, pollIntervall time.Duration, logger simpleLogger) Tailer {
	makeWatcher := func(abspath string, _ *File) (Watcher, error) {
		return NewPollingWatcher(abspath, pollIntervall)
	}
	return runFileTailer(path, readall, failOnMissingFile, logger, makeWatcher)
}

func runFileTailer(path string, readall bool, failOnMissingFile bool, logger simpleLogger, makeWatcher func(string, *File) (Watcher, error)) Tailer {
	if logger == nil {
		logger = &nilLogger{}
	}

	lines := make(chan string)
	done := make(chan struct{})
	errors := make(chan Error)

	result := &fileTailer{
		lines:  lines,
		errors: errors,
		done:   done,
		closed: false,
	}

	file, abspath, err := openLogfile(path, readall, failOnMissingFile) // file may be nil if failOnMissingFile is false and the file doesn't exist yet.
	if err != nil {
		go func(err error) {
			writeError(errors, done, err, "failed to initialize file system watcher for %v", path)
			close(lines)
			close(errors)
		}(err)
		return result
	}
	watcher, err := makeWatcher(abspath, file) // if file is nil the watcher assumes the file doesn't exist yet and waits for CREATE events.
	if err != nil {
		go func(err error) {
			writeError(errors, done, err, "failed to initialize file system watcher for %v", path)
			if file != nil {
				file.Close()
			}
			close(lines)
			close(errors)
		}(err)
		return result
	}

	// The watcher is initialized now. Fork off the event loop goroutine.
	go func() {
		defer func() {
			watcher.Close()
			if file != nil {
				file.Close()
			}
			close(lines)
			close(errors)
		}()
		eventLoop := watcher.StartEventLoop()
		if eventLoop != nil {
			defer eventLoop.Close()
		}
		reader := NewBufferedLineReader()
		if file != nil {
			// process all pre-existing lines
			freshLines, err := reader.ReadAvailableLines(file)
			if err != nil {
				writeError(errors, done, err, "failed to initialize file system watcher for %v", path)
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

		for {
			// process events from event loop
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
						writeError(errors, done, nil, "failed to watch %v", abspath)
					}
				} else {
					writeError(errors, done, err, "failed to watch %v", abspath)
				}
				return
			case evnts := <-eventLoop.Events():
				if evnts == nil {
					select {
					case <-done:
						// The tailer is shutting down and closed the 'done' and 'errors' channels. This is ok.
					default:
						// 'done' is still open, the tailer is not shutting down. This is a bug.
						writeError(errors, done, nil, "failed to watch %v", abspath)
					}
					return
				}
				var freshLines []string
				file, freshLines, err = evnts.Process(file, reader, abspath, logger)
				if err != nil {
					writeError(errors, done, err, "failed to watch %v", abspath)
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
	return result
}

// may return *File == nil if the file does not exist and failOnMissingFile == false
func openLogfile(path string, readall bool, failOnMissingFile bool) (*File, string, error) {
	abspath, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	file, err := open(abspath)
	if err != nil && (failOnMissingFile || !os.IsNotExist(err)) {
		return nil, "", err
	}
	if !readall && file != nil {
		_, err = file.Seek(0, io.SeekEnd)
		if err != nil {
			if file != nil {
				file.Close()
			}
			return nil, "", err
		}
	}
	return file, abspath, nil
}

func writeError(errors chan Error, done chan struct{}, cause error, format string, a ...interface{}) {
	select {
	case errors <- newError(fmt.Sprintf(format, a...), cause):
	case <-done:
	}
}

type simpleLogger interface {
	Debug(format string, a ...interface{})
}

type nilLogger struct{}

func (_ *nilLogger) Debug(_ string, _ ...interface{}) {}
