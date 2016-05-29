package tailer

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func TestTailer(t *testing.T) {
	fmt.Printf("starting the test...\n")
	logfile, err := ioutil.TempFile(os.TempDir(), "grok_exporter_test_log")
	if err != nil {
		t.Errorf("Cannot create temp file: %v\n", err.Error())
	}
	fmt.Printf("Using tmp file %v\n", logfile.Name())
	defer os.Remove(logfile.Name())
	log(logfile, "line 1")
	log(logfile, "line 2")
	tail, err := RunFileTailer2(logfile.Name(), true)
	if err != nil {
		t.Errorf("Failed to start tailer: %v", err.Error())
	}
	expect(tail.LineChan(), "line 1", 1*time.Second, t)
	expect(tail.LineChan(), "line 2", 1*time.Second, t)
	logfile = rotate(logfile, t)
	log(logfile, "line 3")
	log(logfile, "line 4")
	expect(tail.LineChan(), "line 3", 1*time.Second, t)
	expect(tail.LineChan(), "line 4", 1*time.Second, t)
	log(logfile, "line 5")
	expect(tail.LineChan(), "line 5", 1*time.Second, t)
	tail.Close()
}

func log(f *os.File, line string) {
	f.WriteString(fmt.Sprintf("%v\n", line))
}

func rotate(f *os.File, t *testing.T) *os.File {
	f.Close()
	os.Remove(f.Name())
	newF, err := os.Create(f.Name())
	if err != nil {
		t.Errorf("Failed to re-create %v while simulating logrotate: %v", f.Name(), err.Error())
	}
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
		}
		return
	case <-timeoutChan:
		t.Errorf("Timeout while waiting for line '%v'", line)
	}
}
