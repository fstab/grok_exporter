package tailer

import (
	"fmt"
	"io"
	"log"
	"os"
)

type tailedFile struct {
	path string
	file *os.File
}

func NewTailedFile(path string) (*tailedFile, error) {
	result := &tailedFile{
		path: path,
	}
	err := result.Open()
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (t *tailedFile) SeekEnd() error {
	if t.IsClosed() {
		log.Fatalf("%v: Cannot call SeekEnd() on a closed file.\n", t.path)
	}
	_, err := t.file.Seek(0, os.SEEK_END)
	if err != nil {
		return fmt.Errorf("%v: Error while seeking to the end of file: %v", t.path, err.Error())
	}
	return nil
}

func (t *tailedFile) SeekStart() error {
	if t.IsClosed() {
		log.Fatalf("%v: Cannot call SeekStart() on a closed file.\n", t.path)
	}
	_, err := t.file.Seek(0, os.SEEK_SET)
	if err != nil {
		return fmt.Errorf("%v: Error while seeking to the beginning of file: %v", t.path, err.Error())
	}
	return nil
}

func (t *tailedFile) Close() error {
	if t.IsClosed() {
		log.Fatalf("%v: Cannot call Close() on a closed file.\n", t.path)
	}
	err := t.file.Close()
	t.file = nil
	return err
}

func (t *tailedFile) IsClosed() bool {
	return t.file == nil
}

func (t *tailedFile) IsOpen() bool {
	return !t.IsClosed()
}

func (t *tailedFile) Open() error {
	if t.IsOpen() {
		log.Fatalf("%v: Cannot call Open() on an open file.\n", t.path)
	}
	file, err := os.Open(t.path)
	if err != nil {
		return fmt.Errorf("%v: %v", t.path, err.Error())
	}
	t.file = file
	return nil
}

func (t *tailedFile) Read2EOF() ([]byte, error) {
	if t.IsClosed() {
		log.Fatalf("%v: Cannot call Read2EOF() on a closed file.\n", t.path)
	}
	result := make([]byte, 0)
	buf := make([]byte, 512)
	for {
		n, err := t.file.Read(buf)
		if err != nil {
			if err == io.EOF {
				return result, nil
			} else {
				return nil, fmt.Errorf("Error reading from %v: %v", t.file.Name(), err.Error())
			}
		}
		result = append(result, buf[0:n]...)
	}
}

func (t *tailedFile) IsTruncated() (bool, error) {
	if t.IsClosed() {
		log.Fatalf("%v: Cannot call IsTruncated() on a closed file.\n", t.path)
	}
	currentPos, err := t.file.Seek(0, os.SEEK_CUR)
	if err != nil {
		return false, fmt.Errorf("%v: Failed to get current read position: %v", t.path, err.Error())
	}
	fileInfo, err := t.file.Stat()
	if err != nil {
		return false, fmt.Errorf("%v: Failed to get file info: %v", t.path, err.Error())
	}
	return currentPos > fileInfo.Size(), nil
}
