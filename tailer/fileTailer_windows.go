package tailer

import (
	"fmt"
	"golang.org/x/exp/winfsnotify"
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
	// TODO (1): If there was an error, this might hang forever, as the loop reading from 'done' has stopped.
	// TODO (2): Will panic if Close() is called multiple times, because writing to closed 'done' channel.
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

func RunFileTailer(path string, readall bool, logger simpleLogger) Tailer {
	linesChannel := make(chan string)
	doneChannel := make(chan bool)
	errorChannel := make(chan error)
	go func() {
		abspath, err := filepath.Abs(path)
		if err != nil {
			errorChannel <- fmt.Errorf("%v: %v", path, err.Error())
			return
		}

		file, err := Open(abspath)
		if err != nil {
			errorChannel <- fmt.Errorf("%v: %v", abspath, err.Error())
			return
		}

		if !readall {
			_, err := file.Seek(0, os.SEEK_END)
			if err != nil {
				errorChannel <- fmt.Errorf("%v: Error while seeking to the end of file: %v", path, err.Error())
				return
			}
		}

		reader := NewBufferedLineReader(file, linesChannel)

		watcher, err := winfsnotify.NewWatcher()
		if err != nil {
			errorChannel <- fmt.Errorf("Failed to create file system watcher: %v", err.Error())
			return
		}

		err = watcher.Watch(filepath.Dir(abspath))
		if err != nil {
			watcher.Close()
			errorChannel <- fmt.Errorf("Failed to watch directory %v: %v", filepath.Dir(abspath), err.Error())
			return
		}

		logger.Debug("Watching filesystem events in directory %v.\n", filepath.Dir(abspath))

		if readall {
			err = reader.ProcessAvailableLines()
			if err != nil {
				watcher.Close()
				errorChannel <- fmt.Errorf("Failed to watch %v: %v", abspath, err.Error())
				return
			}
		}

		for {
			select {
			case <-doneChannel:
				watcher.Close()
				return
			case ev := <-watcher.Event:
				logger.Debug("Received filesystem event: %v\n", ev)

				// WRITE or TRUNCATE
				if file != nil && norm(ev.Name) == norm(abspath) && ev.Mask&winfsnotify.FS_MODIFY == winfsnotify.FS_MODIFY {
					truncated, err := checkTruncated(file)
					if err != nil {
						watcher.Close()
						errorChannel <- err
						return
					}
					if truncated {
						_, err := file.Seek(0, os.SEEK_SET)
						if err != nil {
							watcher.Close()
							errorChannel <- fmt.Errorf("Failed to seek to the beginning of file %v: %v", abspath, err.Error())
							return
						}
					}
					err = reader.ProcessAvailableLines()
					if err != nil {
						watcher.Close()
						errorChannel <- err
						return
					}
				}

				// MOVE or DELETE
				if file != nil && norm(ev.Name) == norm(abspath) && (ev.Mask&winfsnotify.FS_MOVED_FROM == winfsnotify.FS_MOVED_FROM || ev.Mask&winfsnotify.FS_DELETE == winfsnotify.FS_DELETE) {
					file = nil
					reader = nil
				}

				// CREATE
				if file == nil && norm(ev.Name) == norm(abspath) && ev.Mask&winfsnotify.FS_CREATE == winfsnotify.FS_CREATE {
					file, err = Open(abspath)
					if err != nil {
						// Should not happen, because we just received the CREATE event for this file.
						watcher.Close()
						errorChannel <- fmt.Errorf("%v: Failed to open file: %v", abspath, err.Error())
						return
					}
					reader = NewBufferedLineReader(file, linesChannel)
					err = reader.ProcessAvailableLines()
					if err != nil {
						watcher.Close()
						errorChannel <- err
						return
					}
				}
			case err := <-watcher.Error:
				watcher.Close()
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

// winfsnotify uses "/" instead of "\" when constructing the path in the event name.
func norm(path string) string {
	path = strings.Replace(path, "/", "\\", -1)
	path = strings.Replace(path, "\\\\", "\\", -1)
	return path
}

// On Windows, logrotate will not be able to delete or move the logfile if grok_exporter keeps the file open.
// The AutoClosingFile has an API similar to os.File, but the underlying file is closed after each operation.
type autoClosingFile struct {
	path       string
	currentPos int64
}

func Open(path string) (*autoClosingFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return &autoClosingFile{
		path:       path,
		currentPos: 0,
	}, nil
}

func (f *autoClosingFile) Seek(offset int64, whence int) (int64, error) {
	file, err := os.Open(f.path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	result, resultErr := file.Seek(offset, whence)
	f.currentPos, err = file.Seek(0, os.SEEK_CUR)
	if err != nil {
		return 0, err
	}
	return result, resultErr
}

func (f *autoClosingFile) Read(b []byte) (int, error) {
	file, err := os.Open(f.path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	_, err = file.Seek(f.currentPos, os.SEEK_SET)
	if err != nil {
		return 0, err
	}
	result, resultErr := file.Read(b)
	f.currentPos, err = file.Seek(0, os.SEEK_CUR)
	if err != nil {
		return 0, err
	}
	return result, resultErr
}

func (f *autoClosingFile) Name() string {
	return f.path
}

func checkTruncated(f *autoClosingFile) (bool, error) {
	file, err := os.Open(f.path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	fileInfo, err := file.Stat()
	if err != nil {
		return false, fmt.Errorf("%v: Stat() failed: %v", file.Name(), err.Error())
	}
	return f.currentPos > fileInfo.Size(), nil
}

type simpleLogger interface {
	Debug(format string, a ...interface{})
}
