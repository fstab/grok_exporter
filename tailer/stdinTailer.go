package tailer

import (
	"bufio"
	"os"
	"strings"
)

type stdinTailer struct {
	lines  chan string
	errors chan error
}

func (t *stdinTailer) Lines() chan string {
	return t.lines
}

func (t *stdinTailer) Errors() chan error {
	return t.errors
}

func (t *stdinTailer) Close() {
	// TODO: How to stop the go-routine reading on stdin?
}

func RunStdinTailer() Tailer {
	lineChan := make(chan string)
	errorChan := make(chan error)
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				errorChan <- err
				return
			}
			line = strings.TrimRight(line, "\r\n")
			lineChan <- line
		}
	}()
	return &stdinTailer{
		lines:  lineChan,
		errors: errorChan,
	}
}
