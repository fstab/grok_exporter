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
	"syscall"
)

type watcher struct {
	dir *os.File
	kq  int // file descriptor for the kevent queue
}

type eventList struct {
	events  []syscall.Kevent_t
	watcher *watcher
}

// File system event watcher, using BSD's kevent
func NewFseventWatcher(abspath string, file *File) (Watcher, error) {
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
	if file != nil {
		// logfile is already there, register for events on dir and file.
		_, err = syscall.Kevent(kq, []syscall.Kevent_t{makeEvent(dir), makeEvent(file.File)}, nil, &zeroTimeout)
	} else {
		// logfile not created yet, register for events on dir.
		_, err = syscall.Kevent(kq, []syscall.Kevent_t{makeEvent(dir)}, nil, &zeroTimeout)
	}
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
	events chan Events
	errors chan error
	done   chan struct{}
}

func (w *watcher) StartEventLoop() EventLoop {
	events := make(chan Events)
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
				case events <- &eventList{ // We cannot write a single event at a time, because sometimes MOVE and WRITE change order, and we need to process WRITE before MOVE if that happens.
					events:  eventBuf[:n],
					watcher: w,
				}:
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

func (l *eventLoop) Events() chan Events {
	return l.events
}

func (events *eventList) Process(fileBefore *File, reader *bufferedLineReader, abspath string, logger simpleLogger) (file *File, lines []string, err error) {
	file = fileBefore
	lines = []string{}
	var truncated bool
	logger.Debug("File system watcher received %v event(s):\n", len(events.events))
	for i, event := range events.events {
		logger.Debug("%v/%v: %v.\n", i+1, len(events.events), event2string(events.watcher.dir, file, event))
	}

	// Handle truncate events.
	for _, event := range events.events {
		if file != nil && event.Ident == fdToInt(file.Fd()) && event.Fflags&syscall.NOTE_ATTRIB == syscall.NOTE_ATTRIB {
			truncated, err = file.CheckTruncated()
			if err != nil {
				return
			}
			if truncated {
				_, err = file.Seek(0, io.SeekStart)
				if err != nil {
					return
				}
			}
		}
	}

	// Handle write event.
	for _, event := range events.events {
		if file != nil && event.Ident == fdToInt(file.Fd()) && event.Fflags&syscall.NOTE_WRITE == syscall.NOTE_WRITE {
			var freshLines []string
			freshLines, err = reader.ReadAvailableLines(file)
			if err != nil {
				return
			}
			lines = append(lines, freshLines...)
		}
	}

	// Handle move and delete events (NOTE_RENAME on the file's fd means the file was moved away, like in inotify's IN_MOVED_FROM).
	for _, event := range events.events {
		if file != nil && event.Ident == fdToInt(file.Fd()) && (event.Fflags&syscall.NOTE_DELETE == syscall.NOTE_DELETE || event.Fflags&syscall.NOTE_RENAME == syscall.NOTE_RENAME) {
			file.Close() // closing the fd will automatically remove event from kq.
			file = nil
			reader.Clear()
		}
	}

	// Handle move_to and create events (NOTE_WRITE on the directory's fd means a file was created or moved, so this covers inotify's MOVED_TO).
	for _, event := range events.events {
		if file == nil && event.Ident == fdToInt(events.watcher.dir.Fd()) && event.Fflags&syscall.NOTE_WRITE == syscall.NOTE_WRITE {
			file, err = open(abspath)
			if err == nil {
				zeroTimeout := syscall.NsecToTimespec(0) // timeout zero means non-blocking kevent() call
				_, err = syscall.Kevent(events.watcher.kq, []syscall.Kevent_t{makeEvent(file.File)}, nil, &zeroTimeout)
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
		Ident:  fdToInt(file.Fd()),
		Filter: syscall.EVFILT_VNODE,              // File modification and deletion events
		Flags:  syscall.EV_ADD | syscall.EV_CLEAR, // Add a new event, automatically enabled unless EV_DISABLE is specified
		Fflags: syscall.NOTE_DELETE | syscall.NOTE_WRITE | syscall.NOTE_EXTEND | syscall.NOTE_ATTRIB | syscall.NOTE_LINK | syscall.NOTE_RENAME | syscall.NOTE_REVOKE,
		Data:   0,
		Udata:  nil,
	}
}

func event2string(dir *os.File, file *File, event syscall.Kevent_t) string {
	result := "event"
	if dir != nil && event.Ident == fdToInt(dir.Fd()) {
		result = fmt.Sprintf("%v for logdir with fflags", result)
	} else if file != nil && event.Ident == fdToInt(file.Fd()) {
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
