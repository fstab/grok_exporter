// Copyright 2018-2020 The grok_exporter Authors
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
	"strings"
	"syscall"
)

type keventloop struct {
	kq     int
	events chan fsevent
	errors chan Error
	done   chan struct{}
}

// Terminate the kevent loop.
// If the loop hangs in syscall.Kevent(), it will keep hanging there until the next event is read.
// Therefore, after the consumer called Close(), it should interrupt the kevent() call by closing the kq descriptor.
func (p *keventloop) Close() {
	close(p.done)
}

func (p *keventloop) Events() chan fsevent {
	return p.events
}

func (p *keventloop) Errors() chan Error {
	return p.errors
}

func runKeventLoop(kq int) *keventloop {
	var result = &keventloop{
		kq:     kq,
		events: make(chan fsevent),
		errors: make(chan Error),
		done:   make(chan struct{}),
	}
	go func(l *keventloop) {
		var (
			n, i     int
			eventBuf []syscall.Kevent_t
			err      error
		)
		defer func() {
			close(result.errors)
			close(result.events)
		}()
		for {
			eventBuf = make([]syscall.Kevent_t, 10)
			n, err = syscall.Kevent(l.kq, nil, eventBuf, nil)
			if err == syscall.EBADF {
				// kq was closed, i.e. Close() was called.
				return
			} else if err == syscall.EINTR {
				continue
			} else if err != nil {
				select {
				case <-l.done:
				case l.errors <- NewError(NotSpecified, err, "kevent system call failed"):
				}
				return
			} else {
				for i = 0; i < n; i++ {
					select {
					case <-l.done:
						return
					case l.events <- eventBuf[i]:
					}
				}
			}
		}
	}(result)
	return result
}

func event2string(event syscall.Kevent_t) string {
	result := make([]string, 0, 1)
	if event.Fflags&syscall.NOTE_DELETE == syscall.NOTE_DELETE {
		result = append(result, "NOTE_DELETE")
	}
	if event.Fflags&syscall.NOTE_WRITE == syscall.NOTE_WRITE {
		result = append(result, "NOTE_WRITE")
	}
	if event.Fflags&syscall.NOTE_EXTEND == syscall.NOTE_EXTEND {
		result = append(result, "NOTE_EXTEND")
	}
	if event.Fflags&syscall.NOTE_ATTRIB == syscall.NOTE_ATTRIB {
		result = append(result, "NOTE_ATTRIB")
	}
	if event.Fflags&syscall.NOTE_LINK == syscall.NOTE_LINK {
		result = append(result, "NOTE_LINK")
	}
	if event.Fflags&syscall.NOTE_RENAME == syscall.NOTE_RENAME {
		result = append(result, "NOTE_RENAME")
	}
	if event.Fflags&syscall.NOTE_REVOKE == syscall.NOTE_REVOKE {
		result = append(result, "NOTE_REVOKE")
	}
	return strings.Join(result, ", ")
}
