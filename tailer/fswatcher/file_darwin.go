// Copyright 2019 The grok_exporter Authors
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
)

// On macOS, we keep dirs open, so we use *os.File.
type Dir struct {
	file *os.File
}

func (d *Dir) Path() string {
	return d.file.Name()
}

func (d *Dir) ls() ([]os.FileInfo, Error) {
	var (
		fileInfos []os.FileInfo
		err       error
	)
	_, err = d.file.Seek(0, io.SeekStart)
	if err != nil {
		return nil, NewError(NotSpecified, os.NewSyscallError("seek", err), d.file.Name())
	}
	fileInfos, err = d.file.Readdir(-1)
	if err != nil {
		return nil, NewError(NotSpecified, os.NewSyscallError("readdir", err), d.file.Name())
	}
	return fileInfos, nil
}

func NewFile(orig *os.File, newPath string) (*os.File, error) {
	// Why do we create a new file descriptor here with Dup()?
	// Because os.File has a finalizer closing the file when the object is garbage collected.
	// This will close orig.Fd() as soon as the GC runs.
	fd, err := syscall.Dup(int(orig.Fd()))
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), newPath), nil
}

func open(path string) (*os.File, Error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, NewError(FileNotFound, os.NewSyscallError("open", err), path)
		} else {
			return nil, NewError(NotSpecified, os.NewSyscallError("open", err), path)
		}
	}
	return file, nil
}
