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
	"github.com/fstab/grok_exporter/tailer/glob"
	"github.com/prometheus/common/log"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/winfsnotify"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type watcher struct {
	globs        []glob.Glob
	watchedDirs  []*Dir
	watchedFiles map[string]*fileWithReader // path -> fileWithReader
	winWatcher   *winfsnotify.Watcher
	lines        chan Line
	errors       chan Error
	done         chan struct{}
}

type fileWithReader struct {
	file   *File
	reader *lineReader
}

type fileInfo struct {
	filename string
	ffd      syscall.Win32finddata
}

func (f *fileInfo) Name() string {
	return f.filename
}

func (f *fileInfo) IsDir() bool {
	return f.ffd.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY == syscall.FILE_ATTRIBUTE_DIRECTORY
}

func (w *watcher) Lines() chan Line {
	return w.lines
}

func (w *watcher) Errors() chan Error {
	return w.errors
}

func (w *watcher) Close() {
	close(w.done)
}

func (w *watcher) shutdown() {

	close(w.lines)
	close(w.errors)

	warnf := func(format string, args ...interface{}) {
		log.Warnf("error while shutting down the file system watcher: %v", fmt.Sprint(format, args))
	}

	err := w.winWatcher.Close()
	if err != nil {
		warnf("failed to close winfsnotify.Watcher: %v", err)
	}

	for _, file := range w.watchedFiles {
		err = file.file.Close()
		if err != nil {
			warnf("close(%q) failed: %v", file.file.Name(), err)
		}
	}
}

func (w *watcher) runFseventProducerLoop() *winwatcherloop {
	return &winwatcherloop{
		events: w.winWatcher.Event,
		errors: w.winWatcher.Error,
	}
}

type winwatcherloop struct {
	events chan *winfsnotify.Event
	errors chan error
}

func (l *winwatcherloop) Close() {
	// noop, winwatcher.Close() called in shutdown()
}

func initWatcher(globs []glob.Glob) (*watcher, Error) {
	var (
		w = &watcher{
			globs:  globs,
			lines:  make(chan Line),
			errors: make(chan Error),
			done:   make(chan struct{}),
		}
		err error
	)
	w.winWatcher, err = winfsnotify.NewWatcher()
	if err != nil {
		return nil, NewError(NotSpecified, err, "failed to initialize file system watcher")
	}
	return w, nil
}

func (w *watcher) watchDirs(log logrus.FieldLogger) Error {
	var (
		err      error
		Err      Error
		dirPaths []string
		dirPath  string
	)

	dirPaths, Err = uniqueDirs(w.globs)
	if Err != nil {
		return Err
	}

	for _, dirPath = range dirPaths {
		log.Debugf("watching directory %v", dirPath)
		err = w.winWatcher.Watch(dirPath)
		if err != nil {
			return NewErrorf(NotSpecified, err, "%v: failed to watch directory", dirPath)
		}
		w.watchedDirs = append(w.watchedDirs, &Dir{path: dirPath})
	}
	return nil
}

func (w *watcher) watchNewFile(newFile *File) Error {
	// nothing to do, because on Windows we watch the directory and don't need to watch individual files.
	return nil
}

func (w *watcher) processEvent(event *winfsnotify.Event, log logrus.FieldLogger) Error {
	dir, fileName := w.dirAndFile(event.Name)
	if dir == nil {
		return NewError(NotSpecified, nil, "watch list inconsistent: received a file system event for an unknown directory")
	}
	log.WithField("directory", dir.path).Debugf("received event: %v", event)
	if event.Mask&winfsnotify.FS_MODIFY == winfsnotify.FS_MODIFY {
		file, ok := w.watchedFiles[filepath.Join(dir.path, fileName)]
		if !ok {
			return nil // unrelated file was modified
		}
		truncated, Err := file.file.CheckTruncated()
		if Err != nil {
			if Err.Type() == WinFileRemoved {
				return w.syncFilesInDir(dir, true, log)
			} else {
				return Err
			}
		}
		if truncated {
			_, err := file.file.Seek(0, io.SeekStart)
			if err != nil {
				return NewError(NotSpecified, os.NewSyscallError("seek", err), file.file.Name())
			}
			file.reader.Clear()
		}
		Err = w.readNewLines(file, log)
		if Err != nil {
			return Err
		}
	}
	if event.Mask&winfsnotify.FS_MOVED_FROM == winfsnotify.FS_MOVED_FROM || event.Mask&winfsnotify.FS_DELETE == winfsnotify.FS_DELETE || event.Mask&winfsnotify.FS_CREATE == winfsnotify.FS_CREATE || event.Mask&winfsnotify.FS_MOVED_TO == winfsnotify.FS_MOVED_TO {
		// There are a lot of corner cases here:
		// * a file is renamed, but still matches the pattern so we continue watching it (MOVED_FROM followed by MOVED_TO)
		// * a file is created overwriting an existing file
		// * a file is moved to the watched directory overwriting an existing file
		// Trying to figure out what happened from the events would be error prone.
		// Therefore, we don't care which of the above events we received, we just update our watched files with the current
		// state of the watched directory.
		err := w.syncFilesInDir(dir, true, log)
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *watcher) readNewLines(file *fileWithReader, log logrus.FieldLogger) Error {
	var (
		line string
		eof  bool
		err  error
	)
	for {
		line, eof, err = file.reader.ReadLine(file.file)
		if err != nil {
			return NewErrorf(NotSpecified, err, "%v: read() failed", file.file.Name())
		}
		if eof {
			return nil
		}
		log.Debugf("read line %q", line)
		select {
		case <-w.done:
			return nil
		case w.lines <- Line{Line: line, File: file.file.Name()}:
		}
	}
}

func (w *watcher) checkMissingFile() Error {
OUTER:
	for _, g := range w.globs {
		for watchedFileName, _ := range w.watchedFiles {
			if g.Match(watchedFileName) {
				continue OUTER
			}
		}
		// Error message must be phrased so that it makes sense for globs,
		// but also if g is a plain path without wildcards.
		return NewErrorf(FileNotFound, nil, "%v: no such file", g)
	}
	return nil
}

// Gets the directory paths from the glob expressions,
// and makes sure these directories exist.
func uniqueDirs(globs []glob.Glob) ([]string, Error) {
	var (
		result  = make([]string, 0, len(globs))
		g       glob.Glob
		dirInfo os.FileInfo
		err     error
	)
	for _, g = range globs {
		if containsString(result, g.Dir()) {
			continue
		}
		dirInfo, err = os.Stat(g.Dir())
		if err != nil {
			if os.IsNotExist(err) {
				return nil, NewErrorf(DirectoryNotFound, nil, "%q: no such directory", g.Dir())
			}
			return nil, NewErrorf(NotSpecified, err, "%q: stat() failed", g.Dir())
		}
		if !dirInfo.IsDir() {
			return nil, NewErrorf(NotSpecified, nil, "%q is not a directory", g.Dir())
		}
		result = append(result, g.Dir())
	}
	return result, nil
}

func isTruncated(file *os.File) (bool, Error) {
	currentPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return false, NewError(NotSpecified, os.NewSyscallError("seek", err), file.Name())
	}
	fileInfo, err := file.Stat()
	if err != nil {
		return false, NewError(NotSpecified, os.NewSyscallError("stat", err), file.Name())
	}
	return currentPos > fileInfo.Size(), nil
}

func anyGlobMatches(globs []glob.Glob, path string) bool {
	for _, pattern := range globs {
		if pattern.Match(path) {
			return true
		}
	}
	return false
}

func (w *watcher) findSameFile(newFileInfo *fileInfo, path string) (*fileWithReader, Error) {
	newFile, Err := open(path)
	if Err != nil {
		return nil, Err
	}
	defer newFile.Close()
	for _, watchedFile := range w.watchedFiles {
		if watchedFile.file.SameFile(newFile) {
			return watchedFile, nil
		}
	}
	return nil, nil
}

func containsString(list []string, s string) bool {
	for _, existing := range list {
		if existing == s {
			return true
		}
	}
	return false
}

func contains(list map[string]*fileWithReader, f *fileWithReader) bool {
	for _, existing := range list {
		if existing == f {
			return true
		}
	}
	return false
}

func (w *watcher) dirAndFile(fileOrDir string) (*Dir, string) {
	var (
		found *Dir
	)
	for _, dir := range w.watchedDirs {
		if strings.HasPrefix(fileOrDir, dir.path) {
			if found == nil || len(dir.path) > len(found.path) {
				found = dir
			}
		}
	}
	return found, strings.TrimLeft(fileOrDir[len(found.path):], "\\/")
}
