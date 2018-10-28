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
	"io"
	"testing"
)

type mockFile struct {
	bytes []string
	pos   int
	eof   bool
}

func NewMockFile(lines ...string) *mockFile {
	return &mockFile{
		bytes: lines,
		pos:   0,
		eof:   false,
	}
}

func (f *mockFile) Read(p []byte) (int, error) {
	if f.eof {
		f.eof = false
		return 0, io.EOF
	} else {
		f.eof = true
		copy(p, []byte(f.bytes[f.pos])) // In this test the buffer p will alwasy be large enough.
		f.pos++
		return len(f.bytes[f.pos-1]), nil
	}
}

func collectLines(linechan chan string) []string {
	lines := []string{}
	for {
		select {
		case line := <-linechan:
			lines = append(lines, line)
		default:
			return lines
		}
	}
}

func TestLineReader(t *testing.T) {
	file := NewMockFile("This is l", "ine 1\n", "This is line two\nThis is line three\n", "This ", "is ", "line 4", "\n", "\n", "\n")

	done := make(chan struct{})
	linechan := make(chan string, 20)

	reader := NewBufferedLineReader(linechan, done)

	finished, err := reader.ReadAvailableLines(file)
	expectEmpty(t, collectLines(linechan), err)
	expectNotFinished(t, finished)

	finished, err = reader.ReadAvailableLines(file)
	expectLines(t, collectLines(linechan), err, "This is line 1")
	expectNotFinished(t, finished)

	finished, err = reader.ReadAvailableLines(file)
	expectLines(t, collectLines(linechan), err, "This is line two", "This is line three")
	expectNotFinished(t, finished)

	finished, err = reader.ReadAvailableLines(file) // This
	expectEmpty(t, collectLines(linechan), err)
	expectNotFinished(t, finished)

	finished, err = reader.ReadAvailableLines(file) // is
	expectEmpty(t, collectLines(linechan), err)
	expectNotFinished(t, finished)

	finished, err = reader.ReadAvailableLines(file) // line 4
	expectEmpty(t, collectLines(linechan), err)
	expectNotFinished(t, finished)

	finished, err = reader.ReadAvailableLines(file) // \n
	expectLines(t, collectLines(linechan), err, "This is line 4")
	expectNotFinished(t, finished)

	finished, err = reader.ReadAvailableLines(file) // \n
	expectLines(t, collectLines(linechan), err, "")
	expectNotFinished(t, finished)

	close(done)
	finished, err = reader.ReadAvailableLines(file) // \n
	expectFinished(t, finished)
}

func expectNotFinished(t *testing.T, finished bool) {
	if finished {
		t.Error("expected to be not finished, but finished")
	}
}

func expectFinished(t *testing.T, finished bool) {
	if !finished {
		t.Error("expected to be finished, but not finished")
	}
}

func expectEmpty(t *testing.T, lines []string, err error) {
	if err != nil {
		t.Error(err)
	}
	if lines == nil {
		t.Error("expected empty slice, but got nil")
	}
	if len(lines) > 0 {
		t.Errorf("expected empty slice, but got len = %v", len(lines))
	}
}

func expectLines(t *testing.T, lines []string, err error, expectedLines ...string) {
	if err != nil {
		t.Error(err)
	}
	if lines == nil {
		t.Error("slice is nil")
	}
	if len(lines) != len(expectedLines) {
		t.Errorf("expected slice with len = %v, but got len = %v", len(expectedLines), len(lines))
	}
	for i, expectedLine := range expectedLines {
		if lines[i] != expectedLine {
			t.Errorf("Expected line '%v', but got '%v'.", expectedLine, lines[i])
		}
	}
}
