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
}

func NewBufferedLineReader() *bufferedLineReader {
	return &bufferedLineReader{
		remainingBytesFromLastRead: []byte{},
	}
}

func (r *bufferedLineReader) ReadAvailableLines(file io.Reader) ([]string, error) {
	var lines []string
	newBytes, err := read2EOF(file)
	if err != nil {
		return nil, err
	}
	lines, r.remainingBytesFromLastRead = splitLines(append(r.remainingBytesFromLastRead, newBytes...))
	return lines, nil
}

func (r *bufferedLineReader) Clear() {
	r.remainingBytesFromLastRead = []byte{}
}

func read2EOF(file io.Reader) ([]byte, error) {
	result := make([]byte, 0)
	buf := make([]byte, 512)
	for {
		n, err := file.Read(buf)
		if n > 0 {
			// Callers should always process the n > 0 bytes returned before considering the error err.
			result = append(result, buf[0:n]...)
		}
		if err != nil {
			if err == io.EOF {
				return result, nil
			} else {
				return nil, fmt.Errorf("read error: %v", err.Error())
			}
		}
	}
}

func splitLines(data []byte) (lines []string, remainingBytes []byte) {
	newline := []byte("\n")
	lines = make([]string, 0)
	remainingBytes = make([]byte, 0)
	for _, line := range bytes.SplitAfter(data, newline) {
		if bytes.HasSuffix(line, newline) {
			line = bytes.TrimSuffix(line, newline)
			line = bytes.TrimSuffix(line, []byte("\r")) // Needed for CRLF line endings?
			lines = append(lines, string(line))
		} else {
			// This is the last (incomplete) line returned by SplitAfter(). We will exit the for loop here.
			remainingBytes = line
		}
	}
	return lines, remainingBytes
}
