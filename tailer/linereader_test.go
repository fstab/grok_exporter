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
	out := make(chan string, 3)
	reader := NewBufferedLineReader(file, out)

	err := reader.ProcessAvailableLines()
	expectEmptyRead(t, err, out)
	err = reader.ProcessAvailableLines()
	expectLine(t, err, out, "This is line 1")
	err = reader.ProcessAvailableLines()
	expectLine(t, err, out, "This is line two") // two consecutive lines with one read
	expectLine(t, nil, out, "This is line three")
	err = reader.ProcessAvailableLines() // This
	expectEmptyRead(t, err, out)
	err = reader.ProcessAvailableLines() // is
	expectEmptyRead(t, err, out)
	err = reader.ProcessAvailableLines() // line 4
	expectEmptyRead(t, err, out)
	err = reader.ProcessAvailableLines() // \n
	expectLine(t, err, out, "This is line 4")
	err = reader.ProcessAvailableLines() // \n
	expectLine(t, err, out, "")
	close(out)
}

func expectEmptyRead(t *testing.T, err error, out chan string) {
	if err != nil {
		t.Error(err)
	}
	select {
	case line := <-out:
		t.Errorf("Expected empty read, but got '%v'.", line)
	default:
		// ok
	}
}

func expectLine(t *testing.T, err error, out chan string, expectedLine string) {
	if err != nil {
		t.Error(err)
	}
	line := <-out
	if line != expectedLine {
		t.Errorf("Expected line '%v', but got '%v'.", expectedLine, line)
	}
}
