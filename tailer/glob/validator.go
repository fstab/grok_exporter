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
	"runtime"
)

type charClassItem int // produced by the lexer lexing character classes (like [a-z]) in a pattern

const (
	charItem  charClassItem = iota // regular character, including escaped special characters
	minusItem                      // minus symbol in a character range, like in 'a-z'
)

// If IsPatternValid(pattern) is true, filepath.Match(pattern, name) will not return an error.
// See also https://go-review.googlesource.com/c/go/+/143477
func IsPatternValid(pattern string) bool {
	p := []rune(pattern)
	charClassItems := make([]charClassItem, 0) // captures content of '[...]'
	insideCharClass := false                   // we saw a '[' but no ']' yet
	escaped := false                           // p[i] is escaped by '\\'
	for i := 0; i < len(p); i++ {
		switch {
		case p[i] == '\\' && !escaped && runtime.GOOS != "windows":
			escaped = true
			continue
		case !insideCharClass && p[i] == '[' && !escaped:
			insideCharClass = true
			if i+1 < len(p) && p[i+1] == '^' {
				i++ // It doesn't matter if the char class starts with '[' or '[^'.
			}
		case insideCharClass && !escaped && p[i] == '-':
			charClassItems = append(charClassItems, minusItem)
		case insideCharClass && !escaped && p[i] == ']':
			if !isCharClassValid(charClassItems) {
				return false
			}
			charClassItems = charClassItems[:0]
			insideCharClass = false
		case insideCharClass:
			charClassItems = append(charClassItems, charItem)
		}
		escaped = false
	}
	return !escaped && !insideCharClass
}

func isCharClassValid(charClassItems []charClassItem) bool {
	if len(charClassItems) == 0 {
		return false
	}
	for i := 0; i < len(charClassItems); i++ {
		if charClassItems[i] == minusItem {
			return false
		}
		if i+1 < len(charClassItems) {
			if charClassItems[i+1] == minusItem {
				i += 2
				if i >= len(charClassItems) || charClassItems[i] == minusItem {
					return false
				}
			}
		}
	}
	return true
}
