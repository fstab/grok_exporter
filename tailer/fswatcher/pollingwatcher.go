package fswatcher

import (
	"github.com/sirupsen/logrus"
	"io"
	"time"
)

type pollingWatcher struct {
	pollInterval time.Duration
}

func initPollingWatcher(pollInterval time.Duration) (fswatcher, Error) {
	return &pollingWatcher{
		pollInterval: pollInterval,
	}, nil
}

func (w *pollingWatcher) runFseventProducerLoop() fseventProducerLoop {
	return runPollLoop(w.pollInterval)
}

func (w *pollingWatcher) processEvent(t *fileTailer, fsevent fsevent, log logrus.FieldLogger) Error {
	for _, dir := range t.watchedDirs {
		err := t.syncFilesInDir(dir, true, log)
		if err != nil {
			return err
		}
	}
	for _, file := range t.watchedFiles {
		truncated, err := isTruncated(file.file)
		if err != nil {
			return NewErrorf(NotSpecified, err, "%v: seek() or stat() failed", file.file.Name())
		}
		if truncated {
			_, err = file.file.Seek(0, io.SeekStart)
			if err != nil {
				return NewErrorf(NotSpecified, err, "%v: seek() failed", file.file.Name())
			}
			file.reader.Clear()
		}
		readErr := t.readNewLines(file, log)
		if readErr != nil {
			return readErr
		}
	}
	return nil
}

func (w *pollingWatcher) Close() error {
	return nil
}

func (w *pollingWatcher) watchDir(path string) (*Dir, Error) {
	return newDir(path)
}

func (w *pollingWatcher) unwatchDir(dir *Dir) error {
	return nil
}

func (w *pollingWatcher) watchFile(file fileMeta) Error {
	return nil
}
