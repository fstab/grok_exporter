// Copyright 2018-2019 The grok_exporter Authors
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
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/fstab/grok_exporter/tailer/glob"
	"github.com/sirupsen/logrus"
)

type FileTailer interface {
	Lines() chan *Line
	Errors() chan Error
	Close()
}

type Line struct {
	Line  string
	File  string
	Extra interface{}
}

// ideas how this might look like in the config file:
//
// * input section may specify multiple inputs and use globs
//
// * metrics may define filters to specify which files they apply to:
//   - filename_filter: filter file names, like *server1*
//   - filepath_filter: filter path, like /logs/server1/*
// Heads up: filters use globs while matches use regular expressions.
// Moreover, we should provide vars {{.filename}} and {{.filepath}} for labels.

type fileTailer struct {
	globs        []glob.Glob
	watchedDirs  []*Dir
	watchedFiles map[string]*fileWithReader // path -> fileWithReader
	osSpecific   fswatcher
	lines        chan *Line
	errors       chan Error
	done         chan struct{}
}

type fswatcher interface {
	io.Closer
	runFseventProducerLoop() fseventProducerLoop
	processEvent(t *fileTailer, event fsevent, log logrus.FieldLogger) Error
	watchDir(path string) (*Dir, Error)
	unwatchDir(dir *Dir) error
	watchFile(file fileMeta) Error
}

type fseventProducerLoop interface {
	Close()
	Events() chan fsevent
	Errors() chan Error
}

type fsevent interface{}

type fileMeta interface {
	Fd() uintptr
	Name() string
}

func (t *fileTailer) Lines() chan *Line {
	return t.lines
}

func (t *fileTailer) Errors() chan Error {
	return t.errors
}

// Close() triggers the shutdown of the file tailer.
// The file tailer will eventually terminate,
// but after Close() returns it might still be running in the background for a few milliseconds.
func (t *fileTailer) Close() {
	// Closing the done channel will stop the consumer loop.
	// Deferred functions within the consumer loop will close the producer loop.
	close(t.done)
}

func RunFileTailer(globs []glob.Glob, readall bool, failOnMissingFile bool, log logrus.FieldLogger) (FileTailer, error) {
	return runFileTailer(initWatcher, globs, readall, failOnMissingFile, log)
}

func RunPollingFileTailer(globs []glob.Glob, readall bool, failOnMissingFile bool, pollInterval time.Duration, log logrus.FieldLogger) (FileTailer, error) {
	initFunc := func() (fswatcher, Error) {
		return initPollingWatcher(pollInterval)
	}
	return runFileTailer(initFunc, globs, readall, failOnMissingFile, log)
}

func runFileTailer(initFunc func() (fswatcher, Error), globs []glob.Glob, readall bool, failOnMissingFile bool, log logrus.FieldLogger) (FileTailer, error) {

	var (
		t   *fileTailer
		Err Error
	)

	t = &fileTailer{
		globs:        globs,
		watchedFiles: make(map[string]*fileWithReader),
		lines:        make(chan *Line),
		errors:       make(chan Error),
		done:         make(chan struct{}),
	}

	t.osSpecific, Err = initFunc()
	if Err != nil {
		return nil, Err
	}

	go func() {

		defer t.shutdown(log)

		Err = t.watchDirs(log)
		if Err != nil {
			select {
			case <-t.done:
			case t.errors <- Err:
			}
			return
		}

		eventProducerLoop := t.osSpecific.runFseventProducerLoop()
		defer eventProducerLoop.Close()

		for _, dir := range t.watchedDirs {
			dirLogger := log.WithField("directory", dir.Path())
			dirLogger.Debugf("initializing directory")
			Err = t.syncFilesInDir(dir, readall, dirLogger) // This may already write lines to the lines channel, so we will not go past this line unless the consumer starts reading lines.
			if Err != nil {
				select {
				case <-t.done:
				case t.errors <- Err:
				}
				return
			}
		}

		// make sure at least one logfile was found for each glob
		if failOnMissingFile {
			missingFileError := t.checkMissingFile()
			if missingFileError != nil {
				select {
				case <-t.done:
				case t.errors <- missingFileError:
				}
				return
			}
		}

		for { // event consumer loop
			select {
			case <-t.done:
				return
			case event, open := <-eventProducerLoop.Events():
				if !open {
					return
				}
				processEventError := t.osSpecific.processEvent(t, event, log)
				if processEventError != nil {
					select {
					case <-t.done:
					case t.errors <- processEventError:
					}
					return
				}
			case err, open := <-eventProducerLoop.Errors():
				if !open {
					return
				}
				select {
				case <-t.done:
				case t.errors <- NewError(NotSpecified, err, "error reading file system events"):
				}
				return
			}
		}
	}()
	return t, nil
}

func (t *fileTailer) shutdown(log logrus.FieldLogger) {

	close(t.lines)
	close(t.errors)

	warnf := func(format string, args ...interface{}) {
		log.Warnf("error while shutting down the file system watcher: %v", fmt.Sprintf(format, args))
	}

	for _, dir := range t.watchedDirs {
		err := t.osSpecific.unwatchDir(dir)
		if err != nil {
			warnf("%v", err)
		}
	}

	err := t.osSpecific.Close()
	if err != nil {
		warnf("%v", err)
	}

	for _, file := range t.watchedFiles {
		err = file.file.Close()
		if err != nil {
			warnf("close(%q) failed: %v", file.file.Name(), err)
		}
	}
}

func (t *fileTailer) watchDirs(log logrus.FieldLogger) Error {
	var (
		Err      Error
		dirPaths []string
		dirPath  string
	)
	dirPaths, Err = uniqueDirs(t.globs)
	if Err != nil {
		return Err
	}
	for _, dirPath = range dirPaths {
		log.Debugf("watching directory %v", dirPath)
		dir, Err := t.osSpecific.watchDir(dirPath)
		if Err != nil {
			return Err
		}
		t.watchedDirs = append(t.watchedDirs, dir)
	}
	return nil
}

func (t *fileTailer) syncFilesInDir(dir *Dir, readall bool, log logrus.FieldLogger) Error {
	watchedFilesAfter := make(map[string]*fileWithReader)
	for path, file := range t.watchedFiles {
		if filepath.Dir(path) != dir.Path() {
			watchedFilesAfter[path] = file
		}
	}
	fileInfos, Err := dir.ls()
	if Err != nil {
		return Err
	}
	for _, fileInfo := range fileInfos {
		filePath := filepath.Join(dir.Path(), fileInfo.Name())
		fileLogger := log.WithField("file", fileInfo.Name())
		if !anyGlobMatches(t.globs, filePath) {
			fileLogger.Debug("skipping file, because file name does not match")
			continue
		}
		if fileInfo.IsDir() {
			fileLogger.Debug("skipping, because it is a directory")
			continue
		}
		alreadyWatched, Err := findSameFile(t, fileInfo, filePath)
		if Err != nil {
			return Err
		}
		if alreadyWatched != nil {
			if alreadyWatched.file.Name() != filePath { // file is already watched but renamed
				renamedFile, err := NewFile(alreadyWatched.file, filePath)
				if err != nil {
					return NewErrorf(NotSpecified, err, "%v: failed to follow moved file", filePath)
				}
				fileLogger.WithField("fd", renamedFile.Fd()).Infof("file with old_fd=%v was moved from old_path=%v", alreadyWatched.file.Fd(), alreadyWatched.file.Name())
				alreadyWatched.file.Close()
				Err = t.osSpecific.watchFile(renamedFile)
				if Err != nil {
					renamedFile.Close()
					return Err
				}
				alreadyWatched.file = renamedFile // re-use lineReader
				Err = t.readNewLines(alreadyWatched, fileLogger)
				if Err != nil {
					alreadyWatched.file.Close()
					return Err
				}
				watchedFilesAfter[filePath] = alreadyWatched
			} else {
				fileLogger.Debug("skipping, because file is already watched")
				watchedFilesAfter[filePath] = alreadyWatched
			}
			continue
		}
		newFile, Err := open(filePath)
		if Err != nil {
			if Err.Type() == FileNotFound {
				fileLogger.Debug("skipping, because file does no longer exist")
				continue
			} else {
				return Err
			}
		}
		if !readall {
			_, err := newFile.Seek(0, io.SeekEnd)
			if err != nil {
				newFile.Close()
				return NewError(NotSpecified, os.NewSyscallError("seek", err), filePath)
			}
		}
		fileLogger = fileLogger.WithField("fd", newFile.Fd())
		fileLogger.Info("watching new file")

		Err = t.osSpecific.watchFile(newFile)
		if Err != nil {
			newFile.Close()
			return Err
		}

		newFileWithReader := &fileWithReader{file: newFile, reader: NewLineReader()}
		Err = t.readNewLines(newFileWithReader, fileLogger)
		if Err != nil {
			newFile.Close()
			return Err
		}
		watchedFilesAfter[filePath] = newFileWithReader
	}
	for _, f := range t.watchedFiles {
		if !contains(watchedFilesAfter, f) {
			fileLogger := log.WithField("file", filepath.Base(f.file.Name())).WithField("fd", f.file.Fd())
			fileLogger.Info("file was removed, closing and un-watching")
			f.file.Close()
		}
	}
	t.watchedFiles = watchedFilesAfter
	return nil
}

func (t *fileTailer) readNewLines(file *fileWithReader, log logrus.FieldLogger) Error {
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
		case <-t.done:
			return nil
		case t.lines <- &Line{Line: line, File: file.file.Name()}:
		}
	}
}

func (t *fileTailer) checkMissingFile() Error {
OUTER:
	for _, g := range t.globs {
		for watchedFileName := range t.watchedFiles {
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

func anyGlobMatches(globs []glob.Glob, path string) bool {
	for _, pattern := range globs {
		if pattern.Match(path) {
			return true
		}
	}
	return false
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
