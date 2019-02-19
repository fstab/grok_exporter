package fswatcher

import (
	"github.com/sirupsen/logrus"
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
