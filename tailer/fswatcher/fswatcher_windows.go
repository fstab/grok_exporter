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
	watchedDirs  []string
	watchedFiles map[string]*fileWithReader // path -> fileWithReader
	winWatcher   *winfsnotify.Watcher
	lines        chan Line
	errors       chan Error
	done         chan struct{}
}

type fileWithReader struct {
	file   *os.File
	reader *lineReader
}

type fileInfo struct {
	filename string
	ffd      syscall.Win32finddata
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

func Run(globs []glob.Glob, readall bool, failOnMissingFile bool, log logrus.FieldLogger) (FSWatcher, error) {
	var (
		w   *watcher
		err error
		Err Error
	)

	w, Err = initWatcher(globs)
	if Err != nil {
		return nil, Err
	}

	go func() {
		defer func() {

			close(w.lines)
			close(w.errors)

			warnf := func(format string, args ...interface{}) {
				log.Warnf("error while shutting down the file system watcher: %v", fmt.Sprint(format, args))
			}

			err = w.winWatcher.Close()
			if err != nil {
				warnf("failed to close winfsnotify.Watcher: %v", err)
			}

			for _, file := range w.watchedFiles {
				err = file.file.Close()
				if err != nil {
					warnf("close(%q) failed: %v", file.file.Name(), err)
				}
			}
		}()

		Err = w.watchDirs(log)
		if Err != nil {
			select {
			case <-w.done:
			case w.errors <- Err:
			}
			return
		}

		for _, dirPath := range w.watchedDirs {
			dirLogger := log.WithField("directory", dirPath)
			dirLogger.Debugf("initializing directory")
			Err = w.syncFilesInDir(dirPath, readall, dirLogger)
			if Err != nil {
				select {
				case <-w.done:
				case w.errors <- Err:
					return
				}
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

		for { // event consumer loop
			select {
			case <-w.done:
				return
			case event := <-w.winWatcher.Event:
				processEventError := w.processEvent(event, log)
				if processEventError != nil {
					select {
					case <-w.done:
					case w.errors <- processEventError:
					}
					return
				}
			case err := <-w.winWatcher.Error:
				select {
				case <-w.done:
				case w.errors <- NewError(NotSpecified, err, "error reading file system events"):
				}
				return
			}
		}

	}()
	return w, nil
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
		w.watchedDirs = append(w.watchedDirs, dirPath)
	}
	return nil
}

func (w *watcher) syncFilesInDir(dirPath string, readall bool, log logrus.FieldLogger) Error {
	var (
		newFile           *os.File
		watchedFilesAfter = make(map[string]*fileWithReader)
		fileInfos         []*fileInfo
		Err               Error
		err               error
	)
	fileInfos, Err = ls(dirPath)
	if Err != nil {
		return Err
	}
	for _, fileInfo := range fileInfos {
		filePath := filepath.Join(dirPath, fileInfo.filename)
		fileLogger := log.WithField("file", fileInfo.filename)
		if !anyGlobMatches(w.globs, filePath) {
			fileLogger.Debug("skipping file, because no glob matches")
			continue
		}
		if fileInfo.ffd.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY == syscall.FILE_ATTRIBUTE_DIRECTORY {
			fileLogger.Debug("skipping, because it is a directory")
			continue
		}
		newFile, Err = openFile(filePath)
		if Err != nil {
			return Err
		}
		// TODO: Maybe call syscall.GetFileType(newFile.fileHandle) and skip files other than FILE_TYPE_DISK
		existingFilePath, Err := w.findSameFile(newFile)
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
			newFile.Close()
			continue
		}
		if !readall {
			_, err = newFile.Seek(0, io.SeekEnd)
			if err != nil {
				newFile.Close()
				return NewError(NotSpecified, os.NewSyscallError("seek", err), filePath)
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

// https://docs.microsoft.com/en-us/windows/desktop/FileIO/listing-the-files-in-a-directory
func ls(dirPath string) ([]*fileInfo, Error) {
	var (
		ffd      syscall.Win32finddata
		handle   syscall.Handle
		result   []*fileInfo
		filename string
		err      error
	)
	globAll := dirPath + `\*`
	globAllP, err := syscall.UTF16PtrFromString(globAll)
	if err != nil {
		return nil, NewErrorf(NotSpecified, os.NewSyscallError("UTF16PtrFromString", err), "%v: invalid directory name", dirPath)
	}
	for handle, err = syscall.FindFirstFile(globAllP, &ffd); err == nil; err = syscall.FindNextFile(handle, &ffd) {
		filename = syscall.UTF16ToString(ffd.FileName[:])
		if filename != "." && filename != ".." {
			result = append(result, &fileInfo{
				filename: filename,
				ffd:      ffd,
			})
		}
	}
	if err != syscall.ERROR_NO_MORE_FILES {
		return nil, NewErrorf(NotSpecified, err, "%v: failed to read directory", dirPath)
	}
	return result, nil
}

func (w *watcher) processEvent(event *winfsnotify.Event, log logrus.FieldLogger) Error {
	dirPath, fileName := w.dirAndFile(event.Name)
	if len(dirPath) == 0 {
		return NewError(NotSpecified, nil, "watch list inconsistent: received a file system event for an unknown directory")
	}
	log.WithField("directory", dirPath).Debugf("received event: %v", event)
	if event.Mask&winfsnotify.FS_MODIFY == winfsnotify.FS_MODIFY {
		file, ok := w.watchedFiles[filepath.Join(dirPath, fileName)]
		if !ok {
			return nil // unrelated file was modified
		}
		truncated, Err := isTruncated(file.file)
		if Err != nil {
			return Err
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

func (w *watcher) findSameFile(newFile *os.File) (string, Error) {
	newFileStat, err := newFile.Stat()
	if err != nil {
		return "", NewError(NotSpecified, os.NewSyscallError("stat", err), newFile.Name())
	}
	for watchedFilePath, watchedFile := range w.watchedFiles {
		watchedFileStat, err := watchedFile.file.Stat()
		if err != nil {
			return "", NewError(NotSpecified, os.NewSyscallError("stat", err), watchedFilePath)
		}
		if os.SameFile(watchedFileStat, newFileStat) {
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

func (w *watcher) dirAndFile(fileOrDir string) (string, string) {
	var (
		dirPath  = ""
		fileName = ""
	)
	for _, dir := range w.watchedDirs {
		if strings.HasPrefix(fileOrDir, dir) && len(dir) > len(dirPath) {
			dirPath = dir
			fileName = strings.TrimLeft(fileOrDir[len(dirPath):], "\\/")
		}
	}
	return dirPath, fileName
}

// Don't use os.Open(), because we want to set the Windows file share flags.
func openFile(filePath string) (*os.File, Error) {
	var (
		filePathPtr *uint16
		fileHandle  syscall.Handle
		err         error
	)
	filePathPtr, err = syscall.UTF16PtrFromString(filePath)
	if err != nil {
		return nil, NewErrorf(NotSpecified, os.NewSyscallError("UTF16PtrFromString", err), "%q: illegal file name", filePath)
	}
	fileHandle, err = syscall.CreateFile(
		filePathPtr,
		syscall.GENERIC_READ,
		uint32(syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE),
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0)
	if err != nil {
		if err == syscall.ERROR_FILE_NOT_FOUND {
			return nil, NewError(FileNotFound, os.NewSyscallError("CreateFile", err), filePath)
		} else {
			return nil, NewErrorf(NotSpecified, os.NewSyscallError("CreateFile", err), "%q: cannot open file", filePath)
		}
	}
	return os.NewFile(uintptr(fileHandle), filePath), nil
}
