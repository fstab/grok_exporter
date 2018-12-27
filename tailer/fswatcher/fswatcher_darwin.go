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
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// how will this eventually be configured in the config file:
//
// * input section may specify multiple inputs and use globs
//
// * metrics may define filters to specify which files they apply to:
//   - filename_filter: filter file names, like *server1*
//   - filepath_filter: filter path, like /logs/server1/*
// Heads up: filters use globs while matches use regular expressions.
// Moreover, we should provide vars {{.filename}} and {{.filepath}} for labels.

var logger2 *logrus.Logger

func init() {
	logger2 = logrus.New()
	logger2.Level = logrus.DebugLevel
}

type watcher struct {
	globs        []string
	watchedDirs  []*os.File
	watchedFiles []*fileWithReader
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

func Run(globs []string, readall bool, failOnMissingFile bool) (FSWatcher, error) {

	var (
		w   *watcher
		err error
	)

	// Initializing directory watches happens in the main thread, so we fail immediately if the directories cannot be watched.
	w, err = initDirs(globs)
	if err != nil {
		return nil, err
	}

	go func() {

		// Initializing watches for the files within the directories happens in the goroutine, because with readall=true
		// this will immediately write lines to the lines channel, so this blocks until the caller starts reading from the lines channel.
		for _, dir := range w.watchedDirs {
			dirLogger := logger2.WithField("directory", dir.Name())
			dirLogger.Debugf("initializing directory")
			syncFilesErr := w.syncFilesInDir(dir, readall, dirLogger)
			if syncFilesErr != nil {
				select {
				case <-w.done:
				case w.errors <- syncFilesErr:
				}
				return
			}
		}

		// make sure at least one logfile was found for each glob
		if failOnMissingFile {
			missingFileError := w.checkMissingFile()
			if missingFileError != nil {
				select {
				case <-w.done:
				case w.errors <- missingFileError:
				}
				return
			}
		}

		keventProducerLoop := runKeventLoop(w.kq)
		defer keventProducerLoop.Close()

		for { // kevent consumer loop
			select {
			case <-w.done:
				return
			case event := <-keventProducerLoop.events:
				processEventError := w.processEvent(event, logger2)
				if processEventError != nil {
					select {
					case <-w.done:
					case w.errors <- processEventError:
					}
					return
				}
			case err := <-keventProducerLoop.errors:
				select {
				case <-w.done:
				case w.errors <- err:
				}
				return
			}
		}
	}()
	return w, nil
}

func (w *watcher) Close() {

	// Stop the kevent consumer loop first.
	// When the consumer loop terminates, the producer loop will automatically be closed, because we called "defer keventProducerLoop.Close()" above.
	// By closing the consumer first, we make sure that the consumer never reads from a closed events or errors channel.
	close(w.done)
	// it's now safe to close lines and errors, because we will not write to these channels if the done channel is closed.
	close(w.lines)
	close(w.errors)

	for _, file := range w.watchedFiles {
		file.file.Close()
	}

	for _, dir := range w.watchedDirs {
		dir.Close()
	}
}

func initDirs(globs []string) (*watcher, error) {
	var (
		w = &watcher{
			globs:  globs,
			lines:  make(chan Line),
			errors: make(chan Error),
			done:   make(chan struct{}),
		}
		err         error
		dir         *os.File
		dirPaths    []string
		dirPath     string
		zeroTimeout = syscall.NsecToTimespec(0) // timeout zero means non-blocking kevent() call
	)
	w.kq, err = syscall.Kqueue()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize file system watcher: %v", err)
	}
	dirPaths, err = uniqueBaseDirs(globs)
	if err != nil {
		w.Close()
		return nil, err
	}
	for _, dirPath = range dirPaths {
		dir, err = os.Open(dirPath)
		if err != nil {
			w.Close()
			return nil, err
		}
		w.watchedDirs = append(w.watchedDirs, dir)
		_, err = syscall.Kevent(w.kq, []syscall.Kevent_t{makeEvent(dir)}, nil, &zeroTimeout)
		if err != nil {
			w.Close()
			return nil, err
		}
	}
	return w, nil
}

// check if files have been added/removed and update kevent file watches accordingly
func (w *watcher) syncFilesInDir(dir *os.File, readall bool, log logrus.FieldLogger) Error {
	var (
		existingFile      *fileWithReader
		newFile           *os.File
		newFileWithReader *fileWithReader
		watchedFilesAfter []*fileWithReader
		fileInfos         []os.FileInfo
		fileInfo          os.FileInfo
		err               error
		tailerErr         Error
		fileLogger        logrus.FieldLogger
		zeroTimeout       = syscall.NsecToTimespec(0) // timeout zero means non-blocking kevent() call
	)
	fileInfos, err = repeatableReaddir(dir)
	if err != nil {
		return NewErrorf(NotSpecified, err, "%v: readdir failed", dir.Name())
	}
	watchedFilesAfter = make([]*fileWithReader, 0, len(w.watchedFiles))
	for _, fileInfo = range fileInfos {
		fullPath := filepath.Join(dir.Name(), fileInfo.Name())
		fileLogger = log.WithField("file", fullPath)
		if !anyGlobMatches(w.globs, fullPath) {
			fileLogger.Debug("skipping file, because no glob matches")
			continue
		}
		if fileInfo.IsDir() {
			fileLogger.Debug("skipping, because it is a directory")
			continue
		}
		existingFile, tailerErr = findSameFile(w.watchedFiles, fileInfo)
		if tailerErr != nil {
			return tailerErr
		}
		if existingFile != nil {
			if existingFile.file.Name() != fullPath {
				fileLogger.WithField("fd", existingFile.file.Fd()).Infof("file was moved from %v", existingFile.file.Name())
				existingFile.file = os.NewFile(existingFile.file.Fd(), fullPath)
			} else {
				fileLogger.Debug("skipping, because file is already watched")
			}
			watchedFilesAfter = append(watchedFilesAfter, existingFile)
			continue
		}
		newFile, err = os.Open(fullPath)
		if err != nil {
			return NewErrorf(NotSpecified, err, "%v: failed to open file", fullPath)
		}
		if !readall {
			_, err = newFile.Seek(0, io.SeekEnd)
			if err != nil {
				return NewErrorf(NotSpecified, err, "%v: failed to seek to end of file", fullPath)
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
		tailerErr = w.readNewLines(newFileWithReader, fileLogger)
		if tailerErr != nil {
			return tailerErr
		}
		watchedFilesAfter = append(watchedFilesAfter, newFileWithReader)
	}
	for _, f := range w.watchedFiles {
		if !contains(watchedFilesAfter, f) {
			fileLogger = log.WithField("file", f.file.Name()).WithField("fd", f.file.Fd())
			fileLogger.Info("file was removed, closing and un-watching")
			f.file.Close()
		}
	}
	w.watchedFiles = watchedFilesAfter
	return nil
}

func (w *watcher) processEvent(kevent syscall.Kevent_t, log logrus.FieldLogger) Error {
	var (
		dir                   *os.File
		file                  *fileWithReader
		dirLogger, fileLogger logrus.FieldLogger
	)
	for _, dir = range w.watchedDirs {
		if kevent.Ident == fdToInt(dir.Fd()) {
			dirLogger = log.WithField("directory", dir.Name())
			dirLogger.Debugf("dir event with fflags %v", fflags2string(kevent))
			return w.processDirEvent(kevent, dir, dirLogger)
		}
	}
	for _, file = range w.watchedFiles {
		if kevent.Ident == fdToInt(file.file.Fd()) {
			fileLogger = log.WithField("file", file.file.Name()).WithField("fd", file.file.Fd())
			fileLogger.Debugf("file event with fflags %v", fflags2string(kevent))
			return w.processFileEvent(kevent, file, fileLogger)
		}
	}
	// Events for unknown file descriptors are ignored. This might happen if syncFilesInDir() already
	// closed a file while a pending event is still coming in.
	log.Debugf("event for unknown file descriptor %v with fflags %v", kevent.Ident, fflags2string(kevent))
	return nil
}

func (w *watcher) processDirEvent(kevent syscall.Kevent_t, dir *os.File, dirLogger logrus.FieldLogger) Error {
	if kevent.Fflags&syscall.NOTE_WRITE == syscall.NOTE_WRITE || kevent.Fflags&syscall.NOTE_EXTEND == syscall.NOTE_EXTEND {
		// NOTE_WRITE on the directory's fd means a file was created, deleted, or moved. This covers inotify's MOVED_TO.
		// NOTE_EXTEND reports that a directory entry was added	or removed as the result of rename operation.
		dirLogger.Debugf("checking for new/deleted/moved files")
		err := w.syncFilesInDir(dir, true, dirLogger)
		if err != nil {
			return NewErrorf(NotSpecified, err, "%v: failed to update list of files in directory", dir.Name())
		}
	}
	if kevent.Fflags&syscall.NOTE_DELETE == syscall.NOTE_DELETE {
		return NewErrorf(NotSpecified, nil, "%v: directory was deleted", dir.Name())
	}
	if kevent.Fflags&syscall.NOTE_RENAME == syscall.NOTE_RENAME {
		return NewErrorf(NotSpecified, nil, "%v: directory was moved", dir.Name())
	}
	if kevent.Fflags&syscall.NOTE_REVOKE == syscall.NOTE_REVOKE {
		return NewErrorf(NotSpecified, nil, "%v: filesystem was unmounted", dir.Name())
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
	for _, glob := range w.globs {
		for _, watchedFile := range w.watchedFiles {
			if match, _ := filepath.Match(glob, watchedFile.file.Name()); match {
				continue OUTER
			}
		}
		return NewErrorf(FileNotFound, nil, "%v: no such file", glob)
	}
	return nil
}

// gets the base directories from the glob expressions,
// makes sure the paths exist and point to directories.
func uniqueBaseDirs(globs []string) ([]string, error) {
	var (
		result  = make([]string, 0, len(globs))
		dirPath string
		err     error
		errMsg  string
		g       string
	)
	for _, g = range globs {
		dirPath, err = filepath.Abs(filepath.Dir(g))
		if err != nil {
			return nil, fmt.Errorf("%q: failed to determine absolute path: %v", filepath.Dir(g), err)
		}
		if containsString(result, dirPath) {
			continue
		}
		dirInfo, err := os.Stat(dirPath)
		if err != nil {
			if os.IsNotExist(err) {
				errMsg = fmt.Sprintf("%v: no such directory", dirPath)
				if strings.Contains(dirPath, "*") || strings.Contains(dirPath, "?") || strings.Contains(dirPath, "[") {
					return nil, fmt.Errorf("%v: note that wildcards are only supported for files but not for directories", errMsg)
				} else {
					return nil, errors.New(errMsg)
				}
			}
			return nil, err
		}
		if !dirInfo.IsDir() {
			return nil, fmt.Errorf("%v is not a directory", dirPath)
		}
		result = append(result, dirPath)
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

func anyGlobMatches(globs []string, path string) bool {
	for _, pattern := range globs {
		if match, _ := filepath.Match(pattern, path); match {
			return true
		}
	}
	return false
}

func findSameFile(watchedFiles []*fileWithReader, other os.FileInfo) (*fileWithReader, Error) {
	var (
		fileInfo os.FileInfo
		err      error
	)
	for _, watchedFile := range watchedFiles {
		fileInfo, err = watchedFile.file.Stat()
		if err != nil {
			return nil, NewErrorf(NotSpecified, err, "%v: stat failed", watchedFile.file.Name())
		}
		if os.SameFile(fileInfo, other) {
			return watchedFile, nil
		}
	}
	return nil, nil
}

func repeatableReaddir(f *os.File) ([]os.FileInfo, error) {
	defer f.Seek(0, io.SeekStart)
	return f.Readdir(-1)
}

func containsString(list []string, s string) bool {
	for _, existing := range list {
		if existing == s {
			return true
		}
	}
	return false
}

func contains(list []*fileWithReader, f *fileWithReader) bool {
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

func fflags2string(event syscall.Kevent_t) string {
	result := make([]string, 0, 1)
	if event.Fflags&syscall.NOTE_DELETE == syscall.NOTE_DELETE {
		result = append(result, "NOTE_DELETE")
	}
	if event.Fflags&syscall.NOTE_WRITE == syscall.NOTE_WRITE {
		result = append(result, "NOTE_WRITE")
	}
	if event.Fflags&syscall.NOTE_EXTEND == syscall.NOTE_EXTEND {
		result = append(result, "NOTE_EXTEND")
	}
	if event.Fflags&syscall.NOTE_ATTRIB == syscall.NOTE_ATTRIB {
		result = append(result, "NOTE_ATTRIB")
	}
	if event.Fflags&syscall.NOTE_LINK == syscall.NOTE_LINK {
		result = append(result, "NOTE_LINK")
	}
	if event.Fflags&syscall.NOTE_RENAME == syscall.NOTE_RENAME {
		result = append(result, "NOTE_RENAME")
	}
	if event.Fflags&syscall.NOTE_REVOKE == syscall.NOTE_REVOKE {
		result = append(result, "NOTE_REVOKE")
	}
	return strings.Join(result, ", ")
}
