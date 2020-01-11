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
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"syscall"
)

type watcher struct {
	kq int
}

type fileWithReader struct {
	file   *os.File
	reader *lineReader
}

func (w *watcher) unwatchDir(dir *Dir) error {
	err := dir.file.Close()
	if err != nil {
		return fmt.Errorf("close(%q) failed: %v", dir.file.Name(), err)
	} else {
		return nil
	}
}

func (w *watcher) Close() error {
	// After calling keventProducerLoop.Close(), we need to close the kq descriptor
	// in order to interrupt the kevent() system call. See keventProducerLoop.Close().
	err := syscall.Close(w.kq)
	if err != nil {
		return fmt.Errorf("closing the kevent file descriptor failed: %v", err)
	} else {
		return nil
	}
}

func (w *watcher) runFseventProducerLoop() fseventProducerLoop {
	return runKeventLoop(w.kq)
}

func initWatcher() (fswatcher, Error) {
	kq, err := syscall.Kqueue()
	if err != nil {
		return nil, NewError(NotSpecified, err, "kqueue() failed")
	}
	return &watcher{kq: kq}, nil
}

func (w *watcher) watchDir(path string) (*Dir, Error) {
	var (
		dir         *Dir
		err         error
		Err         Error
		zeroTimeout = syscall.NsecToTimespec(0) // timeout zero means non-blocking kevent() call
	)
	dir, Err = newDir(path)
	if Err != nil {
		return nil, Err
	}
	_, err = syscall.Kevent(w.kq, []syscall.Kevent_t{makeEvent(dir.file)}, nil, &zeroTimeout)
	if err != nil {
		dir.file.Close()
		return nil, NewErrorf(NotSpecified, err, "%v: kevent() failed", path)
	}
	return dir, nil
}

func newDir(path string) (*Dir, Error) {
	var (
		err error
		dir *os.File
	)
	dir, err = os.Open(path)
	if err != nil {
		return nil, NewErrorf(NotSpecified, err, "%v: open() failed", path)
	}
	return &Dir{dir}, nil
}

func (w *watcher) watchFile(newFile fileMeta) Error {
	zeroTimeout := syscall.NsecToTimespec(0) // timeout zero means non-blocking kevent() call
	_, err := syscall.Kevent(w.kq, []syscall.Kevent_t{makeEvent(newFile)}, nil, &zeroTimeout)
	if err != nil {
		return NewErrorf(NotSpecified, err, "%v: failed to watch file", newFile.Name())
	}
	return nil
}

func (w *watcher) processEvent(t *fileTailer, event fsevent, log logrus.FieldLogger) Error {
	var (
		dir                   *Dir
		file                  *fileWithReader
		dirLogger, fileLogger logrus.FieldLogger
		kevent                syscall.Kevent_t
		ok                    bool
	)

	kevent, ok = event.(syscall.Kevent_t)
	if !ok {
		return NewErrorf(NotSpecified, nil, "received a file system event of unknown type %T", event)
	}

	for _, dir = range t.watchedDirs {
		if kevent.Ident == fdToInt(dir.file.Fd()) {
			dirLogger = log.WithField("directory", dir.file.Name())
			dirLogger.Debugf("dir event: %v", event2string(kevent))
			return w.processDirEvent(t, kevent, dir, dirLogger)
		}
	}
	for _, file = range t.watchedFiles {
		if kevent.Ident == fdToInt(file.file.Fd()) {
			fileLogger = log.WithField("file", file.file.Name()).WithField("fd", file.file.Fd())
			fileLogger.Debugf("file event: %v", event2string(kevent))
			return w.processFileEvent(t, kevent, file, fileLogger)
		}
	}
	// Events for unknown file descriptors are ignored. This might happen if syncFilesInDir() already
	// closed a file while a pending event is still coming in.
	log.Debugf("event for unknown file descriptor %v: %v", kevent.Ident, event2string(kevent))
	return nil
}

func (w *watcher) processDirEvent(t *fileTailer, kevent syscall.Kevent_t, dir *Dir, dirLogger logrus.FieldLogger) Error {
	if kevent.Fflags&syscall.NOTE_WRITE == syscall.NOTE_WRITE || kevent.Fflags&syscall.NOTE_EXTEND == syscall.NOTE_EXTEND {
		// NOTE_WRITE on the directory's fd means a file was created, deleted, or moved. This covers inotify's MOVED_TO.
		// NOTE_EXTEND reports that a directory entry was added	or removed as the result of rename operation.
		dirLogger.Debugf("checking for new/deleted/moved files")
		err := t.syncFilesInDir(dir, true, dirLogger)
		if err != nil {
			return NewErrorf(NotSpecified, err, "%v: failed to update list of files in directory", dir.file.Name())
		}
	}
	if kevent.Fflags&syscall.NOTE_DELETE == syscall.NOTE_DELETE {
		return NewErrorf(NotSpecified, nil, "%v: directory was deleted", dir.file.Name())
	}
	if kevent.Fflags&syscall.NOTE_RENAME == syscall.NOTE_RENAME {
		return NewErrorf(NotSpecified, nil, "%v: directory was moved", dir.file.Name())
	}
	if kevent.Fflags&syscall.NOTE_REVOKE == syscall.NOTE_REVOKE {
		return NewErrorf(NotSpecified, nil, "%v: filesystem was unmounted", dir.file.Name())
	}
	// NOTE_LINK (sub directory created) and NOTE_ATTRIB (attributes changed) are ignored.
	return nil
}

func (w *watcher) processFileEvent(t *fileTailer, kevent syscall.Kevent_t, file *fileWithReader, log logrus.FieldLogger) Error {
	var (
		truncated bool
		err       error
		readErr   Error
	)

	// Handle truncate events.
	if kevent.Fflags&syscall.NOTE_ATTRIB == syscall.NOTE_ATTRIB {
		truncated, err = isTruncated(file.file)
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
	}

	// Handle write event.
	if kevent.Fflags&syscall.NOTE_WRITE == syscall.NOTE_WRITE {
		readErr = t.readNewLines(file, log)
		if readErr != nil {
			return readErr
		}
	}

	// Handle move and delete events (NOTE_RENAME on the file's fd means the file was moved away, like in inotify's IN_MOVED_FROM).
	if kevent.Fflags&syscall.NOTE_DELETE == syscall.NOTE_DELETE || kevent.Fflags&syscall.NOTE_RENAME == syscall.NOTE_RENAME {
		// File deleted or moved away. Ignoring, because this will also trigger a NOTE_WRITE event on the directory, and we update the list of watched files there.
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

type withFd interface {
	Fd() uintptr
}

func makeEvent(file withFd) syscall.Kevent_t {

	// Note about the EV_CLEAR flag:
	//
	// The NOTE_WRITE event is triggered by the first write to the file after register, and remains set.
	// This means that we continue to receive the event indefinitely.
	//
	// There are two flags to stop receiving the event over and over again:
	//
	// * EV_ONESHOT: This suppresses consecutive events of the same type. However, that means that means that
	//               we don't receive new WRITE events even if new lines are written to the file.
	//               Therefore we cannot use EV_ONESHOT.
	// * EV_CLEAR:   This resets the state after the event, so that an event is only delivered once for each write.
	//               (Actually it could be less than once per write, since events are coalesced.)
	//               This is our desired behaviour.
	//
	// See also http://benno.id.au/blog/2008/05/15/simplefilemon

	return syscall.Kevent_t{
		Ident:  fdToInt(file.Fd()),
		Filter: syscall.EVFILT_VNODE,              // File modification and deletion events
		Flags:  syscall.EV_ADD | syscall.EV_CLEAR, // Add a new event, automatically enabled unless EV_DISABLE is specified
		Fflags: syscall.NOTE_DELETE | syscall.NOTE_WRITE | syscall.NOTE_EXTEND | syscall.NOTE_ATTRIB | syscall.NOTE_LINK | syscall.NOTE_RENAME | syscall.NOTE_REVOKE,
		Data:   0,
		Udata:  nil,
	}
}
