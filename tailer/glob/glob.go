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

package glob

import (
	"fmt"
	"path/filepath"
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
		return "", fmt.Errorf("%q: failed to finnd absolute path for glob pattern: %v", pattern, err)
	}
	result = Glob(absglob)
	if containsWildcards(result.Dir()) {
		return "", fmt.Errorf("%q: wildcards are only allowed in the file name, but not in the directory path", pattern)
	}
	return result, nil
}

func (g Glob) Dir() string {
	return filepath.Dir(string(g))
}

func (g Glob) Match(path string) bool {
	matched, _ := filepath.Match(string(g), path)
	return matched
}

// The file tailer implementation switched from watching single paths to globs,
// but the rest of grok_exporter just supports single files.
// FromPath creates a Glob from a file path, so that we can use the new file
// tailers but be sure only a single file is watched.
func FromPath(path string) (Glob, error) {
	if containsWildcards(path) {
		return "", fmt.Errorf("%v: illegal file name", path)
	}
	return Parse(path)
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
			return false
		}
		escaped = false
	}
	return false
}
