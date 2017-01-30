// Copyright 2016-2017 The grok_exporter Authors
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
	"os"
	"strings"
)

type stdinTailer struct {
	lines  chan string
	errors chan error
}

func (t *stdinTailer) Lines() chan string {
	return t.lines
}

func (t *stdinTailer) Errors() chan error {
	return t.errors
}

func (t *stdinTailer) Close() {
	// TODO: How to stop the go-routine reading on stdin?
}

func RunStdinTailer() Tailer {
	lineChan := make(chan string)
	errorChan := make(chan error)
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				errorChan <- err
				return
			}
			line = strings.TrimRight(line, "\r\n")
			lineChan <- line
		}
	}()
	return &stdinTailer{
		lines:  lineChan,
		errors: errorChan,
	}
}
