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

package tailer

import (
	"github.com/fstab/grok_exporter/tailer/fswatcher"
)

type tailerWrapper struct {
	lines  chan string
	errors chan Error
	done   chan struct{}
}

func (t *tailerWrapper) Close() {
	close(t.done)
}

func (t *tailerWrapper) Lines() chan string {
	return t.lines
}

func (t *tailerWrapper) Errors() chan Error {
	return t.errors
}

// Switch to the new file tailer implementation which supports watching multiple files.
// Once we switched for all supported operating systems, we can remove the old implementation and the wrapper.
func RunFseventFileTailer(path string, readall bool, failOnMissingFile bool, _ interface{}) Tailer {
	result := &tailerWrapper{
		lines:  make(chan string),
		errors: make(chan Error),
		done:   make(chan struct{}),
	}

	newTailer, err := fswatcher.Run([]string{path}, readall, failOnMissingFile)
	if err != nil {
		go func() {
			result.errors <- newError("failed to initialize file system watcher", err)
		}()
		return result
	}

	go func() {
		defer func() {
			close(result.lines)
			close(result.errors)
			newTailer.Close()
		}()
		for {
			select {
			case l := <-newTailer.Lines():
				// fmt.Printf("*** forwarding line %q to wrapped tailer\n", l.Line)
				result.lines <- l.Line
			case e := <-newTailer.Errors():
				result.errors <- newError(e.Error(), e.Cause())
				return
			case <-result.done:
				return
			}
		}
	}()
	return result
}
