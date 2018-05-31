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
	"golang.org/x/exp/winfsnotify"
	"io"
	"path/filepath"
	"strings"
)

// File system event watcher, using golang.org/x/exp/winfsnotify
func NewFseventWatcher(abspath string, _ *File) (Watcher, error) {
	w, err := winfsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	err = w.Watch(filepath.Dir(abspath))
	if err != nil {
		w.Close()
		return nil, err
	}
	return &watcher{w: w}, nil
}

type watcher struct {
	w *winfsnotify.Watcher
}

func (w *watcher) Close() error {
	// Nothing to do, because the winfsnotify's watcher is closed with the event loop's Close() function.
	return nil
}

type eventLoop struct {
	w      *winfsnotify.Watcher
	events chan Events
	errors chan error
	done   chan struct{}
}

type event struct {
	*winfsnotify.Event
}

func (watcher *watcher) StartEventLoop() EventLoop {
	events := make(chan Events)
	errors := make(chan error)
	done := make(chan struct{})

	go func() {
		defer func() {
			close(events)
			close(errors)
		}()
		for {
			select {
			case ev := <-watcher.w.Event:
				select {
				case events <- &event{ev}:
				case <-done:
					return
				}
			case err := <-watcher.w.Error:
				select {
				case errors <- err:
				case <-done:
				}
				return
			case <-done:
				return
			}
		}
	}()
	return &eventLoop{
		w:      watcher.w,
		events: events,
		errors: errors,
		done:   done,
	}
}

func (l *eventLoop) Close() error {
	close(l.done)
	return l.w.Close()
}

func (l *eventLoop) Errors() chan error {
	return l.errors
}

func (l *eventLoop) Events() chan Events {
	return l.events
}

func (event *event) Process(fileBefore *File, reader *bufferedLineReader, abspath string, logger simpleLogger) (file *File, lines []string, err error) {
	file = fileBefore
	lines = []string{}
	var truncated bool
	logger.Debug("File system watcher received %v.\n", event.String())

	// WRITE or TRUNCATE
	if file != nil && norm(event.Name) == norm(abspath) && event.Mask&winfsnotify.FS_MODIFY == winfsnotify.FS_MODIFY {
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

	// MOVED_FROM or DELETE
	if file != nil && norm(event.Name) == norm(abspath) && (event.Mask&winfsnotify.FS_MOVED_FROM == winfsnotify.FS_MOVED_FROM || event.Mask&winfsnotify.FS_DELETE == winfsnotify.FS_DELETE) {
		file = nil
		reader.Clear()
	}

	// MOVED_TO or CREATE
	if file == nil && norm(event.Name) == norm(abspath) && (event.Mask&winfsnotify.FS_MOVED_TO == winfsnotify.FS_MOVED_TO || event.Mask&winfsnotify.FS_CREATE == winfsnotify.FS_CREATE) {
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
	return
}

// winfsnotify uses "/" instead of "\" when constructing the path in the event name.
func norm(path string) string {
	path = strings.Replace(path, "/", "\\", -1)
	path = strings.Replace(path, "\\\\", "\\", -1)
	return path
}
