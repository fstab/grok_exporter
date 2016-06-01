package tailer

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const OPEN_CLOSE = 1
const KEEP_OPEN = 2

func TestTailer(t *testing.T) {
	testdir, err := ioutil.TempDir("", "grok_exporter")
	if err != nil {
		t.Errorf("Failed to create test directory: %v", err.Error())
		return
	}
	defer os.RemoveAll(testdir)
	logfile, err := ioutil.TempFile(testdir, "grok_exporter_test_log")
	if err != nil {
		t.Errorf("Cannot create temp file: %v\n", err.Error())
		return
	}
	logfile.Sync()
	logfile.Close()
	fmt.Printf("Using tmp file %v\n", logfile.Name())
	defer os.Remove(logfile.Name())
	simulateLog(logfile, "test line 1", OPEN_CLOSE, t)
	simulateLog(logfile, "test line 2", OPEN_CLOSE, t)
	tail, err := RunFileTailer2(logfile.Name(), true)
	if err != nil {
		t.Errorf("Failed to start tailer: %v", err.Error())
	}
	// The first two lines are received without any fsnotify event,
	// because they were written before the watcher was started.
	expect(tail.LineChan(), "test line 1", 1*time.Second, t)
	expect(tail.LineChan(), "test line 2", 1*time.Second, t)
	// Append a line and see if the event is processed.
	simulateLog(logfile, "test line 3", OPEN_CLOSE, t)
	expect(tail.LineChan(), "test line 3", 10*time.Second, t)
	// Simulate logrotate: Remove and Re-Create.
	logfile = rotate(logfile, testdir, t)
	logfile.Close()
	// Log two more lines and see if they are received.
	simulateLog(logfile, "line 4", OPEN_CLOSE, t)
	simulateLog(logfile, "line 5", OPEN_CLOSE, t)
	expect(tail.LineChan(), "line 4", 10*time.Second, t)
	expect(tail.LineChan(), "line 5", 1*time.Second, t)
	tail.Close()
}

func simulateLog(file *os.File, line string, mode int, t *testing.T) {
	switch {
	case mode == OPEN_CLOSE:
		logOpenClose(file, line, t)
	case mode == KEEP_OPEN:
		logKeepOpen(file, line, t)
	default:
		t.Errorf("Uexpected mode: %v", mode)
	}
}

func logKeepOpen(file *os.File, line string, t *testing.T) {
	_, err := file.WriteString(fmt.Sprintf("%v\n", line))
	if err != nil {
		t.Errorf("%v: %v", file.Name(), err.Error())
	}
}

func logOpenClose(file *os.File, line string, t *testing.T) {
	f, err := os.OpenFile(file.Name(), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		t.Errorf("%v: %v", file.Name(), err.Error())
		return
	}
	defer f.Close()
	logKeepOpen(f, line, t)
	f.Sync()
}

func containsFile(files []os.FileInfo, file *os.File) bool {
	for _, f := range files {
		if filepath.Base(f.Name()) == filepath.Base(file.Name()) {
			return true
		}
	}
	return false
}

func ls(path string, t *testing.T) []os.FileInfo {
	result, err := ioutil.ReadDir(path)
	if err != nil {
		t.Fatalf("%v: %v", path, err.Error())
	}
	return result
}

func rotate(f *os.File, dir string, t *testing.T) *os.File {
	filesBefore := ls(dir, t)
	if !containsFile(filesBefore, f) {
		t.Fatalf("%v does not contain %v before logrotate.", dir, filepath.Base(f.Name()))
	}
	time.Sleep(1 * time.Second)
	os.Remove(f.Name())
	filesAfterRemove := ls(dir, t)
	if containsFile(filesAfterRemove, f) {
		t.Fatalf("%v still contains file %v after remove.", dir, filepath.Base(f.Name()))
	}
	time.Sleep(1 * time.Second)
	newF, err := os.Create(f.Name())
	if err != nil {
		t.Errorf("Failed to re-create %v while simulating logrotate: %v", f.Name(), err.Error())
	}
	filesAfterCreate := ls(dir, t)
	if !containsFile(filesAfterCreate, f) {
		t.Fatalf("%v does not contain %v after logrotate.", dir, filepath.Base(f.Name()))
	}
	time.Sleep(1 * time.Second)
	return newF
}

func expect(c chan string, line string, timeout time.Duration, t *testing.T) {
	timeoutChan := make(chan bool, 1)
	go func() {
		time.Sleep(timeout)
		timeoutChan <- true
	}()
	select {
	case result := <-c:
		if result != line {
			t.Errorf("Expected '%v', but got '%v'.", line, result)
		} else {
			fmt.Printf("Read expected line '%v'\n", line)
		}
	case <-timeoutChan:
		t.Errorf("Timeout while waiting for line '%v'", line)
	}
}
