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

import "golang.org/x/exp/winfsnotify"

type winwatcherloop struct {
	events chan fsevent
	errors chan Error
	done   chan struct{}
}

func (l *winwatcherloop) Events() chan fsevent {
	return l.events
}

func (l *winwatcherloop) Errors() chan Error {
	return l.errors
}

func (l *winwatcherloop) Close() {
	close(l.done)
}

func runWinWatcherLoop(w *winfsnotify.Watcher) *winwatcherloop {
	var (
		events = make(chan fsevent)
		errors = make(chan Error)
		done   = make(chan struct{})
	)
	go func() {
		for {
			select {
			case event := <-w.Event:
				select {
				case events <- event:
				case <-done:
					w.Close()
					return
				}
			case err := <-w.Error:
				select {
				case errors <- NewError(NotSpecified, err, ""):
				case <-done:
					w.Close()
					return
				}
			case <-done:
				w.Close()
				return
			}
		}
	}()
	return &winwatcherloop{
		events: events,
		errors: errors,
		done:   done,
	}
}
