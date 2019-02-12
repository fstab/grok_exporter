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
	"fmt"
	"github.com/fstab/grok_exporter/tailer/glob"
	"github.com/prometheus/common/log"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// ideas how this might look like in the config file:
//
// * input section may specify multiple inputs and use globs
//
// * metrics may define filters to specify which files they apply to:
//   - filename_filter: filter file names, like *server1*
//   - filepath_filter: filter path, like /logs/server1/*
// Heads up: filters use globs while matches use regular expressions.
// Moreover, we should provide vars {{.filename}} and {{.filepath}} for labels.

type watcher struct {
	globs        []glob.Glob
	watchedDirs  []*Dir
	watchedFiles map[string]*fileWithReader // path -> fileWithReader
	kq           int
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

	// After calling keventProducerLoop.Close(), we need to close the kq descriptor
	// in order to interrupt the kevent() system call. See keventProducerLoop.Close().
	err := syscall.Close(w.kq)
	if err != nil {
		warnf("closing the kq descriptor failed: %v", err)
	}

	for _, file := range w.watchedFiles {
		err = file.file.Close()
		if err != nil {
			warnf("close(%q) failed: %v", file.file.Name(), err)
		}
	}

	for _, dir := range w.watchedDirs {
		err = dir.file.Close()
		if err != nil {
			warnf("close(%q) failed: %v", dir.file.Name(), err)
		}
	}
}

func (w *watcher) runFseventProducerLoop() *keventloop {
	return runKeventLoop(w.kq)
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
	w.kq, err = syscall.Kqueue()
	if err != nil {
		return nil, NewError(NotSpecified, err, "kqueue() failed")
	}
	return w, nil
}

func (w *watcher) watchDirs(log logrus.FieldLogger) Error {
	var (
		err         error
		Err         Error
		dir         *os.File
		dirPaths    []string
		dirPath     string
		zeroTimeout = syscall.NsecToTimespec(0) // timeout zero means non-blocking kevent() call
	)
	dirPaths, Err = uniqueDirs(w.globs)
	if Err != nil {
		return Err
	}
	for _, dirPath = range dirPaths {
		log.Debugf("watching directory %v", dirPath)
		dir, err = os.Open(dirPath)
		if err != nil {
			return NewErrorf(NotSpecified, err, "%v: open() failed", dirPath)
		}
		w.watchedDirs = append(w.watchedDirs, &Dir{file: dir})
		_, err = syscall.Kevent(w.kq, []syscall.Kevent_t{makeEvent(dir)}, nil, &zeroTimeout)
		if err != nil {
			return NewErrorf(NotSpecified, err, "%v: kevent() failed", dirPath)
		}
	}
	return nil
}

// check if files have been added/removed and update kevent file watches accordingly
func (w *watcher) syncFilesInDir(dir *Dir, readall bool, log logrus.FieldLogger) Error {
	var (
		existingFile      *fileWithReader
		newFile           *os.File
		newFileWithReader *fileWithReader
		watchedFilesAfter = make(map[string]*fileWithReader)
		fileInfos         []os.FileInfo
		fileInfo          os.FileInfo
		err               error
		Err               Error
		fileLogger        logrus.FieldLogger
		zeroTimeout       = syscall.NsecToTimespec(0) // timeout zero means non-blocking kevent() call
	)
	fileInfos, Err = dir.ls()
	if Err != nil {
		return Err
	}
	for _, fileInfo = range fileInfos {
		filePath := filepath.Join(dir.file.Name(), fileInfo.Name())
		fileLogger = log.WithField("file", fileInfo.Name())
		if !anyGlobMatches(w.globs, filePath) {
			fileLogger.Debug("skipping file, because file name does not match")
			continue
		}
		if fileInfo.IsDir() {
			fileLogger.Debug("skipping, because it is a directory")
			continue
		}
		existingFile, Err = w.findSameFile(fileInfo)
		if Err != nil {
			return Err
		}
		if existingFile != nil {
			if existingFile.file.Name() != filePath {
				fileLogger.WithField("fd", existingFile.file.Fd()).Infof("file was moved from %v", existingFile.file.Name())
				existingFile.file = os.NewFile(existingFile.file.Fd(), filePath)
			} else {
				fileLogger.Debug("skipping, because file is already watched")
			}
			watchedFilesAfter[filePath] = existingFile
			continue
		}
		newFile, err = os.Open(filePath)
		if err != nil {
			return NewErrorf(NotSpecified, err, "%v: failed to open file", filePath)
		}
		if !readall {
			_, err = newFile.Seek(0, io.SeekEnd)
			if err != nil {
				return NewErrorf(NotSpecified, err, "%v: failed to seek to end of file", filePath)
			}
		}
		fileLogger = fileLogger.WithField("fd", newFile.Fd())
		fileLogger.Info("watching new file")
		_, err = syscall.Kevent(w.kq, []syscall.Kevent_t{makeEvent(newFile)}, nil, &zeroTimeout)
		if err != nil {
			_ = newFile.Close()
			return NewErrorf(NotSpecified, err, "%v: failed to watch file", newFile.Name())
		}
		newFileWithReader = &fileWithReader{file: newFile, reader: NewLineReader()}
		Err = w.readNewLines(newFileWithReader, fileLogger)
		if Err != nil {
			return Err
		}
		watchedFilesAfter[filePath] = newFileWithReader
	}
	for _, f := range w.watchedFiles {
		if !contains(watchedFilesAfter, f) {
			fileLogger = log.WithField("file", filepath.Base(f.file.Name())).WithField("fd", f.file.Fd())
			fileLogger.Info("file was removed, closing and un-watching")
			// TODO: explicit un-watch needed, or are kevents for deleted files removed automatically?
			f.file.Close()
		}
	}
	w.watchedFiles = watchedFilesAfter
	return nil
}

func (w *watcher) processEvent(kevent syscall.Kevent_t, log logrus.FieldLogger) Error {
	var (
		dir                   *Dir
		file                  *fileWithReader
		dirLogger, fileLogger logrus.FieldLogger
	)
	for _, dir = range w.watchedDirs {
		if kevent.Ident == fdToInt(dir.file.Fd()) {
			dirLogger = log.WithField("directory", dir.file.Name())
			dirLogger.Debugf("dir event: %v", kevent)
			return w.processDirEvent(kevent, dir, dirLogger)
		}
	}
	for _, file = range w.watchedFiles {
		if kevent.Ident == fdToInt(file.file.Fd()) {
			fileLogger = log.WithField("file", file.file.Name()).WithField("fd", file.file.Fd())
			fileLogger.Debugf("file event: %v", kevent)
			return w.processFileEvent(kevent, file, fileLogger)
		}
	}
	// Events for unknown file descriptors are ignored. This might happen if syncFilesInDir() already
	// closed a file while a pending event is still coming in.
	log.Debugf("event for unknown file descriptor %v: %v", kevent.Ident, kevent)
	return nil
}

func (w *watcher) processDirEvent(kevent syscall.Kevent_t, dir *Dir, dirLogger logrus.FieldLogger) Error {
	if kevent.Fflags&syscall.NOTE_WRITE == syscall.NOTE_WRITE || kevent.Fflags&syscall.NOTE_EXTEND == syscall.NOTE_EXTEND {
		// NOTE_WRITE on the directory's fd means a file was created, deleted, or moved. This covers inotify's MOVED_TO.
		// NOTE_EXTEND reports that a directory entry was added	or removed as the result of rename operation.
		dirLogger.Debugf("checking for new/deleted/moved files")
		err := w.syncFilesInDir(dir, true, dirLogger)
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

func (w *watcher) processFileEvent(kevent syscall.Kevent_t, file *fileWithReader, log logrus.FieldLogger) Error {
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
		readErr = w.readNewLines(file, log)
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

func anyGlobMatches(globs []glob.Glob, path string) bool {
	for _, pattern := range globs {
		if pattern.Match(path) {
			return true
		}
	}
	return false
}

func (w *watcher) findSameFile(file os.FileInfo) (*fileWithReader, Error) {
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

func makeEvent(file *os.File) syscall.Kevent_t {

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
