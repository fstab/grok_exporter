package logger

import "fmt"

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
		fmt.Printf(format, a...)
	}
}
