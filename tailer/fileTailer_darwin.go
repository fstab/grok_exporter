package tailer

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func open(abspath string) (*os.File, error) {
	return os.Open(abspath)
}

type watcher struct {
	dir *os.File
	kq  int // file descriptor for the kevent queue
}

func initWatcher(abspath string, file *os.File) (*watcher, error) {
	dir, err := os.Open(filepath.Dir(abspath))
	if err != nil {
		return nil, err
	}
	kq, err := syscall.Kqueue()
	if err != nil {
		dir.Close()
		return nil, err
	}
	zeroTimeout := syscall.NsecToTimespec(0) // timeout zero means non-blocking kevent() call
	// Register for events on dir and file.
	_, err = syscall.Kevent(kq, []syscall.Kevent_t{makeEvent(dir), makeEvent(file)}, nil, &zeroTimeout)
	if err != nil {
		dir.Close()
		syscall.Close(kq)
		return nil, err
	}
	return &watcher{
		dir: dir,
		kq:  kq,
	}, nil
}

func (w *watcher) Close() error {
	var err1, err2 error
	if w.dir != nil {
		err1 = w.dir.Close()
	}
	if w.kq != 0 {
		err2 = syscall.Close(w.kq)
	}
	if err1 != nil {
		return err1
	}
	return err2
}

type eventLoop struct {
	w      *watcher
	events chan []syscall.Kevent_t
	errors chan error
	done   chan struct{}
}

func startEventLoop(w *watcher) *eventLoop {
	events := make(chan []syscall.Kevent_t)
	errors := make(chan error)
	done := make(chan struct{})
	go func() {
		defer func() {
			close(events)
			close(errors)
		}()
		for {
			eventBuf := make([]syscall.Kevent_t, 10)
			n, err := syscall.Kevent(w.kq, nil, eventBuf, nil)
			if err == syscall.EINTR || err == syscall.EBADF {
				// kq was closed, i.e. eventLoop.Close() was called.
				return
			} else if err != nil {
				select {
				case errors <- err:
				case <-done:
				}
				return
			} else {
				select {
				case events <- eventBuf[:n]: // We cannot write a single event at a time, because sometimes MOVE and WRITE change order, and we need to process WRITE before MOVE if that happens.
				case <-done:
					return
				}
			}
		}
	}()
	return &eventLoop{
		w:      w,
		events: events,
		errors: errors,
		done:   done,
	}
}

func (l *eventLoop) Close() error {
	err := syscall.Close(l.w.kq) // Interrupt the blocking kevent() system call.
	l.w.kq = 0                   // Prevent it from being closed again when watcher.Close() is called.
	close(l.done)
	return err
}

func (l *eventLoop) Errors() chan error {
	return l.errors
}

func (l *eventLoop) Events() chan []syscall.Kevent_t {
	return l.events
}

func processEvents(events []syscall.Kevent_t, w *watcher, fileBefore *os.File, reader *bufferedLineReader, abspath string, logger simpleLogger) (file *os.File, lines []string, err error) {
	file = fileBefore
	lines = []string{}
	for _, event := range events {
		logger.Debug("File system watcher received %v.\n", event2string(w.dir, file, event))
	}

	// Handle truncate events.
	for _, event := range events {
		if file != nil && event.Ident == uint64(file.Fd()) && event.Fflags&syscall.NOTE_ATTRIB == syscall.NOTE_ATTRIB {
			_, err = file.Seek(0, os.SEEK_SET)
			if err != nil {
				return
			}
		}
	}

	// Handle write event.
	for _, event := range events {
		if file != nil && event.Ident == uint64(file.Fd()) && event.Fflags&syscall.NOTE_WRITE == syscall.NOTE_WRITE {
			var freshLines []string
			freshLines, err = reader.ReadAvailableLines(file)
			if err != nil {
				return
			}
			lines = append(lines, freshLines...)
		}
	}

	// Handle move and delete events.
	for _, event := range events {
		if file != nil && event.Ident == uint64(file.Fd()) && (event.Fflags&syscall.NOTE_DELETE == syscall.NOTE_DELETE || event.Fflags&syscall.NOTE_RENAME == syscall.NOTE_RENAME) {
			file.Close() // closing the fd will automatically remove event from kq.
			file = nil
			reader.Clear()
		}
	}

	// Handle create events.
	for _, event := range events {
		if file == nil && event.Ident == uint64(w.dir.Fd()) && event.Fflags&syscall.NOTE_WRITE == syscall.NOTE_WRITE {
			file, err = os.Open(abspath)
			if err == nil {
				zeroTimeout := syscall.NsecToTimespec(0) // timeout zero means non-blocking kevent() call
				_, err = syscall.Kevent(w.kq, []syscall.Kevent_t{makeEvent(file)}, nil, &zeroTimeout)
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
			} else {
				// If file could not be opened, the CREATE event was for another file, we ignore this.
				err = nil
			}
		}
	}
	return
}

func makeEvent(file *os.File) syscall.Kevent_t {

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

	return syscall.Kevent_t{
		Ident:  uint64(file.Fd()),
		Filter: syscall.EVFILT_VNODE,              // File modification and deletion events
		Flags:  syscall.EV_ADD | syscall.EV_CLEAR, // Add a new event, automatically enabled unless EV_DISABLE is specified
		Fflags: syscall.NOTE_DELETE | syscall.NOTE_WRITE | syscall.NOTE_EXTEND | syscall.NOTE_ATTRIB | syscall.NOTE_LINK | syscall.NOTE_RENAME | syscall.NOTE_REVOKE,
		Data:   0,
		Udata:  nil,
	}
}

func event2string(dir *os.File, file *os.File, event syscall.Kevent_t) string {
	result := "event"
	if dir != nil && event.Ident == uint64(dir.Fd()) {
		result = fmt.Sprintf("%v for logdir with fflags", result)
	} else if file != nil && event.Ident == uint64(file.Fd()) {
		result = fmt.Sprintf("%v for logfile with fflags", result)
	} else {
		result = fmt.Sprintf("%s for unknown fd=%v with fflags", result, event.Ident)
	}

	if event.Fflags&syscall.NOTE_DELETE == syscall.NOTE_DELETE {
		result = fmt.Sprintf("%v NOTE_DELETE", result)
	}
	if event.Fflags&syscall.NOTE_WRITE == syscall.NOTE_WRITE {
		result = fmt.Sprintf("%v NOTE_WRITE", result)
	}
	if event.Fflags&syscall.NOTE_EXTEND == syscall.NOTE_EXTEND {
		result = fmt.Sprintf("%v NOTE_EXTEND", result)
	}
	if event.Fflags&syscall.NOTE_ATTRIB == syscall.NOTE_ATTRIB {
		result = fmt.Sprintf("%v NOTE_ATTRIB", result)
	}
	if event.Fflags&syscall.NOTE_LINK == syscall.NOTE_LINK {
		result = fmt.Sprintf("%v NOTE_LINK", result)
	}
	if event.Fflags&syscall.NOTE_RENAME == syscall.NOTE_RENAME {
		result = fmt.Sprintf("%v NOTE_RENAME", result)
	}
	if event.Fflags&syscall.NOTE_REVOKE == syscall.NOTE_REVOKE {
		result = fmt.Sprintf("%v NOTE_REVOKE", result)
	}
	return result
}
