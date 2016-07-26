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
	done   chan struct{}
	closed bool
}

func (f *fileTailer) Close() {
	if !f.closed {
		f.closed = true
		close(f.done)
		close(f.lines)
		close(f.errors)
	}
}

func (f *fileTailer) Lines() chan string {
	return f.lines
}

func (f *fileTailer) Errors() chan error {
	return f.errors
}

func RunFileTailer(path string, readall bool, logger simpleLogger) Tailer {
	lines := make(chan string)
	done := make(chan struct{})
	errors := make(chan error)
	go func() {
		abspath, watcher, file, err := initWatcher(path, readall)
		defer func() {
			watcher.Close()
		}()
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

		for {
			select {
			case <-done:
				return
			case err = <-watcher.Error:
				writeError(errors, done, "Failed to watch %v: %v", abspath, err.Error())
				return
			case event := <-watcher.Event:
				var freshLines []string
				file, freshLines, err = processEvent(event, file, reader, abspath, logger)
				if err != nil {
					writeError(errors, done, "Failed to watch %v: %v", abspath, err.Error())
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

func writeError(errors chan error, done chan struct{}, format string, a ...interface{}) {
	select {
	case errors <- fmt.Errorf(format, a...):
	case <-done:
	}
}

func initWatcher(path string, readall bool) (abspath string, watcher *winfsnotify.Watcher, file *autoClosingFile, err error) {
	abspath, err = filepath.Abs(path)
	if err != nil {
		return
	}
	file, err = Open(abspath)
	if err != nil {
		return
	}
	if !readall {
		_, err = file.Seek(0, os.SEEK_END)
		if err != nil {
			return
		}
	}
	watcher, err = winfsnotify.NewWatcher()
	if err != nil {
		return
	}
	err = watcher.Watch(filepath.Dir(abspath))
	if err != nil {
		return
	}
	return
}

func processEvent(event *winfsnotify.Event, fileBefore *autoClosingFile, reader *bufferedLineReader, abspath string, logger simpleLogger) (file *autoClosingFile, lines []string, err error) {
	file = fileBefore
	lines = []string{}
	var truncated bool
	logger.Debug("File system watcher received %v.\n", event.String())

	// WRITE or TRUNCATE
	if file != nil && norm(event.Name) == norm(abspath) && event.Mask&winfsnotify.FS_MODIFY == winfsnotify.FS_MODIFY {
		truncated, err = checkTruncated(file)
		if err != nil {
			return
		}
		if truncated {
			_, err = file.Seek(0, os.SEEK_SET)
			if err != nil {
				return
			}
		}
		var freshLines []string
		freshLines, err = reader.ReadAvailableLines(file)
		if err != nil {
			return
		}
		lines = append(lines, freshLines...)
	}

	// MOVE or DELETE
	if file != nil && norm(event.Name) == norm(abspath) && (event.Mask&winfsnotify.FS_MOVED_FROM == winfsnotify.FS_MOVED_FROM || event.Mask&winfsnotify.FS_DELETE == winfsnotify.FS_DELETE) {
		file = nil
		reader.Clear()
	}

	// CREATE
	if file == nil && norm(event.Name) == norm(abspath) && event.Mask&winfsnotify.FS_CREATE == winfsnotify.FS_CREATE {
		file, err = Open(abspath)
		if err != nil {
			return
		}
		reader.Clear()
		var freshLines []string
		freshLines, err = reader.ReadAvailableLines(file)
		if err != nil {
			return
		}
		lines = append(lines, freshLines...)
	}
	return
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
