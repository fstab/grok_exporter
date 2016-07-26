package tailer

import (
	"fmt"
	"golang.org/x/exp/winfsnotify"
	"os"
	"path/filepath"
	"strings"
)

func initWatcher(abspath string, _ *autoClosingFile) (*winfsnotify.Watcher, error) {
	watcher, err := winfsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	err = watcher.Watch(filepath.Dir(abspath))
	if err != nil {
		watcher.Close()
		return nil, err
	}
	return watcher, nil
}

type eventLoop struct {
	watcher *winfsnotify.Watcher
}

func startEventLoop(watcher *winfsnotify.Watcher) *eventLoop {
	return &eventLoop{
		watcher: watcher,
	}
}

func (l *eventLoop) Close() error {
	// watcher.Close() may be called twice, once for the eventLoop and once for the watcher. This should be fine.
	return l.watcher.Close()
}

func (l *eventLoop) Errors() chan error {
	return l.watcher.Error
}

func (l *eventLoop) Events() chan *winfsnotify.Event {
	return l.watcher.Event
}

func processEvents(event *winfsnotify.Event, _ *winfsnotify.Watcher, fileBefore *autoClosingFile, reader *bufferedLineReader, abspath string, logger simpleLogger) (file *autoClosingFile, lines []string, err error) {
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
		file, err = open(abspath)
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

func open(path string) (*autoClosingFile, error) {
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

func (f *autoClosingFile) Close() error {
	// nothing to do
	return nil
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
