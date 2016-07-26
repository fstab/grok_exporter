package tailer

import (
	"os"
	"testing"
	"time"
)

func TestShutdownDuringSyscallRead(t *testing.T) {
	runTestShutdown(t, "shutdown while the watcher is hanging in the blocking syscall.Read()")
}

func TestShutdownDuringSendEvent(t *testing.T) {
	runTestShutdown(t, "shutdown while the watcher is sending an event")
}

func runTestShutdown(t *testing.T, mode string) {
	tmpDir := mkTmpDirOrFail(t)
	defer cleanUp(t, tmpDir)
	logfile := mkTmpFileOrFail(t, tmpDir)
	lines := make(chan string)
	defer close(lines)

	_, fd, wd, _, err := initWatcher(logfile, false)
	if err != nil {
		t.Error(err)
	}
	events, errors, shutdownCallback := startEventReader(fd, wd)
	switch {
	case mode == "shutdown while the watcher is hanging in the blocking syscall.Read()":
		time.Sleep(200 * time.Millisecond)
		shutdownCallback()
	case mode == "shutdown while the watcher is sending an event":
		err = os.Remove(logfile) // trigger file system event so kevent() returns.
		if err != nil {
			t.Errorf("Failed to remove logfile: %v", err)
		}
		// The watcher is now waiting until we read the event from the event channel.
		// However, we shut down and abort the event.
		shutdownCallback()
	default:
		t.Errorf("Unknown mode: %v", mode)
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
