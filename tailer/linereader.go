package tailer

import (
	"bytes"
	"fmt"
	"io"
	"log"
)

type bufferedLineReader struct {
	bytesFromLastRead []byte // bytesFromLastRead is a buffer for Bytes read from the logfile, but no newline yet, so we need to wait until we can send it to outputChannel.
	outputChannel     chan string
	file              File
}

func NewBufferedLineReader(file File, outputChannel chan string) *bufferedLineReader {
	return &bufferedLineReader{
		bytesFromLastRead: make([]byte, 0),
		outputChannel:     outputChannel,
		file:              file,
	}
}

func (r *bufferedLineReader) ProcessAvailableLines() error {
	newBytes, err := read2EOF(r.file)
	if err != nil {
		return err
	}
	remainingBytes, lines := stripLines(append(r.bytesFromLastRead, newBytes...))
	for _, line := range lines {
		r.outputChannel <- line
	}
	r.bytesFromLastRead = remainingBytes
	return nil
}

func read2EOF(file File) ([]byte, error) {
	result := make([]byte, 0)
	buf := make([]byte, 512)
	for {
		n, err := file.Read(buf)
		if err != nil {
			if err == io.EOF {
				return result, nil
			} else {
				return nil, fmt.Errorf("Error reading from %v: %v", file.Name(), err.Error())
			}
		}
		result = append(result, buf[0:n]...)
	}
}

func stripLines(data []byte) ([]byte, []string) {
	newline := []byte("\n")
	result := make([]string, 0)
	lines := bytes.SplitAfter(data, newline)
	for i, line := range lines {
		if bytes.HasSuffix(line, newline) {
			line = bytes.TrimSuffix(line, newline)
			line = bytes.TrimSuffix(line, []byte("\r")) // Needed for CRLF line endings?
			result = append(result, string(line))
		} else {
			if i != len(lines)-1 {
				log.Fatal("Unexpected error while splitting log data into lines. This is a bug.\n")
			}
			return line, result
		}
	}
	return make([]byte, 0), result
}

// implemented by *os.File
type File interface {
	Read(b []byte) (int, error)
	Name() string
}
