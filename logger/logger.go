package logger

import (
	"fmt"
	"time"
)

type logger struct {
	debugEnabled bool
}

func New(debugEnabled bool) *logger {
	return &logger{
		debugEnabled: debugEnabled,
	}
}

func (log *logger) Debug(format string, a ...interface{}) {
	if log.debugEnabled {
		fmt.Printf("%v %v", time.Now().Format("2006-01-02 15:04:05.0000"), fmt.Sprintf(format, a...))
	}
}
