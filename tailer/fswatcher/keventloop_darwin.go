// Copyright 2018 The grok_exporter Authors
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
	"syscall"
)

type keventloop struct {
	kq     int
	events chan syscall.Kevent_t
	errors chan Error
	done   chan struct{}
}

func (p *keventloop) Close() {
	close(p.done)
	close(p.errors)
	close(p.events)
	// closing the kq file descriptor will interrupt syscall.Kevent()
	syscall.Close(p.kq)
}

func runKeventLoop(kq int) *keventloop {
	var result = &keventloop{
		kq:     kq,
		events: make(chan syscall.Kevent_t),
		errors: make(chan Error),
		done:   make(chan struct{}),
	}
	go func(l *keventloop) {
		var (
			n, i     int
			eventBuf []syscall.Kevent_t
			err      error
		)
		for {
			eventBuf = make([]syscall.Kevent_t, 10)
			n, err = syscall.Kevent(l.kq, nil, eventBuf, nil)
			if err == syscall.EINTR || err == syscall.EBADF {
				// kq was closed, i.e. Close() was called.
				return
			} else if err != nil {
				select {
				case <-l.done:
				case l.errors <- NewError(NotSpecified, "kevent system call failed", err):
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
