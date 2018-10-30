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
	"math/rand"
	"strings"
	"testing"
)

type mockFile struct {
	results []readResult
	pos     int
}

type readResult struct {
	data string
	err  error
}

func newMockFile(readResults []readResult) *mockFile {
	return &mockFile{
		results: readResults,
		pos:     0,
	}
}

func (f *mockFile) Read(p []byte) (int, error) {
	if f.pos >= len(f.results) {
		return 0, io.EOF
	}
	result := f.results[f.pos]
	f.pos++
	copy(p, result.data) // In this test the buffer p will always be large enough.
	//fmt.Printf("mock file read() returning %q, %v\n", result.data, result.err)
	return len(result.data), result.err
}

func TestLineReader(t *testing.T) {
	var (
		file = newMockFile([]readResult{
			// reading line 1 takes two read operations
			{"This is l", nil},
			{"ine 1\n", nil},
			// line 2, line 3, and the beginning of line 4 are read in a single read operation
			{"This is line 2\nThis is line 3\nThis ", nil},
			// reading line 4 takes three read operations
			{"is line 4", nil},
			{"\n", nil},
			// while reading line 5 we temporarily hit eof
			{"This is ", nil},
			{"", io.EOF},
			{"", io.EOF},
			{"line ", nil},
			// eof with data
			{"5\n", io.EOF},
			// line 6 is empty
			{"\n", nil},
			// line 7 is empty
			{"\n", nil},
		})
		reader = NewLineReader()
		line   string
		eof    bool
		err    error
		tries  int
	)
	for _, expected := range []string{
		"This is line 1",
		"This is line 2",
		"This is line 3",
		"This is line 4",
		"This is line 5",
		"",
		"",
	} {
		for tries = 0; tries < 10; tries++ {
			line, eof, err = reader.ReadLine(file)
			if err != nil {
				t.Fatalf("unexpected error reading line: %v", err)
			}
			if !eof {
				break
			}
		}
		if tries >= 10 {
			t.Fatalf("failed to read line after %v tries.\n", tries)
		}
		if line != expected {
			t.Fatalf("expected line %q, but got %q\n", expected, line)
		}
	}
	line, eof, err = reader.ReadLine(file)
	if len(line) > 0 || !eof || err != nil {
		t.Fatalf("unexpected result after hitting last line: line=%q eof=%v err=%v", line, eof, err)
	}
}

func TestLineReaderWindowsLineEndings(t *testing.T) {
	file := newMockFile([]readResult{
		// reading line 1 takes two read operations
		{"Line with Windows line ending\r\n", nil},
		{"Line 2\r\n", nil},
	})
	reader := NewLineReader()
	line, _, _ := reader.ReadLine(file)
	if line != "Line with Windows line ending" {
		t.Fatalf("expected \"Line with Windows line ending\", but got %q", line)
	}
	line, _, _ = reader.ReadLine(file)
	if line != "Line 2" {
		t.Fatalf("expected \"Line 2\", but got %q", line)
	}
}

type largeLineMockFile struct {
	currentLine string
	pos         int
}

func (file *largeLineMockFile) next() {
	length := rand.Intn(1024*1024) + 1
	nextLine := make([]byte, length+1) // next line is initialized with '\0'
	// Commented out, because generating random content makes the test much slower
	//for i := 0; i<length; i++ {
	//	nextLine[i] = 'a' + byte(rand.Intn('z'-'a'))
	//}
	nextLine[length] = '\n'
	file.currentLine = string(nextLine)
	file.pos = 0
}

func (file *largeLineMockFile) Read(p []byte) (int, error) {
	if file.pos >= len(file.currentLine) {
		return 0, io.EOF
	}
	n := min(len(p), len(file.currentLine)-file.pos)
	copy(p, file.currentLine[file.pos:])
	file.pos += n
	return n, nil
}

func min(a, b int) int {
	if a > b {
		return b
	} else {
		return a
	}
}

func TestLineReaderLargeLines(t *testing.T) {
	rand.Seed(0)
	file := &largeLineMockFile{}
	reader := NewLineReader()
	for i := 0; i < 100; i++ {
		file.next()
		line, _, _ := reader.ReadLine(file)
		if line != strings.TrimRight(file.currentLine, "\n") {
			t.Fatal("read unexpected line")
		}
	}
}
