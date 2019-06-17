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
	"os"
	"syscall"
)

// On Linux, we don't need to keep the directory open, but we need to keep an open watch descriptor handle.
type Dir struct {
	wd   int
	path string
}

func (d *Dir) Path() string {
	return d.path
}

// TODO: Replace with ioutil.Readdir
func (d *Dir) ls() ([]os.FileInfo, Error) {
	var (
		dir       *os.File
		fileInfos []os.FileInfo
		err       error
	)
	dir, err = os.Open(d.path)
	if err != nil {
		return nil, NewErrorf(NotSpecified, err, "%q: failed to open directory", d.path)
	}
	defer dir.Close()
	fileInfos, err = dir.Readdir(-1)
	if err != nil {
		return nil, NewErrorf(NotSpecified, err, "%q: failed to read directory", d.path)
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
