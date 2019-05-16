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
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

type watcher struct {
	fd int
}

type fileWithReader struct {
	file   *os.File
	reader *lineReader
}

func (w *watcher) unwatchDir(dir *Dir) error {
	// After calling eventProducerLoop.Close(), we need to call inotify_rm_watch()
	// in order to terminate the inotify loop. See eventProducerLoop.Close().
	success, err := syscall.InotifyRmWatch(w.fd, uint32(dir.wd))
	if success != 0 || err != nil {
		return fmt.Errorf("inotify_rm_watch(%q) failed: status=%v, err=%v", dir.path, success, err)
	} else {
		return nil
	}
}

func (w *watcher) Close() error {
	err := syscall.Close(w.fd)
	if err != nil {
		return fmt.Errorf("failed to close the inotify file descriptor: %v", err)
	} else {
		return nil
	}
}

func unwatchDirByEvent(t *fileTailer, event inotifyEvent) {
	watchedDirsAfter := make([]*Dir, 0, len(t.watchedDirs)-1)
	for _, existing := range t.watchedDirs {
		if existing.wd != int(event.Wd) {
			watchedDirsAfter = append(watchedDirsAfter, existing)
		}
	}
	t.watchedDirs = watchedDirsAfter
}

func (w *watcher) runFseventProducerLoop() fseventProducerLoop {
	return runInotifyLoop(w.fd)
}

func initWatcher() (fswatcher, Error) {
	fd, err := syscall.InotifyInit1(syscall.IN_CLOEXEC)
	if err != nil {
		return nil, NewError(NotSpecified, err, "inotify_init1() failed")
	}
	return &watcher{fd: fd}, nil
}

func (w *watcher) watchDir(path string) (*Dir, Error) {
	var (
		dir *Dir
		err error
		Err Error
	)
	dir, Err = newDir(path)
	if Err != nil {
		return nil, Err
	}
	dir.wd, err = syscall.InotifyAddWatch(w.fd, path, syscall.IN_MODIFY|syscall.IN_MOVED_FROM|syscall.IN_MOVED_TO|syscall.IN_DELETE|syscall.IN_CREATE)
	if err != nil {
		return nil, NewErrorf(NotSpecified, err, "%q: inotify_add_watch() failed", path)
	}
	return dir, nil
}

func newDir(path string) (*Dir, Error) {
	return &Dir{path: path}, nil
}

func (w *watcher) watchFile(_ fileMeta) Error {
	// nothing to do, because on Linux we watch the directory and don't need to watch individual files.
	return nil
}

func findDir(t *fileTailer, event inotifyEvent) *Dir {
	for _, dir := range t.watchedDirs {
		if dir.wd == int(event.Wd) {
			return dir
		}
	}
	return nil
}

func (w *watcher) processEvent(t *fileTailer, fsevent fsevent, log logrus.FieldLogger) Error {
	event, ok := fsevent.(inotifyEvent)
	if !ok {
		return NewErrorf(NotSpecified, nil, "received a file system event of unknown type %T", event)
	}
	dir := findDir(t, event)
	if dir == nil {
		return NewError(NotSpecified, nil, "watch list inconsistent: received a file system event for an unknown directory")
	}
	dirLogger := log.WithField("directory", dir.path)
	dirLogger.Debugf("received event: %v", event)
	if event.Mask&syscall.IN_IGNORED == syscall.IN_IGNORED {
		unwatchDirByEvent(t, event) // need to remove it from watchedDirs, because otherwise we close the removed dir on shutdown which causes an error
		return NewErrorf(NotSpecified, nil, "%s: directory was removed while being watched", dir.path)
	}
	if event.Mask&syscall.IN_MODIFY == syscall.IN_MODIFY {
		file, ok := t.watchedFiles[filepath.Join(dir.path, event.Name)]
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
		readErr := t.readNewLines(file, dirLogger)
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
		err := t.syncFilesInDir(dir, true, dirLogger)
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

func findSameFile(t *fileTailer, file os.FileInfo, _ string) (*fileWithReader, Error) {
	var (
		fileInfo os.FileInfo
		err      error
	)
	for _, watchedFile := range t.watchedFiles {
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
