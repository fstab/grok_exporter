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
