package tailer

import (
	"fmt"
	"golang.org/x/exp/inotify"
	"os"
	"path/filepath"
)

type fileTailer struct {
	lines  chan string
	errors chan error
	done   chan bool
}

func (f *fileTailer) Close() {
	// TODO (1): If there was an error, this might hang forever, as the loop reading from 'done' has stopped.
	// TODO (2): Will panic if Close() is called multiple times, because writing to closed 'done' channel.
	f.done <- true
	close(f.done)
	close(f.lines)
	close(f.errors)
}

func (f *fileTailer) Lines() chan string {
	return f.lines
}

func (f *fileTailer) Errors() chan error {
	return f.errors
}

func RunFileTailer(path string, readall bool, logger simpleLogger) Tailer {
	linesChannel := make(chan string)
	doneChannel := make(chan bool)
	errorChannel := make(chan error)
	go func() {
		var (
			file    *os.File
			watcher *inotify.Watcher
		)
		abspath, err := filepath.Abs(path)
		if err != nil {
			closeAll(file, watcher)
			errorChannel <- fmt.Errorf("%v: %v", path, err.Error())
			return
		}

		file, err = os.Open(abspath)
		if err != nil {
			closeAll(file, watcher)
			errorChannel <- fmt.Errorf("%v: %v", abspath, err.Error())
			return
		}

		if !readall {
			_, err := file.Seek(0, os.SEEK_END)
			if err != nil {
				closeAll(file, watcher)
				errorChannel <- fmt.Errorf("%v: Error while seeking to the end of file: %v", path, err.Error())
				return
			}
		}

		reader := NewBufferedLineReader(file, linesChannel)

		watcher, err = inotify.NewWatcher()
		if err != nil {
			closeAll(file, watcher)
			errorChannel <- fmt.Errorf("Failed to create file system watcher: %v", err.Error())
			return
		}

		err = watcher.Watch(filepath.Dir(abspath))
		if err != nil {
			closeAll(file, watcher)
			errorChannel <- fmt.Errorf("Failed to watch directory %v: %v", filepath.Dir(abspath), err.Error())
			return
		}

		logger.Debug("Watching filesystem events in directory %v.\n", filepath.Dir(abspath))

		if readall {
			err = reader.ProcessAvailableLines()
			if err != nil {
				closeAll(file, watcher)
				errorChannel <- fmt.Errorf("Failed to watch %v: %v", abspath, err.Error())
				return
			}
		}

		for {
			select {
			case <-doneChannel:
				return
			case ev := <-watcher.Event:
				logger.Debug("Received filesystem event: %v\n", ev)

				// WRITE or TRUNCATE
				if file != nil && ev.Name == abspath && ev.Mask&inotify.IN_MODIFY == inotify.IN_MODIFY {
					truncated, err := checkTruncated(file)
					if err != nil {
						closeAll(file, watcher)
						errorChannel <- err
						return
					}
					if truncated {
						_, err := file.Seek(0, os.SEEK_SET)
						if err != nil {
							closeAll(file, watcher)
							errorChannel <- fmt.Errorf("Failed to seek to the beginning of file %v: %v", abspath, err.Error())
							return
						}
					}
					err = reader.ProcessAvailableLines()
					if err != nil {
						closeAll(file, watcher)
						errorChannel <- err
						return
					}
				}

				// MOVE or DELETE
				if file != nil && ev.Name == abspath && (ev.Mask&inotify.IN_MOVED_FROM == inotify.IN_MOVED_FROM || ev.Mask&inotify.IN_DELETE == inotify.IN_DELETE) {
					file.Close()
					file = nil
					reader = nil
				}

				// CREATE
				if file == nil && ev.Name == abspath && ev.Mask&inotify.IN_CREATE == inotify.IN_CREATE {
					file, err = os.Open(abspath)
					if err != nil {
						// Should not happen, because we just received the CREATE event for this file.
						closeAll(file, watcher)
						errorChannel <- fmt.Errorf("%v: Failed to open file: %v", abspath, err.Error())
						return
					}
					reader = NewBufferedLineReader(file, linesChannel)
					err = reader.ProcessAvailableLines()
					if err != nil {
						closeAll(file, watcher)
						errorChannel <- err
						return
					}
				}

			case err := <-watcher.Error:
				closeAll(file, watcher)
				errorChannel <- fmt.Errorf("Error while watching %v: %v\n", filepath.Dir(abspath), err.Error())
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

func checkTruncated(file *os.File) (bool, error) {
	currentPos, err := file.Seek(0, os.SEEK_CUR)
	if err != nil {
		return false, fmt.Errorf("%v: Seek() failed: %v", file.Name(), err.Error())
	}
	fileInfo, err := file.Stat()
	if err != nil {
		return false, fmt.Errorf("%v: Stat() failed: %v", file.Name(), err.Error())
	}
	return currentPos > fileInfo.Size(), nil
}

func closeAll(file *os.File, watcher *inotify.Watcher) {
	if watcher != nil {
		watcher.Close()
	}
	if file != nil {
		file.Close()
	}
}

type simpleLogger interface {
	Debug(format string, a ...interface{})
}
