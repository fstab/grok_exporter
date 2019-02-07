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
	watchedDirs  map[int]string             // watch descriptor -> path
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
			watchedDirs:  make(map[int]string),
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
		w.watchedDirs[wd] = dirPath
	}
	return nil
}

func (w *watcher) syncFilesInDir(dirPath string, readall bool, log logrus.FieldLogger) Error {
	var (
		watchedFilesAfter = make(map[string]*fileWithReader)
		fileInfos         []os.FileInfo
		Err               Error
	)
	fileInfos, Err = ls(dirPath)
	if Err != nil {
		return Err
	}
	for _, fileInfo := range fileInfos {
		filePath := filepath.Join(dirPath, fileInfo.Name())
		fileLogger := log.WithField("file", fileInfo.Name())
		if !anyGlobMatches(w.globs, filePath) {
			fileLogger.Debug("skipping file, because file name does not match")
			continue
		}
		if fileInfo.IsDir() {
			fileLogger.Debug("skipping, because it is a directory")
			continue
		}
		existingFilePath, Err := w.findSameFile(fileInfo)
		if Err != nil {
			return Err
		}
		if len(existingFilePath) > 0 {
			existingFile := w.watchedFiles[existingFilePath]
			if existingFilePath != filePath {
				fileLogger.WithField("fd", existingFile.file.Fd()).Infof("file was moved from %v", existingFilePath)
				existingFile.file = os.NewFile(existingFile.file.Fd(), filePath)
			} else {
				fileLogger.Debug("skipping, because file is already watched")
			}
			watchedFilesAfter[filePath] = existingFile
			continue
		}
		newFile, err := os.Open(filePath)
		if err != nil {
			return NewErrorf(NotSpecified, err, "%v: failed to open file", newFile)
		}
		if !readall {
			_, err = newFile.Seek(0, io.SeekEnd)
			if err != nil {
				newFile.Close()
				return NewErrorf(NotSpecified, err, "%v: failed to seek to end of file", filePath)
			}
		}
		fileLogger = fileLogger.WithField("fd", newFile.Fd())
		fileLogger.Info("watching new file")
		newFileWithReader := &fileWithReader{file: newFile, reader: NewLineReader()}
		Err = w.readNewLines(newFileWithReader, fileLogger)
		if Err != nil {
			return Err
		}
		watchedFilesAfter[filePath] = newFileWithReader
	}
	for _, f := range w.watchedFiles {
		if !contains(watchedFilesAfter, f) {
			fileLogger := log.WithField("file", filepath.Base(f.file.Name())).WithField("fd", f.file.Fd())
			fileLogger.Info("file was removed, closing and un-watching")
			f.file.Close()
		}
	}
	w.watchedFiles = watchedFilesAfter
	return nil
}

// TODO: Replace with ioutil.Readdir
func ls(dirPath string) ([]os.FileInfo, Error) {
	var (
		dir       *os.File
		fileInfos []os.FileInfo
		err       error
	)
	dir, err = os.Open(dirPath)
	if err != nil {
		return nil, NewErrorf(NotSpecified, err, "%q: failed to open directory", dirPath)
	}
	defer dir.Close()
	fileInfos, err = dir.Readdir(-1)
	if err != nil {
		return nil, NewErrorf(NotSpecified, err, "%q: failed to read directory", dirPath)
	}
	return fileInfos, nil
}

func (w *watcher) processEvent(event inotifyEvent, log logrus.FieldLogger) Error {
	dirPath, ok := w.watchedDirs[int(event.Wd)]
	if !ok {
		return NewError(NotSpecified, nil, "watch list inconsistent: received a file system event for an unknown directory")
	}
	log.WithField("directory", dirPath).Debugf("received event: %v", event)
	if event.Mask&syscall.IN_IGNORED == syscall.IN_IGNORED {
		delete(w.watchedDirs, int(event.Wd))
		return NewErrorf(NotSpecified, nil, "%s: directory was removed while being watched", dirPath)
	}
	if event.Mask&syscall.IN_MODIFY == syscall.IN_MODIFY {
		file, ok := w.watchedFiles[filepath.Join(dirPath, event.Name)]
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
		err := w.syncFilesInDir(dirPath, true, log)
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

func (w *watcher) findSameFile(file os.FileInfo) (string, Error) {
	var (
		fileInfo os.FileInfo
		err      error
	)
	for watchedFilePath, watchedFile := range w.watchedFiles {
		fileInfo, err = watchedFile.file.Stat()
		if err != nil {
			return "", NewErrorf(NotSpecified, err, "%v: stat failed", watchedFile.file.Name())
		}
		if os.SameFile(fileInfo, file) {
			return watchedFilePath, nil
		}
	}
	return "", nil
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
