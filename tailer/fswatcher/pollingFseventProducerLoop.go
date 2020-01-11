// Copyright 2019-2020 The grok_exporter Authors
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

import "time"

type pollloop struct {
	events chan fsevent
	errors chan Error // unused
	done   chan struct{}
}

func (l *pollloop) Events() chan fsevent {
	return l.events
}

func (l *pollloop) Errors() chan Error {
	return l.errors
}

func (l *pollloop) Close() {
	close(l.done)
}

func runPollLoop(pollInterval time.Duration) *pollloop {

	events := make(chan fsevent)
	errors := make(chan Error) // unused
	done := make(chan struct{})

	go func() {
		defer func() {
			close(events)
			close(errors)
		}()
		for {
			tick := time.After(pollInterval)
			select {
			case <-tick:
				select {
				case events <- struct{}{}:
				case <-done:
					return
				}
			case <-done:
				return
			}
		}
	}()
	return &pollloop{
		events: events,
		errors: errors,
		done:   done,
	}
}
