package tailer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
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
		abspath, fd, _, file, reader, err := initWatcher(path, readall, lines)
		defer func() {
			if fd != 0 {
				syscall.Close(fd)
			}
			if file != nil {
				file.Close()
			}
		}()
		if err != nil {
			writeError(errors, done, "Failed to initialize file system watcher for %v: %v", path, err.Error())
			return
		}

		events, eventReaderErrors, eventReaderDone := startEventReader(fd)
		defer close(eventReaderDone)

		for {
			select {
			case <-done:
				return
			case err = <-eventReaderErrors:
				writeError(errors, done, "Failed to watch %v: %v", abspath, err.Error())
				return
			case evnts := <-events:

				// TODO: linereader must return lines, and need to send it to 'lines' channel while checking 'done' channel.

				file, reader, err = processEvents(evnts, file, reader, abspath, lines, logger)
				if err != nil {
					writeError(errors, done, "Failed to watch %v: %v", abspath, err.Error())
					return
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

func initWatcher(path string, readall bool, lines chan string) (abspath string, fd int, wd int, file *os.File, reader *bufferedLineReader, err error) {
	abspath, err = filepath.Abs(path)
	if err != nil {
		return
	}
	file, err = os.Open(abspath)
	if err != nil {
		return
	}
	if !readall {
		_, err = file.Seek(0, os.SEEK_END)
		if err != nil {
			return
		}
	}
	fd, err = syscall.InotifyInit1(syscall.IN_CLOEXEC)
	if err != nil {
		return
	}
	wd, err = syscall.InotifyAddWatch(fd, filepath.Dir(abspath), syscall.IN_MODIFY|syscall.IN_MOVED_FROM|syscall.IN_DELETE|syscall.IN_CREATE)
	if err != nil {
		return
	}
	reader = NewBufferedLineReader(file, lines)
	err = reader.ProcessAvailableLines()
	if err != nil {
		return
	}
	return
}

type eventWithName struct {
	syscall.InotifyEvent
	Name string
}

func startEventReader(fd int) (chan []eventWithName, chan error, chan struct{}) {
	events := make(chan []eventWithName)
	errors := make(chan error)
	done := make(chan struct{})

	go func() {
		defer func() {
			close(events)
			close(errors)
		}()

		buf := make([]byte, (syscall.SizeofInotifyEvent+syscall.NAME_MAX+1)*10)

		for {
			n, err := syscall.Read(fd, buf)

			if err == syscall.EINTR || err == syscall.EBADF { // TODO: CHECK IF THIS IS RETURNED WHEN FD IS CLOSED
				// fd closed
				return
			} else if err != nil {
				select {
				case errors <- err:
				case <-done:
				}
				return
			} else {
				eventList := make([]eventWithName, 0)
				for offset := 0; offset < n; {
					if n-offset < syscall.SizeofInotifyEvent {
						select {
						case errors <- fmt.Errorf("inotify: read %v bytes, but sizeof(struct inotify_event) is %v bytes.", n, syscall.SizeofInotifyEvent):
						case <-done:
						}
						return
					}
					event := eventWithName{*(*syscall.InotifyEvent)(unsafe.Pointer(&buf[offset])), ""}
					if event.Len > 0 {
						bytes := (*[syscall.NAME_MAX]byte)(unsafe.Pointer(&buf[offset+syscall.SizeofInotifyEvent]))
						event.Name = strings.TrimRight(string(bytes[0:event.Len]), "\000")
					}
					eventList = append(eventList, event)
					offset += syscall.SizeofInotifyEvent + int(event.Len)
				}
				select {
				case events <- eventList:
				case <-done:
					return
				}
			}
		}
	}()
	return events, errors, done
}

func processEvents(events []eventWithName, fileBefore *os.File, readerBefore *bufferedLineReader, abspath string, lines chan string, logger simpleLogger) (file *os.File, reader *bufferedLineReader, err error) {
	file = fileBefore
	reader = readerBefore
	filename := filepath.Base(abspath)
	var truncated bool
	for _, event := range events {
		logger.Debug("File system watcher received %v.\n", event2string(event))
	}

	// WRITE or TRUNCATE
	for _, event := range events {
		if file != nil && event.Name == filename && event.Mask&syscall.IN_MODIFY == syscall.IN_MODIFY {
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
			err = reader.ProcessAvailableLines()
			if err != nil {
				return
			}
		}
	}

	// MOVE or DELETE
	for _, event := range events {
		if file != nil && event.Name == filename && (event.Mask&syscall.IN_MOVED_FROM == syscall.IN_MOVED_FROM || event.Mask&syscall.IN_DELETE == syscall.IN_DELETE) {
			file.Close()
			file = nil
			reader = nil
		}
	}

	// CREATE
	for _, event := range events {
		if file == nil && event.Name == filename && event.Mask&syscall.IN_CREATE == syscall.IN_CREATE {
			file, err = os.Open(abspath)
			if err != nil {
				return
			}
			reader = NewBufferedLineReader(file, lines)
			err = reader.ProcessAvailableLines()
			if err != nil {
				return
			}
		}
	}
	return
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

func event2string(event eventWithName) string {
	result := "event"
	if len(event.Name) > 0 {
		result = fmt.Sprintf("%v with path %v and mask", result, event.Name)
	} else {
		result = fmt.Sprintf("%v with unknown path and mask", result)
	}
	if event.Mask&syscall.IN_ACCESS == syscall.IN_ACCESS {
		result = fmt.Sprintf("%v IN_ACCESS", result)
	}
	if event.Mask&syscall.IN_ATTRIB == syscall.IN_ATTRIB {
		result = fmt.Sprintf("%v IN_ATTRIB", result)
	}
	if event.Mask&syscall.IN_CLOSE_WRITE == syscall.IN_CLOSE_WRITE {
		result = fmt.Sprintf("%v IN_CLOSE_WRITE", result)
	}
	if event.Mask&syscall.IN_CLOSE_NOWRITE == syscall.IN_CLOSE_NOWRITE {
		result = fmt.Sprintf("%v IN_CLOSE_NOWRITE", result)
	}
	if event.Mask&syscall.IN_CREATE == syscall.IN_CREATE {
		result = fmt.Sprintf("%v IN_CREATE", result)
	}
	if event.Mask&syscall.IN_DELETE == syscall.IN_DELETE {
		result = fmt.Sprintf("%v IN_DELETE", result)
	}
	if event.Mask&syscall.IN_DELETE_SELF == syscall.IN_DELETE_SELF {
		result = fmt.Sprintf("%v IN_DELETE_SELF", result)
	}
	if event.Mask&syscall.IN_MODIFY == syscall.IN_MODIFY {
		result = fmt.Sprintf("%v IN_MODIFY", result)
	}
	if event.Mask&syscall.IN_MOVE_SELF == syscall.IN_MOVE_SELF {
		result = fmt.Sprintf("%v IN_MOVE_SELF", result)
	}
	if event.Mask&syscall.IN_MOVED_FROM == syscall.IN_MOVED_FROM {
		result = fmt.Sprintf("%v IN_MOVED_FROM", result)
	}
	if event.Mask&syscall.IN_MOVED_TO == syscall.IN_MOVED_TO {
		result = fmt.Sprintf("%v IN_MOVED_TO", result)
	}
	if event.Mask&syscall.IN_OPEN == syscall.IN_OPEN {
		result = fmt.Sprintf("%v IN_OPEN", result)
	}
	if event.Mask&syscall.IN_IGNORED == syscall.IN_IGNORED {
		result = fmt.Sprintf("%v IN_IGNORED", result)
	}
	if event.Mask&syscall.IN_ISDIR == syscall.IN_ISDIR {
		result = fmt.Sprintf("%v IN_ISDIR", result)
	}
	if event.Mask&syscall.IN_Q_OVERFLOW == syscall.IN_Q_OVERFLOW {
		result = fmt.Sprintf("%v IN_Q_OVERFLOW", result)
	}
	if event.Mask&syscall.IN_UNMOUNT == syscall.IN_UNMOUNT {
		result = fmt.Sprintf("%v IN_UNMOUNT", result)
	}
	return result
}

type simpleLogger interface {
	Debug(format string, a ...interface{})
}
