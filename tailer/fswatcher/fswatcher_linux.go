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
	"io"
	"os"
	"path/filepath"
	"syscall"
)

type watcher struct {
	globs        []glob.Glob
	watchedDirs  []*Dir
	watchedFiles map[string]*fileWithReader // path -> fileWithReader
	fd           int
	lines        chan Line
	errors       chan Error
	done         chan struct{}
}

type fileWithReader struct {
	file   *os.File
	reader *lineReader
}

func (w *watcher) Lines() chan Line {
	return w.lines
}

func (w *watcher) Errors() chan Error {
	return w.errors
}

func (w *watcher) Close() {
	// Closing the done channel will stop the consumer loop.
	// Deferred functions within the consumer loop will close the producer loop.
	close(w.done)
}

func (w *watcher) shutdown() {

	close(w.lines)
	close(w.errors)

	warnf := func(format string, args ...interface{}) {
		log.Warnf("error while shutting down the file system watcher: %v", fmt.Sprint(format, args))
	}

	for wd, dirPath := range w.watchedDirs {
		// After calling eventProducerLoop.Close(), we need to call inotify_rm_watch()
		// in order to terminate the inotify loop. See eventProducerLoop.Close().
		success, err := syscall.InotifyRmWatch(w.fd, uint32(wd))
		if success != 0 || err != nil {
			warnf("inotify_rm_watch(%q) failed: status=%v, err=%v", dirPath, success, err)
		}
	}

	err := syscall.Close(w.fd)
	if err != nil {
		warnf("failed to close the inotify file descriptor: %v", err)
	}

	for _, file := range w.watchedFiles {
		err = file.file.Close()
		if err != nil {
			warnf("close(%q) failed: %v", file.file.Name(), err)
		}
	}
}

func (w *watcher) runFseventProducerLoop() *inotifyloop {
	return runInotifyLoop(w.fd)
}

func initWatcher(globs []glob.Glob) (*watcher, Error) {
	var (
		w = &watcher{
			globs:        globs,
			watchedFiles: make(map[string]*fileWithReader),
			lines:        make(chan Line),
			errors:       make(chan Error),
			done:         make(chan struct{}),
		}
		err error
	)
	w.fd, err = syscall.InotifyInit1(syscall.IN_CLOEXEC)
	if err != nil {
		return nil, NewError(NotSpecified, err, "inotify_init1() failed")
	}
	return w, nil
}

func (w *watcher) watchDirs(log logrus.FieldLogger) Error {
	var (
		wd       int
		dirPaths []string
		dirPath  string
		Err      Error
		err      error
	)
	dirPaths, Err = uniqueDirs(w.globs)
	if Err != nil {
		return Err
	}
	for _, dirPath = range dirPaths {
		log.Debugf("watching directory %v", dirPath)
		wd, err = syscall.InotifyAddWatch(w.fd, dirPath, syscall.IN_MODIFY|syscall.IN_MOVED_FROM|syscall.IN_MOVED_TO|syscall.IN_DELETE|syscall.IN_CREATE)
		if err != nil {
			w.Close()
			return NewErrorf(NotSpecified, err, "%q: inotify_add_watch() failed", dirPath)
		}
		w.watchedDirs = append(w.watchedDirs, &Dir{wd: wd, path: dirPath})
	}
	return nil
}

func (w *watcher) watchNewFile(newFile *os.File) Error {
	// nothing to do, because on Linux we watch the directory and don't need to watch individual files.
	return nil
}

func (w *watcher) findDir(event inotifyEvent) *Dir {
	for _, dir := range w.watchedDirs {
		if dir.wd == int(event.Wd) {
			return dir
		}
	}
	return nil
}

func (w *watcher) unwatchDir(event inotifyEvent) {
	watchedDirsAfter := make([]*Dir, 0, len(w.watchedDirs)-1)
	for _, existing := range w.watchedDirs {
		if existing.wd != int(event.Wd) {
			watchedDirsAfter = append(watchedDirsAfter, existing)
		}
	}
	w.watchedDirs = watchedDirsAfter
}

func (w *watcher) processEvent(event inotifyEvent, log logrus.FieldLogger) Error {
	dir := w.findDir(event)
	if dir == nil {
		return NewError(NotSpecified, nil, "watch list inconsistent: received a file system event for an unknown directory")
	}
	log.WithField("directory", dir.path).Debugf("received event: %v", event)
	if event.Mask&syscall.IN_IGNORED == syscall.IN_IGNORED {
		w.unwatchDir(event) // need to remove it from watchedDirs, because otherwise we close the removed dir on shutdown which causes an error
		return NewErrorf(NotSpecified, nil, "%s: directory was removed while being watched", dir.path)
	}
	if event.Mask&syscall.IN_MODIFY == syscall.IN_MODIFY {
		file, ok := w.watchedFiles[filepath.Join(dir.path, event.Name)]
		if !ok {
			return nil // unrelated file was modified
		}
		truncated, err := isTruncated(file.file)
		if err != nil {
			return NewErrorf(NotSpecified, err, "%v: seek() or stat() failed", file.file.Name())
		}
		if truncated {
			_, err = file.file.Seek(0, io.SeekStart)
			if err != nil {
				return NewErrorf(NotSpecified, err, "%v: seek() failed", file.file.Name())
			}
			file.reader.Clear()
		}
		readErr := w.readNewLines(file, log)
		if readErr != nil {
			return readErr
		}
	}
	if event.Mask&syscall.IN_MOVED_FROM == syscall.IN_MOVED_FROM || event.Mask&syscall.IN_DELETE == syscall.IN_DELETE || event.Mask&syscall.IN_CREATE == syscall.IN_CREATE || event.Mask&syscall.IN_MOVED_TO == syscall.IN_MOVED_TO {
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

func isTruncated(file *os.File) (bool, error) {
	currentPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return false, err
	}
	fileInfo, err := file.Stat()
	if err != nil {
		return false, err
	}
	return currentPos > fileInfo.Size(), nil
}

func (w *watcher) findSameFile(file os.FileInfo, _ string) (*fileWithReader, Error) {
	var (
		fileInfo os.FileInfo
		err      error
	)
	for _, watchedFile := range w.watchedFiles {
		fileInfo, err = watchedFile.file.Stat()
		if err != nil {
			return nil, NewErrorf(NotSpecified, err, "%v: stat failed", watchedFile.file.Name())
		}
		if os.SameFile(fileInfo, file) {
			return watchedFile, nil
		}
	}
	return nil, nil
}
