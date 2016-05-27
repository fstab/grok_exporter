package tailer

import (
	"bufio"
	"fmt"
	mtail_tailer "github.com/google/mtail/tailer"
	"os"
)

type Tailer interface {
	LineChan() chan string
	ErrorChan() chan error
	Close()
}

type fileTailer struct {
	lines  chan string
	errors chan error
	t      *mtail_tailer.Tailer
}

func (f *fileTailer) Close() {
	f.t.Close()
	// f.lines is closed by f.t.Close() above.
	//close(f.lines)
	close(f.errors)
}

func (f *fileTailer) LineChan() chan string {
	return f.lines
}

func (f *fileTailer) ErrorChan() chan error {
	return f.errors
}

func RunFileTailer(path string, readall bool) (Tailer, error) {
	lines := make(chan string)
	t, err := mtail_tailer.New(mtail_tailer.Options{Lines: lines})
	if err != nil {
		return nil, fmt.Errorf("Initialization error: Failed to initialize the tail process: %v", err.Error())
	}
	go t.Tail(path, readall)
	return &fileTailer{
		lines:  lines,
		errors: make(chan error),
		t:      t,
	}, nil
}

type stdinTailer struct {
	lines  chan string
	errors chan error
}

func (s *stdinTailer) Close() {
	// TODO: stop reading from stdin
	close(s.lines)
	close(s.errors)
}

func (s *stdinTailer) LineChan() chan string {
	return s.lines
}

func (s *stdinTailer) ErrorChan() chan error {
	return s.errors
}

func RunStdinTailer() (Tailer, error) {
	lines := make(chan string)
	errors := make(chan error)
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				errors <- err
			} else {
				lines <- line
			}
		}
	}()
	return &stdinTailer{
		lines:  lines,
		errors: errors,
	}, nil
}
