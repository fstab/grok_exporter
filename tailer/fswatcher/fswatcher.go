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
	"github.com/fstab/grok_exporter/tailer/glob"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"path/filepath"
)

type Line struct {
	Line string
	File string
}

type FSWatcher interface {
	Lines() chan Line
	Errors() chan Error
	Close()
}

func Run(globs []glob.Glob, readall bool, failOnMissingFile bool, log logrus.FieldLogger) (FSWatcher, error) {

	var (
		w   *watcher
		Err Error
	)

	w, Err = initWatcher(globs)
	if Err != nil {
		return nil, Err
	}

	go func() {

		defer w.shutdown()

		Err = w.watchDirs(log)
		if Err != nil {
			select {
			case <-w.done:
			case w.errors <- Err:
			}
			return
		}

		eventProducerLoop := w.runFseventProducerLoop()
		defer eventProducerLoop.Close()

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
			case event := <-eventProducerLoop.events:
				processEventError := w.processEvent(event, log)
				if processEventError != nil {
					select {
					case <-w.done:
					case w.errors <- processEventError:
					}
					return
				}
			case err := <-eventProducerLoop.errors:
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

func (w *watcher) syncFilesInDir(dir *Dir, readall bool, log logrus.FieldLogger) Error {
	watchedFilesAfter := make(map[string]*fileWithReader)
	fileInfos, Err := dir.ls()
	if Err != nil {
		return Err
	}
	for _, fileInfo := range fileInfos {
		filePath := filepath.Join(dir.Path(), fileInfo.Name())
		fileLogger := log.WithField("file", fileInfo.Name())
		if !anyGlobMatches(w.globs, filePath) {
			fileLogger.Debug("skipping file, because file name does not match")
			continue
		}
		if fileInfo.IsDir() {
			fileLogger.Debug("skipping, because it is a directory")
			continue
		}
		alreadyWatched, Err := w.findSameFile(fileInfo, filePath)
		if Err != nil {
			return Err
		}
		if alreadyWatched != nil {
			if alreadyWatched.file.Name() != filePath {
				fileLogger.WithField("fd", alreadyWatched.file.Fd()).Infof("file was moved from %v", alreadyWatched.file.Name())
				alreadyWatched.file = NewFile(alreadyWatched.file, filePath)
			} else {
				fileLogger.Debug("skipping, because file is already watched")
			}
			watchedFilesAfter[filePath] = alreadyWatched
			continue
		}
		newFile, err := open(filePath)
		if err != nil {
			return NewErrorf(NotSpecified, err, "%v: failed to open file", filePath)
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

		Err = w.watchNewFile(newFile)
		if Err != nil {
			newFile.Close()
			return Err
		}

		newFileWithReader := &fileWithReader{file: newFile, reader: NewLineReader()}
		Err = w.readNewLines(newFileWithReader, fileLogger)
		if Err != nil {
			newFile.Close()
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
