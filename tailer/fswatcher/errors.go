// Copyright 2018 The grok_exporter Authors
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

import "fmt"

type ErrorType int

const (
	NotSpecified = iota
	DirectoryNotFound
	FileNotFound
)

type Error interface {
	Cause() error
	Type() ErrorType
	error
}

type tailerError struct {
	msg       string
	cause     error
	errorType ErrorType
}

func NewErrorf(errorType ErrorType, cause error, format string, a ...interface{}) Error {
	return NewError(errorType, cause, fmt.Sprintf(format, a...))
}

func NewError(errorType ErrorType, cause error, msg string) Error {
	return tailerError{
		msg:       msg,
		cause:     cause,
		errorType: errorType,
	}
}

func (e tailerError) Cause() error {
	return e.cause
}

func (e tailerError) Type() ErrorType {
	return e.errorType
}

func (e tailerError) Error() string {
	if len(e.msg) > 0 && e.cause != nil {
		return fmt.Sprintf("%v: %v", e.msg, e.cause)
	} else if len(e.msg) > 0 {
		return e.msg
	} else if e.cause != nil {
		return e.cause.Error()
	} else {
		return "unknown error"
	}
}
