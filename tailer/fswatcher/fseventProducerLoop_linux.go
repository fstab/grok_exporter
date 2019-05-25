// Copyright 2019 The grok_exporter Authors
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

package fswatcher

import (
	"fmt"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type inotifyloop struct {
	fd     int
	events chan fsevent
	errors chan Error
	done   chan struct{}
}

type inotifyEvent struct {
	syscall.InotifyEvent
	Name string
}

func (l *inotifyloop) Events() chan fsevent {
	return l.events
}

func (l *inotifyloop) Errors() chan Error {
	return l.errors
}

// Terminate the inotify loop.
// If the loop hangs in syscall.Read(), it will keep hanging there until the next event is read.
// Therefore, after the consumer called Close(), it should generate an artificial IN_IGNORE event to
// interrupt syscall.Read(). This can be done by calling inotify_rm_watch() on one of the watched directories.
func (l *inotifyloop) Close() {
	close(l.done)
}

func runInotifyLoop(fd int) *inotifyloop {
	var result = &inotifyloop{
		fd:     fd,
		events: make(chan fsevent),
		errors: make(chan Error),
		done:   make(chan struct{}),
	}
	go func(l *inotifyloop) {
		var (
			n, offset int
			event     inotifyEvent
			bytes     *[syscall.NAME_MAX]byte
			buf       = make([]byte, (syscall.SizeofInotifyEvent+syscall.NAME_MAX+1)*10)
			err       error
		)
		defer func() {
			close(result.errors)
			close(result.events)
		}()
		for {
			n, err = syscall.Read(l.fd, buf)
			if err != nil {
				// Getting an err might be part of the shutdown, when l.fd is closed.
				// We decide whether it is an actual error or not by checking if l.done is closed.
				select {
				case <-l.done:
				case <-time.After(2 * time.Second):
					select {
					case l.errors <- NewError(NotSpecified, err, "failed to read inotify events"):
					case <-l.done:
					}
				}
				return
			}
			for offset = 0; offset < n; {
				if n-offset < syscall.SizeofInotifyEvent {
					select {
					case l.errors <- NewError(NotSpecified, nil, fmt.Sprintf("inotify: read %v bytes, but sizeof(struct inotify_event) is %v bytes.", n, syscall.SizeofInotifyEvent)):
					case <-l.done:
					}
					return
				}
				event = inotifyEvent{*(*syscall.InotifyEvent)(unsafe.Pointer(&buf[offset])), ""}
				if event.Len > 0 {
					bytes = (*[syscall.NAME_MAX]byte)(unsafe.Pointer(&buf[offset+syscall.SizeofInotifyEvent]))
					event.Name = strings.TrimRight(string(bytes[0:event.Len]), "\000")
				}
				select {
				case l.events <- event:
				case <-l.done:
					return
				}
				if event.Mask&syscall.IN_IGNORED == syscall.IN_IGNORED {
					// IN_IGNORED event can have two reasons:
					// 1) The consumer loop is shutting down and called inotify_rm_watch() to interrupt syscall.Read()
					// 2) The watched directory was deleted. fswatcher will report an error and terminate if that happens.
					// In both cases, we should terminate here and not call syscall.Read() again, as the next
					// call might block forever as we don't receive events anymore.
					return
				}
				offset += syscall.SizeofInotifyEvent + int(event.Len)
			}
		}
	}(result)
	return result
}

func (e inotifyEvent) String() string {
	var name = "<nil>"
	if len(e.Name) > 0 {
		name = e.Name
	}
	return fmt.Sprintf("%v: %v", name, flags2string(e.InotifyEvent))
}

func flags2string(event syscall.InotifyEvent) string {
	result := make([]string, 0, 1)
	if event.Mask&syscall.IN_ACCESS == syscall.IN_ACCESS {
		result = append(result, "IN_ACCESS")
	}
	if event.Mask&syscall.IN_ATTRIB == syscall.IN_ATTRIB {
		result = append(result, "IN_ATTRIB")
	}
	if event.Mask&syscall.IN_CLOSE_WRITE == syscall.IN_CLOSE_WRITE {
		result = append(result, "IN_CLOSE_WRITE")
	}
	if event.Mask&syscall.IN_CLOSE_NOWRITE == syscall.IN_CLOSE_NOWRITE {
		result = append(result, "IN_CLOSE_NOWRITE")
	}
	if event.Mask&syscall.IN_CREATE == syscall.IN_CREATE {
		result = append(result, "IN_CREATE")
	}
	if event.Mask&syscall.IN_DELETE == syscall.IN_DELETE {
		result = append(result, "IN_DELETE")
	}
	if event.Mask&syscall.IN_DELETE_SELF == syscall.IN_DELETE_SELF {
		result = append(result, "IN_DELETE_SELF")
	}
	if event.Mask&syscall.IN_IGNORED == syscall.IN_IGNORED {
		result = append(result, "IN_IGNORED")
	}
	if event.Mask&syscall.IN_MODIFY == syscall.IN_MODIFY {
		result = append(result, "IN_MODIFY")
	}
	if event.Mask&syscall.IN_MOVE_SELF == syscall.IN_MOVE_SELF {
		result = append(result, "IN_MOVE_SELF")
	}
	if event.Mask&syscall.IN_MOVED_FROM == syscall.IN_MOVED_FROM {
		result = append(result, "IN_MOVED_FROM")
	}
	if event.Mask&syscall.IN_MOVED_TO == syscall.IN_MOVED_TO {
		result = append(result, "IN_MOVED_TO")
	}
	if event.Mask&syscall.IN_OPEN == syscall.IN_OPEN {
		result = append(result, "IN_OPEN")
	}
	return strings.Join(result, ", ")
}
