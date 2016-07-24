// +build !linux,!windows

package tailer

import (
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
	// TODO (1): If there was an error, this might hang forever, as the loop reading from 'done' has stopped.
	// TODO (2): Will panic if Close() is called multiple times, because writing to closed 'done' channel.
	f.done <- true
	close(f.done)
	close(f.lines)
	close(f.errors)
}

func (f *fileTailer) Lines() chan string {
	return f.lines
}

func (f *fileTailer) Errors() chan error {
	return f.errors
}

func RunFileTailer(path string, readall bool, logger simpleLogger) Tailer {
	linesChannel := make(chan string)
	doneChannel := make(chan bool)
	errorChannel := make(chan error)
	go func() {
		var (
			dir, file *os.File
			kq        int
		)
		abspath, err := filepath.Abs(path)
		if err != nil {
			closeAll(dir, file, kq)
			errorChannel <- fmt.Errorf("%v: %v", path, err.Error())
			return
		}

		file, err = os.Open(abspath)
		if err != nil {
			closeAll(dir, file, kq)
			errorChannel <- fmt.Errorf("%v: %v", abspath, err.Error())
			return
		}

		if !readall {
			_, err := file.Seek(0, os.SEEK_END)
			if err != nil {
				closeAll(dir, file, kq)
				errorChannel <- fmt.Errorf("%v: Error while seeking to the end of file: %v", path, err.Error())
				return
			}
		}

		reader := NewBufferedLineReader(file, linesChannel)

		dir, err = os.Open(filepath.Dir(abspath))
		if err != nil {
			closeAll(dir, file, kq)
			errorChannel <- fmt.Errorf("%v: %v", filepath.Dir(abspath), err.Error())
			return
		}

		kq, err = unix.Kqueue()
		if kq == -1 || err != nil {
			closeAll(dir, file, kq)
			errorChannel <- fmt.Errorf("Failed to watch %v: %v", abspath, err.Error())
			return
		}

		timeout := unix.NsecToTimespec((1 * time.Second).Nanoseconds())
		zeroTimeout := unix.NsecToTimespec(0) // timeout zero means non-blocking kevent() call

		// Register for events on dir and file.
		_, err = unix.Kevent(kq, []unix.Kevent_t{makeEvent(dir), makeEvent(file)}, nil, &zeroTimeout)
		if err != nil {
			closeAll(dir, file, kq)
			errorChannel <- fmt.Errorf("Failed to watch %v: %v", abspath, err.Error())
			return
		}

		if readall {
			err = reader.ProcessAvailableLines()
			if err != nil {
				closeAll(dir, file, kq)
				errorChannel <- fmt.Errorf("Failed to watch %v: %v", abspath, err.Error())
				return
			}
		}

		events := make([]unix.Kevent_t, 10)

		for {
			select {
			case <-doneChannel:
				closeAll(dir, file, kq)
				return
			default:
				n, err := unix.Kevent(kq, nil, events, &timeout)
				if err != nil {
					closeAll(dir, file, kq)
					errorChannel <- fmt.Errorf("Failed to watch %v: %v", abspath, err.Error())
					return
				}
				logger.Debug("File system watcher got %v events:\n", n)
				for i := 0; i < n; i++ {
					logger.Debug(" * %s\n", event2string(dir, file, events[i]))
				}
				for i := 0; i < n; i++ {
					// Handle truncate events.
					if file != nil && events[i].Ident == uint64(file.Fd()) {
						if events[i].Fflags&unix.NOTE_ATTRIB == unix.NOTE_ATTRIB {
							_, err := file.Seek(0, os.SEEK_SET)
							if err != nil {
								closeAll(dir, file, kq)
								errorChannel <- fmt.Errorf("Failed to seek to the beginning of file %v: %v", abspath, err.Error())
								return
							}
						}
					}
				}
				for i := 0; i < n; i++ {
					// Handle write events.
					if file != nil && events[i].Ident == uint64(file.Fd()) {
						if events[i].Fflags&unix.NOTE_WRITE == unix.NOTE_WRITE {
							err = reader.ProcessAvailableLines()
							if err != nil {
								closeAll(dir, file, kq)
								errorChannel <- fmt.Errorf("Failed to watch %v: %v", abspath, err.Error())
								return
							}
						}
					}
				}
				for i := 0; i < n; i++ {
					// Handle move and delete events.
					if file != nil && events[i].Ident == uint64(file.Fd()) {
						if events[i].Fflags&unix.NOTE_DELETE == unix.NOTE_DELETE || events[i].Fflags&unix.NOTE_RENAME == unix.NOTE_RENAME {
							file.Close() // closing the fd will automatically remove event from kq.
							file = nil
							reader = nil
						}
					}
				}
				for i := 0; i < n; i++ {
					// Handle create events.
					if file == nil && events[i].Ident == uint64(dir.Fd()) {
						if events[i].Fflags&unix.NOTE_WRITE == unix.NOTE_WRITE {
							file, err = os.Open(abspath)
							if err == nil {
								_, err = unix.Kevent(kq, []unix.Kevent_t{makeEvent(file)}, nil, &zeroTimeout)
								if err != nil {
									closeAll(dir, file, kq)
									errorChannel <- fmt.Errorf("Failed to watch %v: %v", abspath, err.Error())
									return
								}
								reader = NewBufferedLineReader(file, linesChannel)
								err = reader.ProcessAvailableLines()
								if err != nil {
									closeAll(dir, file, kq)
									errorChannel <- fmt.Errorf("Failed to watch %v: %v", abspath, err.Error())
									return
								}
							}
							// If file could not be opened, the CREATE event was for another file, we ignore this.
						}
					}
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

func makeEvent(file *os.File) unix.Kevent_t {

	// Note about the EV_CLEAR flag:
	//
	// The NOTE_WRITE event is triggered by the first write to the file after register, and remains set.
	// This means that we continue to receive the event indefinitely.
	//
	// There are two flags to stop receiving the event over and over again:
	//
	// * EV_ONESHOT: This suppresses consecutive events of the same type. However, that means that means that
	//               we don't receive new WRITE events even if new lines are written to the file.
	//               Therefore we cannot use EV_ONESHOT.
	// * EV_CLEAR:   This resets the state after the event, so that an event is only delivered once for each write.
	//               (Actually it could be less than once per write, since events are coalesced.)
	//               This is our desired behaviour.
	//
	// See also http://benno.id.au/blog/2008/05/15/simplefilemon

	return unix.Kevent_t{
		Ident:  uint64(file.Fd()),
		Filter: unix.EVFILT_VNODE,           // File modification and deletion events
		Flags:  unix.EV_ADD | unix.EV_CLEAR, // Add a new event, automatically enabled unless EV_DISABLE is specified
		Fflags: unix.NOTE_DELETE | unix.NOTE_WRITE | unix.NOTE_EXTEND | unix.NOTE_ATTRIB | unix.NOTE_LINK | unix.NOTE_RENAME | unix.NOTE_REVOKE,
		Data:   0,
		Udata:  nil,
	}
}

func closeAll(dir *os.File, file *os.File, kq int) {
	if dir != nil {
		dir.Close()
	}
	if file != nil {
		file.Close()
	}
	if kq != 0 {
		unix.Close(kq)
	}
}

type simpleLogger interface {
	Debug(format string, a ...interface{})
}

func event2string(dir *os.File, file *os.File, event unix.Kevent_t) string {
	result := "event"
	if dir != nil && event.Ident == uint64(dir.Fd()) {
		result = fmt.Sprintf("%v for logdir with fflags", result)
	} else if file != nil && event.Ident == uint64(file.Fd()) {
		result = fmt.Sprintf("%v for logfile with fflags", result)
	} else {
		result = fmt.Sprintf("%s for unknown fd=%v with fflags", result, event.Ident)
	}

	if event.Fflags&unix.NOTE_DELETE == unix.NOTE_DELETE {
		result = fmt.Sprintf("%v NOTE_DELETE", result)
	}
	if event.Fflags&unix.NOTE_WRITE == unix.NOTE_WRITE {
		result = fmt.Sprintf("%v NOTE_WRITE", result)
	}
	if event.Fflags&unix.NOTE_EXTEND == unix.NOTE_EXTEND {
		result = fmt.Sprintf("%v NOTE_EXTEND", result)
	}
	if event.Fflags&unix.NOTE_ATTRIB == unix.NOTE_ATTRIB {
		result = fmt.Sprintf("%v NOTE_ATTRIB", result)
	}
	if event.Fflags&unix.NOTE_LINK == unix.NOTE_LINK {
		result = fmt.Sprintf("%v NOTE_LINK", result)
	}
	if event.Fflags&unix.NOTE_RENAME == unix.NOTE_RENAME {
		result = fmt.Sprintf("%v NOTE_RENAME", result)
	}
	if event.Fflags&unix.NOTE_REVOKE == unix.NOTE_REVOKE {
		result = fmt.Sprintf("%v NOTE_REVOKE", result)
	}
	return result
}
