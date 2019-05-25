// Copyright 2016-2019 The grok_exporter Authors
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

package fswatcher

import (
	"io"
	"os"
	"syscall"
	"time"
)

// File implementation that closes the file after each operation,
// in order to avoid "the file is in use by another process" errors on Windows.
// Note: Even if we use the FILE_SHARE_DELETE flag while opening the file, it is not save to keep it open.
// If another program deletes the file (like logrotate), the file will remain in the directory until
// grok_exporter closes it. In order to avoid this, we must not keep the file open.
type File struct {
	path          string
	currentPos    int64
	fileIndexLow  uint32
	fileIndexHigh uint32
}

type Dir struct {
	path string
}

func (d *Dir) Path() string {
	return d.path
}

// https://docs.microsoft.com/en-us/windows/desktop/FileIO/listing-the-files-in-a-directory
func (d *Dir) ls() ([]*fileInfo, Error) {
	var (
		ffd      syscall.Win32finddata
		handle   syscall.Handle
		result   []*fileInfo
		filename string
		err      error
	)
	globAll := d.path + `\*`
	globAllP, err := syscall.UTF16PtrFromString(globAll)
	if err != nil {
		return nil, NewErrorf(NotSpecified, os.NewSyscallError("UTF16PtrFromString", err), "%v: invalid directory name", d.path)
	}
	for handle, err = syscall.FindFirstFile(globAllP, &ffd); err == nil; err = syscall.FindNextFile(handle, &ffd) {
		filename = syscall.UTF16ToString(ffd.FileName[:])
		if filename != "." && filename != ".." {
			result = append(result, &fileInfo{
				filename: filename,
				ffd:      ffd,
			})
		}
	}
	if err != syscall.ERROR_NO_MORE_FILES {
		return nil, NewErrorf(NotSpecified, err, "%v: failed to read directory", d.path)
	}
	return result, nil
}

// like os.NewFile()
func NewFile(orig *File, newPath string) (*File, error) {
	return &File{
		path:          newPath,
		currentPos:    orig.currentPos,
		fileIndexLow:  orig.fileIndexLow,
		fileIndexHigh: orig.fileIndexHigh,
	}, nil
}

func open(path string) (*File, Error) {
	file, info, err := openWithBackoff(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return &File{
		path:          path,
		currentPos:    0,
		fileIndexLow:  info.FileIndexLow,
		fileIndexHigh: info.FileIndexHigh,
	}, nil
}

func (f *File) SameFile(other *File) bool {
	return f.fileIndexLow == other.fileIndexLow && f.fileIndexHigh == other.fileIndexHigh
}

func (f *File) Seek(offset int64, whence int) (int64, Error) {
	var (
		file *os.File
		Err  Error
		err  error
		ret  int64
	)
	file, Err = f.reopen()
	if Err != nil {
		return 0, Err
	}
	defer file.Close()
	_, err = file.Seek(f.currentPos, io.SeekStart)
	if err != nil {
		return 0, NewError(NotSpecified, os.NewSyscallError("seek", err), f.Name())
	}
	ret, err = file.Seek(offset, whence)
	if err != nil {
		return 0, NewError(NotSpecified, os.NewSyscallError("seek", err), f.Name())
	}
	f.currentPos, err = file.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, NewError(NotSpecified, os.NewSyscallError("seek", err), f.Name())
	}
	return ret, nil
}

// TODO: error handling is confusing.
// in case of EOF must return io.EOF, because linereader expects that.
// in case of WinFileMoved, must return special error to trigger sync dir.
func (f *File) Read(b []byte) (int, error) {
	file, Err := f.reopen()
	if Err != nil {
		return 0, Err
	}
	defer file.Close()
	_, err := file.Seek(f.currentPos, io.SeekStart)
	if err != nil {
		return 0, NewError(NotSpecified, os.NewSyscallError("seek", err), f.Name())
	}
	result, resultErr := file.Read(b)
	f.currentPos, err = file.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, NewError(NotSpecified, os.NewSyscallError("seek", err), f.Name())
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

func (f *File) Fd() uintptr {
	return 0 // only used for logging, doesn't make sense on Windows.
}

func (f *File) CheckTruncated() (bool, Error) {
	file, Err := f.reopen()
	if Err != nil {
		return false, Err
	}
	defer file.Close()
	fileInfo, err := file.Stat()
	if err != nil {
		return false, NewError(NotSpecified, os.NewSyscallError("stat", err), f.Name())
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
func openWithBackoff(fileName string) (*os.File, syscall.ByHandleFileInformation, Error) {
	var (
		fileNamePtr *uint16
		fileHandle  syscall.Handle
		fileInfo    syscall.ByHandleFileInformation
		err         error
	)
	fileNamePtr, err = syscall.UTF16PtrFromString(fileName)
	if err != nil {
		return nil, fileInfo, NewErrorf(NotSpecified, err, "%v: path contains an illegal character", fileName)
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
		if err != nil {
			time.Sleep(time.Duration(i*125) * time.Millisecond)
			continue
		}
		err = syscall.GetFileInformationByHandle(fileHandle, &fileInfo)
		if err != nil {
			return nil, fileInfo, NewError(NotSpecified, os.NewSyscallError("GetFileInformationByHandle", err), fileName)
		}
		return os.NewFile(uintptr(fileHandle), fileName), fileInfo, nil
	}
	if err == syscall.ERROR_FILE_NOT_FOUND {
		return nil, fileInfo, NewError(FileNotFound, os.NewSyscallError("CreateFile", err), fileName)
	} else {
		return nil, fileInfo, NewErrorf(NotSpecified, os.NewSyscallError("CreateFile", err), "%q: cannot open file", fileName)
	}
}

func (f *File) reopen() (*os.File, Error) {
	file, info, Err := openWithBackoff(f.Name())
	if Err != nil && Err.Type() == FileNotFound {
		return nil, NewErrorf(WinFileRemoved, Err.Cause(), f.Name())
	}
	if f.fileIndexLow != info.FileIndexLow || f.fileIndexHigh != info.FileIndexHigh {
		return nil, NewErrorf(WinFileRemoved, nil, f.Name())
	}
	return file, Err
}
