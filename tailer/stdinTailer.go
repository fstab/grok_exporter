// Copyright 2016-2019 The grok_exporter Authors
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
	"bufio"
	"github.com/fstab/grok_exporter/tailer/fswatcher"
	"os"
	"strings"
)

type stdinTailer struct {
	lines  chan *fswatcher.Line
	errors chan fswatcher.Error
}

func (t *stdinTailer) Lines() chan *fswatcher.Line {
	return t.lines
}

func (t *stdinTailer) Errors() chan fswatcher.Error {
	return t.errors
}

func (t *stdinTailer) Close() {
	// TODO: How to stop the go-routine reading on stdin?
}

func RunStdinTailer() fswatcher.FileTailer {
	lineChan := make(chan *fswatcher.Line)
	errorChan := make(chan fswatcher.Error)
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				errorChan <- fswatcher.NewError(fswatcher.NotSpecified, err, "")
				return
			}
			line = strings.TrimRight(line, "\r\n")
			lineChan <- &fswatcher.Line{Line: line}
		}
	}()
	return &stdinTailer{
		lines:  lineChan,
		errors: errorChan,
	}
}
