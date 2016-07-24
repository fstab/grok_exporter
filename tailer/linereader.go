package tailer

import (
	"bytes"
	"fmt"
	"io"
)

type bufferedLineReader struct {
	remainingBytesFromLastRead []byte
	out                        chan string
	in                         io.Reader
}

func NewBufferedLineReader(file io.Reader, out chan string) *bufferedLineReader {
	return &bufferedLineReader{
		remainingBytesFromLastRead: make([]byte, 0),
		out: out,
		in:  file,
	}
}

func (r *bufferedLineReader) ProcessAvailableLines() error {
	newBytes, err := read2EOF(r.in)
	if err != nil {
		return err
	}
	lines, remainingBytes := splitLines(append(r.remainingBytesFromLastRead, newBytes...))
	for _, line := range lines {
		r.out <- line
	}
	r.remainingBytesFromLastRead = remainingBytes
	return nil
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
