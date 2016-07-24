// +build !linux,!windows

package tailer

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"time"
)

type fileTailer struct {
	lines  chan string
	errors chan error
	done   chan bool
	closed bool
}

func (f *fileTailer) Close() {
	if !f.closed {
		f.closed = true
		f.done <- true // Blocks until the goroutine is done, so we are sure we get no writes to lines or errors after that.
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
	done := make(chan bool)
	errors := make(chan error)
	go func() {
		abspath, dir, file, reader, kq, err := initWatcher(path, readall, lines)
		if err != nil {
			errors <- fmt.Errorf("Failed to initialize file system watcher for %v: %v", path, err.Error())
			closeAll(dir, file, kq)
			<-done
			return
		}

		// Timeout is needed so that we check the done channel from time to time.
		timeout := unix.NsecToTimespec((1 * time.Second).Nanoseconds())

		for {
			select {
			case <-done:
				closeAll(dir, file, kq)
				return
			default:
				file, reader, err = processEvents(kq, timeout, dir, file, reader, abspath, lines, logger)
				if err != nil {
					errors <- fmt.Errorf("Failed to watch %v: %v", abspath, err.Error())
					closeAll(dir, file, kq)
					<-done
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

func initWatcher(path string, readall bool, lines chan string) (abspath string, dir *os.File, file *os.File, reader *bufferedLineReader, kq int, err error) {
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
	dir, err = os.Open(filepath.Dir(abspath))
	if err != nil {
		return
	}
	kq, err = unix.Kqueue()
	if err != nil {
		return
	}
	zeroTimeout := unix.NsecToTimespec(0) // timeout zero means non-blocking kevent() call

	// Register for events on dir and file.
	_, err = unix.Kevent(kq, []unix.Kevent_t{makeEvent(dir), makeEvent(file)}, nil, &zeroTimeout)
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

func processEvents(kq int, timeout unix.Timespec, dir *os.File, fileBefore *os.File, readerBefore *bufferedLineReader, abspath string, lines chan string, logger simpleLogger) (file *os.File, reader *bufferedLineReader, err error) {
	var n int
	file = fileBefore
	reader = readerBefore
	events := make([]unix.Kevent_t, 10)
	zeroTimeout := unix.NsecToTimespec(0) // timeout zero means non-blocking kevent() call
	n, err = unix.Kevent(kq, nil, events, &timeout)
	if err != nil {
		return
	}
	logger.Debug("File system watcher got %v events:\n", n)
	for i := 0; i < n; i++ {
		logger.Debug(" * %s\n", event2string(dir, file, events[i]))
	}
	for i := 0; i < n; i++ {
		// Handle truncate events.
		if file != nil && events[i].Ident == uint64(file.Fd()) {
			if events[i].Fflags&unix.NOTE_ATTRIB == unix.NOTE_ATTRIB {
				_, err = file.Seek(0, os.SEEK_SET)
				if err != nil {
					return
				}
			}
		}
	}
	for i := 0; i < n; i++ {
		// Handle write events.
		if file != nil && events[i].Ident == uint64(file.Fd()) {
			if events[i].Fflags&unix.NOTE_WRITE == unix.NOTE_WRITE {
				err = reader.ProcessAvailableLines()
				if err != nil {
					return
				}
			}
		}
	}
	for i := 0; i < n; i++ {
		// Handle move and delete events.
		if file != nil && events[i].Ident == uint64(file.Fd()) {
			if events[i].Fflags&unix.NOTE_DELETE == unix.NOTE_DELETE || events[i].Fflags&unix.NOTE_RENAME == unix.NOTE_RENAME {
				file.Close() // closing the fd will automatically remove event from kq.
				file = nil
				reader = nil
			}
		}
	}
	for i := 0; i < n; i++ {
		// Handle create events.
		if file == nil && events[i].Ident == uint64(dir.Fd()) {
			if events[i].Fflags&unix.NOTE_WRITE == unix.NOTE_WRITE {
				file, err = os.Open(abspath)
				if err == nil {
					_, err = unix.Kevent(kq, []unix.Kevent_t{makeEvent(file)}, nil, &zeroTimeout)
					if err != nil {
						return
					}
					reader = NewBufferedLineReader(file, lines)
					err = reader.ProcessAvailableLines()
					if err != nil {
						return
					}
				} else {
					// If file could not be opened, the CREATE event was for another file, we ignore this.
					err = nil
				}
			}
		}
	}
	return
}

func makeEvent(file *os.File) unix.Kevent_t {

	// Note about the EV_CLEAR flag:
	//
	// The NOTE_WRITE event is triggered by the first write to the file after register, and remains set.
	// This means that we continue to receive the event indefinitely.
	//
	// There are two flags to stop receiving the event over and over again:
	//
	// * EV_ONESHOT: This suppresses consecutive events of the same type. However, that means that means that
	//               we don't receive new WRITE events even if new lines are written to the file.
	//               Therefore we cannot use EV_ONESHOT.
	// * EV_CLEAR:   This resets the state after the event, so that an event is only delivered once for each write.
	//               (Actually it could be less than once per write, since events are coalesced.)
	//               This is our desired behaviour.
	//
	// See also http://benno.id.au/blog/2008/05/15/simplefilemon

	return unix.Kevent_t{
		Ident:  uint64(file.Fd()),
		Filter: unix.EVFILT_VNODE,           // File modification and deletion events
		Flags:  unix.EV_ADD | unix.EV_CLEAR, // Add a new event, automatically enabled unless EV_DISABLE is specified
		Fflags: unix.NOTE_DELETE | unix.NOTE_WRITE | unix.NOTE_EXTEND | unix.NOTE_ATTRIB | unix.NOTE_LINK | unix.NOTE_RENAME | unix.NOTE_REVOKE,
		Data:   0,
		Udata:  nil,
	}
}

func closeAll(dir *os.File, file *os.File, kq int) {
	if dir != nil {
		dir.Close()
	}
	if file != nil {
		file.Close()
	}
	if kq != 0 {
		unix.Close(kq)
	}
}

type simpleLogger interface {
	Debug(format string, a ...interface{})
}

func event2string(dir *os.File, file *os.File, event unix.Kevent_t) string {
	result := "event"
	if dir != nil && event.Ident == uint64(dir.Fd()) {
		result = fmt.Sprintf("%v for logdir with fflags", result)
	} else if file != nil && event.Ident == uint64(file.Fd()) {
		result = fmt.Sprintf("%v for logfile with fflags", result)
	} else {
		result = fmt.Sprintf("%s for unknown fd=%v with fflags", result, event.Ident)
	}

	if event.Fflags&unix.NOTE_DELETE == unix.NOTE_DELETE {
		result = fmt.Sprintf("%v NOTE_DELETE", result)
	}
	if event.Fflags&unix.NOTE_WRITE == unix.NOTE_WRITE {
		result = fmt.Sprintf("%v NOTE_WRITE", result)
	}
	if event.Fflags&unix.NOTE_EXTEND == unix.NOTE_EXTEND {
		result = fmt.Sprintf("%v NOTE_EXTEND", result)
	}
	if event.Fflags&unix.NOTE_ATTRIB == unix.NOTE_ATTRIB {
		result = fmt.Sprintf("%v NOTE_ATTRIB", result)
	}
	if event.Fflags&unix.NOTE_LINK == unix.NOTE_LINK {
		result = fmt.Sprintf("%v NOTE_LINK", result)
	}
	if event.Fflags&unix.NOTE_RENAME == unix.NOTE_RENAME {
		result = fmt.Sprintf("%v NOTE_RENAME", result)
	}
	if event.Fflags&unix.NOTE_REVOKE == unix.NOTE_REVOKE {
		result = fmt.Sprintf("%v NOTE_REVOKE", result)
	}
	return result
}
