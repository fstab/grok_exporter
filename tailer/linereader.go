package tailer

import (
	"bytes"
	"log"
)

type lineReader struct {
	bytesFromLastRead []byte // bytesFromLastRead is a buffer for Bytes read from the logfile, but no newline yet, so we need to wait until we can send it to outputChannel.
	outputChannel     chan string
	file              *tailedFile
}

func NewLineReader(file *tailedFile, outputChannel chan string) *lineReader {
	return &lineReader{
		bytesFromLastRead: make([]byte, 0),
		outputChannel:     outputChannel,
		file:              file,
	}
}

func (l *lineReader) ProcessAvailableLines() error {
	newBytes, err := l.file.Read2EOF()
	if err != nil {
		return err
	}
	remainingBytes, lines := stripLines(append(l.bytesFromLastRead, newBytes...))
	for _, line := range lines {
		l.outputChannel <- line
	}
	l.bytesFromLastRead = remainingBytes
	return nil
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
