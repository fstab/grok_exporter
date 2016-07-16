package tailer

import (
	"fmt"
	"golang.org/x/sys/unix"
	"log"
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
// We suspect the reason to be a race condition: When logrotate moves a file but the file is re-created immediately
// fsnotify will not trigger a CREATE event with the kqueue() and kevent() implementation on macOS.
// To solve that, we hacked a simplified file watcher for macOS that compares inodes to learn if a file was re-created.
func RunFileTailer(path string, readall bool, logger simpleLogger) Tailer {
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

		lineReader := NewLineReader(file, linesChannel)

		if !readall {
			err = file.SeekEnd()
			if err != nil {
				errorChannel <- err
				return
			}
		}

		kqueueEventChannel, kqueueDoneChannel, err := runKeventLoop(abspath, readall, logger)
		if err != nil {
			errorChannel <- err
			return
		}

		for {
			select {
			case <-doneChannel:
				kqueueDoneChannel <- true
				if file.IsOpen() {
					file.Close()
				}
				return
			case <-kqueueEventChannel:
				if file.IsClosed() {
					err = file.Open()
					if err != nil {
						continue // File not found. Wait for next event.
					}
				}
				if file.WasMoved() {
					err = lineReader.ProcessAvailableLines() // remaining lines in old file
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
				if file.IsTruncated() {
					err = file.SeekStart()
					if err != nil {
						errorChannel <- err
						return
					}
				}
				err = lineReader.ProcessAvailableLines()
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

func runKeventLoop(abspath string, readall bool, logger simpleLogger) (chan int, chan bool, error) {

	dir, err := os.Open(filepath.Dir(abspath))
	if err != nil {
		return nil, nil, fmt.Errorf("%v: %v", abspath, err.Error())
	}

	kq, err := unix.Kqueue()
	if kq == -1 || err != nil {
		dir.Close()
		return nil, nil, fmt.Errorf("Failed to watch %v: %v", abspath, err.Error())
	}

	kqueueEventChannel := make(chan int, 10)
	kqueueDoneChannel := make(chan bool)

	go func() {
		defer unix.Close(kq)
		defer dir.Close()
		defer close(kqueueEventChannel)
		defer close(kqueueDoneChannel)
		// Timeout is needed so we read from kqueueDoneChannel from time to time.
		timeout := unix.NsecToTimespec((500 * time.Millisecond).Nanoseconds())
		zeroTimeout := unix.NsecToTimespec(0) // timeout zero means non-blocking kevent() call
		events := make([]unix.Kevent_t, 10)
		if readall {
			kqueueEventChannel <- 1 // Simulate event, so that pre-existing lines are read.
		}
		for {
			f, _ := os.Open(abspath) // f may be nil if abspath does not exist. In that case we just listen for events on dir.
			logger.Debug("Waiting for file system events.\n")
			n, err := unix.Kevent(kq, makeKeventFilter(dir, f), events, &timeout)
			if err != nil {
				// If we cannot call kevent(), there's not much we can do.
				log.Fatalf("%v: kevent() failed: %v", dir.Name(), err.Error())
			}
			logger.Debug("Got %v file system events.\n", n)

			// Remove the events, so we don't see them again with the next kevent() call.
			for i := 0; i < n; i++ {
				events[i].Flags = unix.EV_DELETE
				_, err = unix.Kevent(kq, events[i:i+1], nil, &zeroTimeout)
				if err != nil {
					log.Fatalf("Failed to remove event (ident=%v, fflags=%v) from kqueue: %v", events[i].Ident, events[i].Fflags, err.Error())
				}
			}
			if f != nil {
				f.Close() // re-open with each loop, because logfile might be moved and re-created by logrotate.
			}
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
	return kqueueEventChannel, kqueueDoneChannel, nil
}

func makeKeventFilter(dir *os.File, file *os.File) []unix.Kevent_t {
	newKevent := func(fd uintptr) unix.Kevent_t {
		return unix.Kevent_t{
			Ident:  uint64(fd),
			Filter: unix.EVFILT_VNODE, // File modification and deletion events
			Flags:  unix.EV_ADD,       // Add a new event, automatically enabled unless EV_DISABLE is specified
			Fflags: unix.NOTE_DELETE | unix.NOTE_WRITE | unix.NOTE_EXTEND | unix.NOTE_ATTRIB | unix.NOTE_LINK | unix.NOTE_RENAME | unix.NOTE_REVOKE,
			Data:   0,
			Udata:  nil,
		}
	}
	changes := make([]unix.Kevent_t, 1)
	changes[0] = newKevent(dir.Fd())
	if file != nil {
		changes = append(changes, newKevent(file.Fd()))
	}
	return changes
}

type simpleLogger interface {
	Debug(format string, a ...interface{})
}
