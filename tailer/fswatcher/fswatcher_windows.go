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
	"golang.org/x/exp/winfsnotify"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type watcher struct {
	winWatcher *winfsnotify.Watcher
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

func (w *watcher) unwatchDir(dir *Dir) error {
	return nil
}

func (w *watcher) Close() error {
	err := w.winWatcher.Close()
	if err != nil {
		return fmt.Errorf("failed to close winfsnotify.Watcher: %v", err)
	} else {
		return nil
	}
}

func (w *watcher) runFseventProducerLoop() fseventProducerLoop {
	return runWinWatcherLoop(w.winWatcher)
}

func initWatcher() (fswatcher, Error) {
	winWatcher, err := winfsnotify.NewWatcher()
	if err != nil {
		return nil, NewError(NotSpecified, err, "failed to initialize file system watcher")
	}
	return &watcher{winWatcher: winWatcher}, nil
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
	err = w.winWatcher.Watch(path)
	if err != nil {
		return nil, NewErrorf(NotSpecified, err, "%v: failed to watch directory", path)
	}
	return dir, nil
}

func newDir(path string) (*Dir, Error) {
	return &Dir{path: path}, nil
}

func (w *watcher) watchFile(_ fileMeta) Error {
	// nothing to do, because on Windows we watch the directory and don't need to watch individual files.
	return nil
}

func (w *watcher) processEvent(t *fileTailer, fsevent fsevent, log logrus.FieldLogger) Error {
	event, ok := fsevent.(*winfsnotify.Event)
	if !ok {
		return NewErrorf(NotSpecified, nil, "received a file system event of unknown type %T", event)
	}

	dir, fileName := dirAndFile(t, event.Name)
	if dir == nil {
		return NewError(NotSpecified, nil, "watch list inconsistent: received a file system event for an unknown directory")
	}
	log.WithField("directory", dir.path).Debugf("received event: %v", event)
	if event.Mask&winfsnotify.FS_MODIFY == winfsnotify.FS_MODIFY {
		file, ok := t.watchedFiles[filepath.Join(dir.path, fileName)]
		if !ok {
			return nil // unrelated file was modified
		}
		truncated, Err := file.file.CheckTruncated()
		if Err != nil {
			if Err.Type() == WinFileRemoved {
				return t.syncFilesInDir(dir, true, log)
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
		Err = t.readNewLines(file, log)
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
		err := t.syncFilesInDir(dir, true, log)
		if err != nil {
			return err
		}
	}
	return nil
}

func isTruncated(file *File) (bool, Error) {
	return file.CheckTruncated()
}

func findSameFile(t *fileTailer, newFileInfo *fileInfo, path string) (*fileWithReader, Error) {
	newFile, Err := open(path)
	if Err != nil {
		if Err.Type() == FileNotFound {
			return nil, nil
		} else {
			return nil, Err
		}
	}
	defer newFile.Close()
	for _, watchedFile := range t.watchedFiles {
		if watchedFile.file.SameFile(newFile) {
			return watchedFile, nil
		}
	}
	return nil, nil
}

func dirAndFile(t *fileTailer, fileOrDir string) (*Dir, string) {
	var (
		found *Dir
	)
	for _, dir := range t.watchedDirs {
		if strings.HasPrefix(fileOrDir, dir.path) {
			if found == nil || len(dir.path) > len(found.path) {
				found = dir
			}
		}
	}
	return found, strings.TrimLeft(fileOrDir[len(found.path):], "\\/")
}
