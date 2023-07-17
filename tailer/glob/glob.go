// Copyright 2018-2020 The grok_exporter Authors
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

package glob

import (
	"fmt"
	"path/filepath"
	"github.com/bmatcuk/doublestar/v4"
	"runtime"
)

type Glob string

func Parse(pattern string) (Glob, error) {
	var (
		result  Glob
		absglob string
		err     error
	)
	if !IsPatternValid(pattern) {
		return "", fmt.Errorf("%q: invalid glob pattern", pattern)
	}
	absglob, err = filepath.Abs(pattern)
	if err != nil {
		return "", fmt.Errorf("%q: failed to find absolute path for glob pattern: %v", pattern, err)
	}
	result = Glob(absglob)
	return result, nil
}

func (g Glob) Dir() string {
	return filepath.Dir(string(g))
}

func (g Glob) Match(path string) bool {
	matched, _ := doublestar.Match(string(g), path)
	return matched
}

func containsWildcards(pattern string) bool {
	p := []rune(pattern)
	escaped := false // p[i] is escaped by '\\'
	for i := 0; i < len(p); i++ {
		if p[i] == '\\' && !escaped && runtime.GOOS != "windows" {
			escaped = true
			continue
		}
		if !escaped && (p[i] == '[' || p[i] == '*' || p[i] == '?') {
			return true
		}
		escaped = false
	}
	return false
}
