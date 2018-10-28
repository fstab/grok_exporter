// Copyright 2016-2018 The grok_exporter Authors
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
	"bytes"
	"fmt"
	"io"
)

type bufferedLineReader struct {
	remainingBytesFromLastRead []byte

	// channels are used to stream the results out
	lines chan<- string
	done  <-chan struct{}
}

func NewBufferedLineReader(lines chan<- string, done <-chan struct{}) *bufferedLineReader {
	return &bufferedLineReader{
		remainingBytesFromLastRead: []byte{},
		lines: lines,
		done:  done,
	}
}

func (r *bufferedLineReader) ReadAvailableLines(file io.Reader) (bool, error) {

	// for each buffer, split lines and stream
	buf := make([]byte, 512)
	var done bool

	for {
		n, err := file.Read(buf)
		if n > 0 {
			// Callers should always process the n > 0 bytes returned before considering the error err.
			result := append(r.remainingBytesFromLastRead, buf[0:n]...)
			done, r.remainingBytesFromLastRead = r.processLines(result)
			if done {
				return true, nil
			}
		}
		if err != nil {
			if err == io.EOF {
				return false, nil
			} else {
				return false, fmt.Errorf("read error: %v", err.Error())
			}
		}
	}
}

func (r *bufferedLineReader) Clear() {
	r.remainingBytesFromLastRead = []byte{}
}

func (r *bufferedLineReader) processLines(data []byte) (finished bool, remainingBytes []byte) {
	newline := []byte("\n")
	for _, line := range bytes.SplitAfter(data, newline) {
		if bytes.HasSuffix(line, newline) {
			line = bytes.TrimSuffix(line, newline)
			line = bytes.TrimSuffix(line, []byte("\r")) // Needed for CRLF line endings?
			select {
			case r.lines <- string(line):
			case <-r.done:
				finished = true
				return
			}
		} else {
			// This is the last (incomplete) line returned by SplitAfter(). We will exit the for loop here.
			remainingBytes = line
		}
	}
	return
}
