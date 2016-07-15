package tailer

import (
	"bytes"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"time"
)

type fileTailer struct {
	lines  chan string
	errors chan error
	done   chan bool
}

func (f *fileTailer) Close() {
	// Will panic if Close() is called multiple times.
	f.done <- true
	close(f.done)
	close(f.lines)
	close(f.errors)
}

func (f *fileTailer) LineChan() chan string {
	return f.lines
}

func (f *fileTailer) ErrorChan() chan error {
	return f.errors
}

// The original fileTailer implementation uses github.com/fsnotify/fsnotify.
// While fsnotify works fine on Linux and Windows, we experienced lost events on macOS.
// We suspect the reason to be a race condition: When logrotate moves a file, but the file is re-created immediately,
// fsnotify will not trigger a CREATE event with the kqueue() and kevent() implementation on macOS.
// To solve that, we hacked a simplified file watcher for macOS that comares inodes to learn if a file was moved.
func RunFileTailer(path string, readall bool, log simpleLogger) Tailer {
	linesChannel := make(chan string)
	doneChannel := make(chan bool)
	errorChannel := make(chan error)
	go func() {
		abspath, err := filepath.Abs(path)
		if err != nil {
			errorChannel <- fmt.Errorf("%v: %v", path, err.Error())
			return
		}

		file, err := NewTailedFile(abspath)
		if err != nil {
			errorChannel <- err
			return
		}

		dir, err := os.Open(filepath.Dir(abspath))
		if err != nil {
			errorChannel <- fmt.Errorf("%v: %v", path, err.Error())
			return
		}

		if !readall {
			err = file.SeekEnd()
			if err != nil {
				errorChannel <- err
				return
			}
		}

		kq, err := unix.Kqueue()
		if kq == -1 || err != nil {
			errorChannel <- fmt.Errorf("Failed to watch %v: %v\n", path, err.Error())
			return
		}

		kqueueEventChannel := make(chan int, 10)
		kqueueDoneChannel := make(chan bool)

		go func() {
			if readall {
				kqueueEventChannel <- 1 // Simulate event, so that pre-existing lines are read.
			}
			for {
				timeout := unix.NsecToTimespec((100 * time.Millisecond).Nanoseconds())
				events := make([]unix.Kevent_t, 10)
				f, _ := os.Open(abspath) // f may be nil if not exists, in that case we just listen for events on dir.
				n, _ := unix.Kevent(kq, makeChanges(dir, f), events, &timeout)
				select {
				case <-kqueueDoneChannel:
					return
				default:
					if n > 0 {
						kqueueEventChannel <- n
					}
				}
			}
		}()

		buf := make([]byte, 0) // Buffer for Bytes read from the logfile, but no newline yet, so we need to wait until we can send it to linesChannel.

		for {
			select {
			case <-doneChannel:
				kqueueDoneChannel <- true
				dir.Close()
				if file.IsOpen() {
					file.Close()
				}
				unix.Close(kq)
				return
			case n := <-kqueueEventChannel:
				fmt.Printf("Processing event (n=%v)\n", n)
				if file.IsClosed() {
					err = file.Open()
					if err != nil {
						continue // File not found. Wait for next event.
					}
				}
				if file.WasMoved() {
					buf, err = processAvailableLines(file, buf, linesChannel) // lines in old file
					if err != nil {
						errorChannel <- err
						return
					}
					file.Close()
					err = file.Open()
					if err != nil {
						continue // File not found: New file was not created yet. Wait for next event.
					}
				}
				trunkated, err := file.IsTruncated()
				if err != nil {
					errorChannel <- err
					return
				}
				if trunkated {
					err = file.SeekStart()
					if err != nil {
						errorChannel <- err
						return
					}
				}
				buf, err = processAvailableLines(file, buf, linesChannel)
				if err != nil {
					errorChannel <- err
					return
				}
			}
		}
	}()
	return &fileTailer{
		lines:  linesChannel,
		errors: errorChannel,
		done:   doneChannel,
	}
}

func processAvailableLines(file *tailedFile, bytesFromLastRead []byte, linesChannel chan string) ([]byte, error) {
	newBytes, err := file.Read2EOF()
	if err != nil {
		return nil, err
	}
	remainingBytes, lines := stripLines(append(bytesFromLastRead, newBytes...))
	for _, line := range lines {
		linesChannel <- line
	}
	return remainingBytes, nil
}

func makeChanges(dir *os.File, file *os.File) []unix.Kevent_t {
	changes := make([]unix.Kevent_t, 1)
	changes[0] = unix.Kevent_t{
		Ident:  uint64(dir.Fd()),
		Filter: unix.EVFILT_VNODE,
		Flags:  unix.EV_ADD | unix.EV_ENABLE | unix.EV_ONESHOT,
		Fflags: unix.NOTE_DELETE | unix.NOTE_WRITE | unix.NOTE_EXTEND | unix.NOTE_ATTRIB | unix.NOTE_LINK | unix.NOTE_RENAME | unix.NOTE_REVOKE,
		Data:   0,
		Udata:  nil,
	}
	if file != nil {
		changes = append(changes, unix.Kevent_t{
			Ident:  uint64(file.Fd()),
			Filter: unix.EVFILT_VNODE,
			Flags:  unix.EV_ADD | unix.EV_ENABLE | unix.EV_ONESHOT,
			Fflags: unix.NOTE_DELETE | unix.NOTE_WRITE | unix.NOTE_EXTEND | unix.NOTE_ATTRIB | unix.NOTE_LINK | unix.NOTE_RENAME | unix.NOTE_REVOKE,
			Data:   0,
			Udata:  nil,
		})
	}
	return changes
}

type simpleLogger interface {
	Debug(format string, a ...interface{})
}

func stripLines(data []byte) ([]byte, []string) {
	newline := []byte("\n")
	result := make([]string, 0)
	lines := bytes.SplitAfter(data, newline)
	for i, line := range lines {
		if bytes.HasSuffix(line, newline) {
			line = bytes.TrimSuffix(line, newline)
			line = bytes.TrimSuffix(line, []byte("\r")) // needed for Windows?
			result = append(result, string(line))
		} else {
			if i != len(lines)-1 {
				fmt.Fprintf(os.Stderr, "Unexpected error while splitting log data into lines. This is a bug.\n")
				os.Exit(-1)
			}
			return line, result
		}
	}
	return make([]byte, 0), result
}
