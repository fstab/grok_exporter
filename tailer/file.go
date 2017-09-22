// Copyright 2016-2017 The grok_exporter Authors
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

// +build !windows

package tailer

import (
	"fmt"
	"io"
	"os"
)

type File struct {
	*os.File // On Unix we can use a regular os.File, but on Windows we replace this with a custom implementation.
}

func open(abspath string) (*File, error) {
	file, err := os.Open(abspath)
	if err != nil {
		return nil, err
	}
	return &File{file}, nil
}

func (file *File) CheckTruncated() (bool, error) {
	currentPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return false, fmt.Errorf("%v: Seek() failed: %v", file.Name(), err.Error())
	}
	fileInfo, err := file.Stat()
	if err != nil {
		return false, fmt.Errorf("%v: Stat() failed: %v", file.Name(), err.Error())
	}
	return currentPos > fileInfo.Size(), nil
}

func (file *File) CheckMoved() (bool, error) {
	self, err := file.File.Stat()
	if err != nil {
		return false, err
	}
	onDisk, err := os.Stat(file.Name())
	if err != nil {
		return true, nil // probably file not found, which means it was moved.
	}
	return !os.SameFile(self, onDisk), nil
}
