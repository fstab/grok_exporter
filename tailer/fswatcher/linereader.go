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

package fswatcher

import (
	"bytes"
	"io"
)

type lineReader struct {
	remainingBytesFromLastRead []byte
}

func NewLineReader() *lineReader {
	return &lineReader{
		remainingBytesFromLastRead: []byte{},
	}
}

// read the next line from the file.
// return values are (line, eof, err).
// * line is the line read.
// * eof is a boolean indicating if the end of file was reached before getting to the next '\n'.
// * err is set if an error other than io.EOF has occurred. err is never io.EOF.
// if eof is true, line is always "" and err always is nil.
// if eof is false and err is nil, an empty line means that there actually was an empty line in the file.
func (r *lineReader) ReadLine(file io.Reader) (string, bool, error) {
	var (
		err error
		buf = make([]byte, 512)
		n   = 0
	)
	for {
		newlinePos := bytes.IndexByte(r.remainingBytesFromLastRead, '\n')
		if newlinePos >= 0 {
			l := len(r.remainingBytesFromLastRead)
			result := make([]byte, newlinePos)
			copy(result, r.remainingBytesFromLastRead[:newlinePos])
			copy(r.remainingBytesFromLastRead, r.remainingBytesFromLastRead[newlinePos+1:])
			r.remainingBytesFromLastRead = r.remainingBytesFromLastRead[:l-(newlinePos+1)]
			return string(stripWindowsLineEnding(result)), false, nil
		} else if err != nil {
			if err == io.EOF {
				return "", true, nil
			} else {
				return "", false, err
			}
		} else {
			n, err = file.Read(buf)
			if n > 0 {
				// io.Reader: Callers should always process the n > 0 bytes returned before considering the error err.
				r.remainingBytesFromLastRead = append(r.remainingBytesFromLastRead, buf[0:n]...)
			}
		}
	}
}

func stripWindowsLineEnding(s []byte) []byte {
	if len(s) > 0 && s[len(s)-1] == '\r' {
		return s[:len(s)-1]
	} else {
		return s
	}
}

func (r *lineReader) Clear() {
	r.remainingBytesFromLastRead = r.remainingBytesFromLastRead[:0]
}
