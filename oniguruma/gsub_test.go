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

package oniguruma

import (
	"fmt"
	"testing"
)

func TestGsub(t *testing.T) {
	for _, data := range []struct {
		input          string
		regex          string
		replacement    string
		expectedResult string
	}{
		// Examples from Ruby's gsub doc: https://ruby-doc.org/core-2.1.4/String.html#method-i-gsub
		{input: "hello", regex: "[aeiou]", replacement: "*", expectedResult: "h*ll*"},
		{input: "hello", regex: "([aeiou])", replacement: "<\\1>", expectedResult: "h<e>ll<o>"},
		{input: "hello", regex: "(?<foo>[aeiou])", replacement: "{\\k<foo>}", expectedResult: "h{e}ll{o}"},

		// Other tests
		{input: "abaabca", regex: "b(?!a)", replacement: ".", expectedResult: "abaa.ca"},
		{input: "aaaaa", regex: "aa", replacement: "..", expectedResult: "....a"},
		{input: "", regex: ".", replacement: "*", expectedResult: ""},

		// matches empty string
		// The following is the same behavior as Ruby's puts "abc".gsub(/.*/, ".")
		{input: "abc", regex: ".*", replacement: ".", expectedResult: ".."},
		// The following is the same behavior as Ruby's puts "abc".gsub(/.*?/, ".")
		{input: "abc", regex: ".*?", replacement: ".", expectedResult: ".a.b.c."},
	} {
		r, err := Compile(data.regex)
		if err != nil {
			t.Fatalf("failed to compile regex %v: %v", data.regex, err)
		}
		result, err := r.Gsub(data.input, data.replacement)
		if err != nil {
			t.Fatalf("failed to apply replace '%v' with '%v': %v", data.regex, data.replacement, err)
		}
		fmt.Printf("input: %v, regex: %v, replacement: %v, result: %v\n", data.input, data.regex, data.replacement, result)
		if result != data.expectedResult {
			t.Fatalf("input: %v, regex: %v, replacement: %v, result: %v, expectedResult: %v\n", data.input, data.regex, data.replacement, result, data.expectedResult)
		}
	}
}

func TestTokenize(t *testing.T) {
	tokens, err := tokenize([]rune("hello\\k<bb>\\k<a>\\\\\\0z"))
	if err != nil {
		t.Fatalf("unexpected error in tokenize(): %v", err)
	}
	i := 0
	for ; i < len("hello"); i++ {
		if !tokens[i].isCharacters() {
			t.Fatalf("unexpected token at position %v: %v", i, tokens[i])
		}
	}
	for _, c := range []string{"bb", "a"} {
		if !tokens[i].isCaptureGroupName() || tokens[i].captureGroupName != c {
			t.Fatalf("unexpected token at position %v: %v", i, tokens[i])
		}
		i++
	}
	if !tokens[i].isCharacters() || tokens[i].characters != "\\" {
		fmt.Printf("characters=%v\n", tokens[i].characters)
		t.Fatalf("unexpected token at position %v: %v", i, tokens[i])
	}
	i++
	if !tokens[i].isCaptureGroupNumber() || tokens[i].captureGroupNumber != 0 {
		t.Fatalf("unexpected token at position %v: %v", i, tokens[i])
	}
	i++
	if !tokens[i].isCharacters() || tokens[i].characters != "z" {
		t.Fatalf("unexpected token at position %v: %v", i, tokens[i])
	}
}
