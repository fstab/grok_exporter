package tailer

import (
	"fmt"
	"io"
	"log"
	"os"
)

// The windows file must not stay open while logrotate tries to remove it.
// So we close it after each read and re-open it with the next read.
type tailedFile struct {
	path       string
	currentPos int64
	isOpen     bool
}

func NewTailedFile(path string) (*tailedFile, error) {
	err := tryRead(path)
	if err != nil {
		return nil, err
	}
	return &tailedFile{
		path:       path,
		currentPos: 0,
		isOpen:     true,
	}, nil
}

func tryRead(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("%v: %v", path, err.Error())
	}
	err = file.Close()
	if err != nil {
		return fmt.Errorf("%v: %v", path, err.Error())
	}
	return nil
}

func (t *tailedFile) SeekEnd() error {
	if t.IsClosed() {
		log.Fatalf("%v: Cannot call SeekEnd() on a closed file.\n", t.path)
	}
	file, err := os.Open(t.path)
	if err != nil {
		return fmt.Errorf("%v: %v", t.path, err.Error())
	}
	defer file.Close()
	_, err = file.Seek(0, os.SEEK_END)
	if err != nil {
		return fmt.Errorf("%v: Error while seeking to the end of file: %v", t.path, err.Error())
	}
	t.currentPos, err = file.Seek(0, os.SEEK_CUR)
	if err != nil {
		return fmt.Errorf("%v: Failed to get current read position: %v", t.path, err.Error())
	}
	return nil
}

func (t *tailedFile) SeekStart() error {
	if t.IsClosed() {
		log.Fatalf("%v: Cannot call SeekStart() on a closed file.\n", t.path)
	}
	t.currentPos = 0
	return nil
}

func (t *tailedFile) Close() error {
	if t.IsClosed() {
		log.Fatalf("%v: Cannot call Close() on a closed file.\n", t.path)
	}
	t.isOpen = false
	t.currentPos = 0
	return nil
}

func (t *tailedFile) IsClosed() bool {
	return !t.isOpen
}

func (t *tailedFile) IsOpen() bool {
	return t.isOpen
}

func (t *tailedFile) Open() error {
	if t.IsOpen() {
		log.Fatalf("%v: Cannot call Open() on an open file.\n", t.path)
	}
	err := tryRead(t.path)
	if err != nil {
		return err
	}
	t.isOpen = true
	t.currentPos = 0
	return nil
}

func (t *tailedFile) Read2EOF() ([]byte, error) {
	if t.IsClosed() {
		log.Fatalf("%v: Cannot call Read2EOF() on a closed file.\n", t.path)
	}
	file, err := os.Open(t.path)
	if err != nil {
		return nil, fmt.Errorf("%v: %v", t.path, err.Error())
	}
	defer file.Close()
	pos, err := file.Seek(t.currentPos, os.SEEK_SET)
	if err != nil || pos != t.currentPos {
		return nil, fmt.Errorf("%v: Failed to seek to current read position: %v", t.path, err.Error())
	}
	result := make([]byte, 0)
	buf := make([]byte, 512)
	for {
		n, err := file.Read(buf)
		if err != nil {
			if err == io.EOF {
				return result, nil
			} else {
				return nil, fmt.Errorf("Error reading from %v: %v", t.path, err.Error())
			}
		}
		t.currentPos += int64(n)
		result = append(result, buf[0:n]...)
	}
}

func (t *tailedFile) IsTruncated() bool {
	if t.IsClosed() {
		log.Fatalf("%v: Cannot call IsTruncated() on a closed file.\n", t.path)
	}
	file, err := os.Open(t.path)
	if err != nil {
		return false // May happen if file was removed. We treat this as "file was not truncated".
	}
	defer file.Close()
	fileInfo, err := file.Stat()
	if err != nil {
		// We can successfully open the file, but Stat() fails.
		// Maybe this might happen if the filesystem fails, like NFS becomes unreachable.
		// Treat it as a catastrophic error.
		log.Fatalf("%v: %v", t.path, err.Error())
	}
	return t.currentPos > fileInfo.Size()
}
