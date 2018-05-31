// Copyright 2016-2018 The grok_exporter Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tailer

import (
	"fmt"
	"io"
	"os"
	"syscall"
	"time"
)

// File implementation that closes the file after each operation,
// in order to avoid "the file is in use by another process" errors on Windows.
// TODO: As we use the FILE_SHARE_DELETE flag now, do we still need to close the file or can we keep the file open?
type File struct {
	path       string
	currentPos int64
}

func open(path string) (*File, error) {
	file, err := openWithBackoff(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return &File{
		path:       path,
		currentPos: 0,
	}, nil
}

func (f *File) Seek(offset int64, whence int) (int64, error) {
	file, err := openWithBackoff(f.path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	result, resultErr := file.Seek(offset, whence)
	f.currentPos, err = file.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	return result, resultErr
}

func (f *File) Read(b []byte) (int, error) {
	file, err := openWithBackoff(f.path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	_, err = file.Seek(f.currentPos, io.SeekStart)
	if err != nil {
		return 0, err
	}
	result, resultErr := file.Read(b)
	f.currentPos, err = file.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	return result, resultErr
}

func (f *File) Name() string {
	return f.path
}

func (f *File) Close() error {
	// nothing to do
	return nil
}

func (f *File) CheckTruncated() (bool, error) {
	file, err := openWithBackoff(f.path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	fileInfo, err := file.Stat()
	if err != nil {
		return false, fmt.Errorf("%v: Stat() failed: %v", file.Name(), err.Error())
	}
	return f.currentPos > fileInfo.Size(), nil
}

func (f *File) CheckMoved() (bool, error) {
	// As this implementation closes the file after each operation, we don't need special treatment for moved files.
	// We can just pretend the file is never moved.
	return false, nil
}

// The semantics of openWithBackoff() function is similar to os.Open().
// If the file is currently locked by a logger or virus scanner,
// CreateFile() might fail (the logfile is being used by another program).
// We don't give up directly in that case, but back off and try again.
// Only if this error persists about 1 second we give up.
func openWithBackoff(fileName string) (*os.File, error) {
	var (
		fileNamePtr *uint16
		fileHandle  syscall.Handle
		err         error
	)
	fileNamePtr, err = syscall.UTF16PtrFromString(fileName)
	if err != nil {
		return nil, fmt.Errorf("%v: failed to open file: path contains an illegal character", fileName)
	}
	for i := 1; i <= 3; i++ {
		// We cannot use os.Open(), because we must set FILE_SHARE_ flags to avoid
		// "the file is being used by another program" errors.
		// Despite its name, CreateFile() will not create a new file if called with the OPEN_EXISTING flag.
		fileHandle, err = syscall.CreateFile(
			fileNamePtr,
			syscall.GENERIC_READ,
			uint32(syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE),
			nil,
			syscall.OPEN_EXISTING,
			syscall.FILE_ATTRIBUTE_NORMAL,
			0)
		if err == nil {
			return os.NewFile(uintptr(fileHandle), fileName), nil
		}
		time.Sleep(time.Duration(i*125) * time.Millisecond)
	}

	// The fileTailer will check if the file exists using os.IsNotExists(err)
	// Return an error that can be used with os.IsNotExists().
	errno, ok := err.(syscall.Errno)
	if ok {
		return nil, &os.PathError{
			Op:   "open",
			Path: fileName,
			Err:  errno,
		}
	} else {
		return nil, fmt.Errorf("%v: failed to open file: %v", fileName, err)
	}
}
