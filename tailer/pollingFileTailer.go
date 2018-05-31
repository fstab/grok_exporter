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
	"io"
	"time"
)

type pollingWatcher struct {
	abspath      string
	timeInterval time.Duration
}

type pollingEventLoop struct {
	events chan Events
	errors chan error
	done   chan struct{}
}

type pollingEvent struct{}

func NewPollingWatcher(abspath string, timeInterval time.Duration) (Watcher, error) {
	return &pollingWatcher{
		abspath:      abspath,
		timeInterval: timeInterval,
	}, nil
}

func (w *pollingWatcher) Close() error {
	return nil
}

func (w *pollingWatcher) StartEventLoop() EventLoop {
	events := make(chan Events)
	errors := make(chan error) // unused
	done := make(chan struct{})

	go func() {
		defer func() {
			close(events)
			close(errors)
		}()
		for {
			tick := time.After(w.timeInterval)
			select {
			case <-tick:
				events <- &pollingEvent{}
			case <-done:
				return
			}
		}
	}()
	return &pollingEventLoop{
		events: events,
		errors: errors,
		done:   done,
	}
}

func (l *pollingEventLoop) Close() error {
	close(l.done)
	return nil
}

func (l *pollingEventLoop) Errors() chan error {
	return l.errors
}

func (l *pollingEventLoop) Events() chan Events {
	return l.events
}

func (e *pollingEvent) Process(fileBefore *File, reader *bufferedLineReader, abspath string, logger simpleLogger) (file *File, lines []string, err error) {
	var (
		truncated, moved bool
		freshLines       []string
		filename         string
	)
	file = fileBefore
	moved, err = file.CheckMoved()
	if err != nil {
		return
	}
	if moved {
		freshLines, err = reader.ReadAvailableLines(file)
		if err != nil {
			return
		}
		lines = append(lines, freshLines...)
		filename = file.Name()
		file.Close()
		file, err = open(filename)
		if err != nil {
			return
		}
	}
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
	freshLines, err = reader.ReadAvailableLines(file)
	if err != nil {
		return
	}
	lines = append(lines, freshLines...)
	return
}
