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
