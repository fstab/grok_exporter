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

package tailer

import "fmt"

type Error interface {
	Cause() error
	error
}

type tailerError struct {
	msg   string
	cause error
}

func (e tailerError) Cause() error {
	return e.cause
}

func (e tailerError) Error() string {
	if len(e.msg) > 0 {
		return fmt.Sprintf("%v: %v", e.msg, e.cause.Error())
	} else {
		return e.cause.Error()
	}
}

func newError(msg string, cause error) Error {
	if cause == nil {
		cause = fmt.Errorf("unknown error")
	}
	return tailerError{
		msg:   msg,
		cause: cause,
	}
}
