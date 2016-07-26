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

func TestLineReader(t *testing.T) {
	file := NewMockFile("This is l", "ine 1\n", "This is line two\nThis is line three\n", "This ", "is ", "line 4", "\n", "\n")
	reader := NewBufferedLineReader()

	lines, err := reader.ReadAvailableLines(file)
	expectEmpty(t, lines, err)
	lines, err = reader.ReadAvailableLines(file)
	expectLines(t, lines, err, "This is line 1")
	lines, err = reader.ReadAvailableLines(file)
	expectLines(t, lines, err, "This is line two", "This is line three")
	lines, err = reader.ReadAvailableLines(file) // This
	expectEmpty(t, lines, err)
	lines, err = reader.ReadAvailableLines(file) // is
	expectEmpty(t, lines, err)
	lines, err = reader.ReadAvailableLines(file) // line 4
	expectEmpty(t, lines, err)
	lines, err = reader.ReadAvailableLines(file) // \n
	expectLines(t, lines, err, "This is line 4")
	lines, err = reader.ReadAvailableLines(file) // \n
	expectLines(t, lines, err, "")
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
