package tailer

import (
	"os"
	"syscall"
	"testing"
	"time"
)

func TestCloseKqBefore(t *testing.T) {
	runTestKevent(t, "close kq before kevent()")
}

func TestCloseKqDuring(t *testing.T) {
	runTestKevent(t, "close kq during kevent()")
}

func TestKqDone(t *testing.T) {
	runTestKevent(t, "exit via done channel")
}

func runTestKevent(t *testing.T, mode string) {
	tmpDir := mkTmpDirOrFail(t)
	defer cleanUp(t, tmpDir)
	logfile := mkTmpFileOrFail(t, tmpDir)
	lines := make(chan string)
	defer close(lines)

	_, _, _, kq, err := initWatcher(logfile, false)
	if err != nil {
		t.Error(err)
	}
	switch {
	case mode == "close kq before kevent()":
		syscall.Close(kq)
	case mode == "close kq during kevent()":
		go func() {
			time.Sleep(1 * time.Second)
			syscall.Close(kq)
		}()
	case mode == "exit via done channel":
		go func() {
			time.Sleep(1 * time.Second)
			err = os.Remove(logfile) // trigger file system event so kevent() returns.
			if err != nil {
				t.Errorf("Failed to remove logfile: %v", err)
			}
		}()
	default:
		t.Errorf("Unknown mode %v", mode)
	}
	events, errors, done := startEventReader(kq)
	switch {
	case mode == "exit via done channel":
		close(done)
	default:
		defer close(done)
	}
	_, ok := <-errors
	if ok {
		t.Error("error channel not closed")
	}
	_, ok = <-events
	if ok {
		t.Error("events channel not closed")
	}
}

//switch err := err.(type) {
//case syscall.Errno:
//	t.Errorf("Error of type %v with errno %v and text %v", reflect.TypeOf(err), int(err), err)
//default:
//	t.Errorf("Error of type %v with text %v", reflect.TypeOf(err), err)
//}
