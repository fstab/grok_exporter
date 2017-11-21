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
	"strings"
	"syscall"
	"unsafe"
)

type watcher struct {
	fd int // file descriptor for reading inotify events
	wd int // watch descriptor for the log directory
}

// File system event watcher, using Linux's inotify.
func NewFseventWatcher(abspath string, _ *File) (Watcher, error) {
	fd, err := syscall.InotifyInit1(syscall.IN_CLOEXEC)
	if err != nil {
		return nil, err
	}
	wd, err := syscall.InotifyAddWatch(fd, filepath.Dir(abspath), syscall.IN_MODIFY|syscall.IN_MOVED_FROM|syscall.IN_MOVED_TO|syscall.IN_DELETE|syscall.IN_CREATE)
	if err != nil {
		return nil, err
	}
	return &watcher{
		fd: fd,
		wd: wd,
	}, nil
}

func (w *watcher) Close() error {
	var err error
	if w.fd != 0 {
		err = syscall.Close(w.fd)
	}
	return err
}

type eventLoop struct {
	w      *watcher
	events chan Events
	errors chan error
	done   chan struct{}
}

type eventWithName struct {
	syscall.InotifyEvent
	Name string
}

type eventList []eventWithName

func (w *watcher) StartEventLoop() EventLoop {
	events := make(chan Events)
	errors := make(chan error)
	done := make(chan struct{})

	go func() {
		defer func() {
			close(events)
			close(errors)
		}()

		buf := make([]byte, (syscall.SizeofInotifyEvent+syscall.NAME_MAX+1)*10)

		for {
			n, err := syscall.Read(w.fd, buf)
			if err != nil {
				select {
				case errors <- err:
				case <-done:
				}
				return
			} else {
				var eventsWithName eventList
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
					if event.Mask&syscall.IN_IGNORED == syscall.IN_IGNORED {
						// eventLoop.Close() was called or log dir was deleted
						return
					}
					eventsWithName = append(eventsWithName, event)
					offset += syscall.SizeofInotifyEvent + int(event.Len)
				}
				if len(eventsWithName) > 0 {
					select {
					case events <- eventsWithName:
					case <-done:
						return
					}
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
	_, err := syscall.InotifyRmWatch(l.w.fd, uint32(l.w.wd)) // generates an IN_IGNORED event, which interrupts the syscall.Read()
	close(l.done)
	return err
}

func (l *eventLoop) Errors() chan error {
	return l.errors
}

func (l *eventLoop) Events() chan Events {
	return l.events
}

func (events eventList) Process(fileBefore *File, reader *bufferedLineReader, abspath string, logger simpleLogger) (file *File, lines []string, err error) {
	file = fileBefore
	lines = []string{}
	filename := filepath.Base(abspath)
	var truncated bool
	for _, event := range events {
		logger.Debug("File system watcher received %v.\n", event2string(event))
	}

	// WRITE or TRUNCATE
	for _, event := range events {
		if file != nil && event.Name == filename && event.Mask&syscall.IN_MODIFY == syscall.IN_MODIFY {
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
			var freshLines []string
			freshLines, err = reader.ReadAvailableLines(file)
			if err != nil {
				return
			}
			lines = append(lines, freshLines...)
		}
	}

	// MOVED_FROM or DELETE
	for _, event := range events {
		if file != nil && event.Name == filename && (event.Mask&syscall.IN_MOVED_FROM == syscall.IN_MOVED_FROM || event.Mask&syscall.IN_DELETE == syscall.IN_DELETE) {
			file.Close()
			file = nil
			reader.Clear()
		}
	}

	// CREATE or MOVED_TO
	for _, event := range events {
		if file == nil && event.Name == filename && (event.Mask&syscall.IN_CREATE == syscall.IN_CREATE || event.Mask&syscall.IN_MOVED_TO == syscall.IN_MOVED_TO) {
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
	}
	return
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
