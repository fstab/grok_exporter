// +build !windows

package tailer

import (
	"fmt"
	"io"
	"log"
	"os"
	"syscall"
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

func (t *tailedFile) WasMoved() bool {
	if t.IsClosed() {
		log.Fatalf("%v: Cannot call WasMoved() on a closed file.\n", t.path)
	}
	fileInfo, err := t.file.Stat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get inode of %v: %v\n", t.path, err.Error())
		os.Exit(-1)
	}
	fileInfoFromFileSystem, err := os.Stat(t.path)
	if err != nil {
		return true // If the path does not exist anymore, the file was moved.
	}
	i1 := inode(fileInfo)
	i2 := inode(fileInfoFromFileSystem)
	fmt.Printf("inode(fileInfo) = %v, inode(fileInfoFromFileSystem) = %v\n", i1, i2)
	return i1 != i2
}

// see github.com/google/mtail
func inode(fileInfo os.FileInfo) uint64 {
	s := fileInfo.Sys()
	if s == nil {
		fmt.Fprintf(os.Stderr, "Failed to get inode of %v.\n", fileInfo.Name())
		os.Exit(-1)
	}
	switch s := s.(type) {
	case *syscall.Stat_t:
		return uint64(s.Ino)
	default:
		fmt.Fprintf(os.Stderr, "Failed to get inode of %v.\n", fileInfo.Name())
		os.Exit(-1)
	}
	return 0
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
