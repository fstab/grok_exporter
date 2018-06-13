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
	"unicode"
)

// Returns a copy of input with the all occurrences of regex substituted with replacement.
// Replacement may contain numeric capture group references \1 and named capture group references \k<name>.
// Syntax is like Ruby's String.gsub(), see https://ruby-doc.org/core-2.1.4/String.html#method-i-gsub
func (regex *Regex) Gsub(input, replacement string) (string, error) {
	tokens, err := tokenize([]rune(replacement))
	if err != nil {
		return "", fmt.Errorf("syntax error in replacement string: %v", err)
	}
	replacements := make([]*replacementRegion, 0, 4)
	searchResult, err := regex.Search(input)
	offset := 0
	for ; err == nil && searchResult.IsMatch(); searchResult, err = regex.searchWithOffset(input, offset) {
		replacements = append(replacements, &replacementRegion{
			start:       searchResult.startPos(),
			end:         searchResult.endPos(),
			replacement: createReplacementString(searchResult, tokens),
		})
		offset = searchResult.endPos()
	}
	err = assertNotOverlapping(replacements) // should never happen, but keep it for debugging
	if err != nil {
		return "", err
	}
	result := ""
	pos := 0
	for _, r := range replacements {
		result += input[pos:r.start]
		result += r.replacement
		pos = r.end
	}
	result += input[pos:]
	return result, nil
}

func ValidateReplacementString(replacement string) error {
	_, err := tokenize([]rune(replacement))
	return err
}

type token struct {
	captureGroupNumber int
	captureGroupName   string
	characters         string
}

func (t *token) isCaptureGroupName() bool {
	return len(t.captureGroupName) > 0
}

func (t *token) isCaptureGroupNumber() bool {
	return !t.isCaptureGroupName() && !t.isCharacters()
}

func (t *token) isCharacters() bool {
	return len(t.characters) > 0
}

func (t *token) String() string {
	switch {
	case t.isCaptureGroupName():
		return fmt.Sprintf("capture group name: '%v'", t.captureGroupName)
	case t.isCaptureGroupNumber():
		return fmt.Sprintf("capture group number: '%v'", t.captureGroupNumber)
	case t.isCharacters():
		return fmt.Sprintf("characters: '%v'", t.characters)
	default:
		return fmt.Sprintf("unexpected")
	}
}

type replacementRegion struct {
	start       int
	end         int
	replacement string
}

func (r *replacementRegion) String() string {
	return fmt.Sprintf("[%v:%v] %v", r.start, r.end, r.replacement)
}

func createReplacementString(searchResult *SearchResult, tokens []*token) string {
	result := ""
	for _, token := range tokens {
		switch {
		case token.isCharacters():
			result += token.characters
		case token.isCaptureGroupName():
			s, err := searchResult.GetCaptureGroupByName(token.captureGroupName)
			if err == nil {
				result += s
			} else {
				result += fmt.Sprintf("\\k<%v>", token.captureGroupName)
			}
		case token.isCaptureGroupNumber():
			s, err := searchResult.GetCaptureGroupByNumber(token.captureGroupNumber)
			if err == nil {
				result += s
			} else {
				result += fmt.Sprintf("\\%v", token.captureGroupNumber)
			}
		}
	}
	return result
}

func assertNotOverlapping(replacements []*replacementRegion) error {
	for i := 0; i < len(replacements)-1; i++ {
		if replacements[i].end > replacements[i+1].start {
			return fmt.Errorf("illegal state while processing replaceAll: overlapping regions [%v:%v] [%v:%v]", replacements[i].start, replacements[i].end, replacements[i+1].start, replacements[i+1].end)
		}
	}
	return nil
}

func tokenize(s []rune) ([]*token, error) {
	result := make([]*token, 0, len(s))
	for pos := 0; pos < len(s); pos++ {
		if s[pos] != '\\' {
			result = append(result, &token{characters: string(s[pos])})
		} else {
			if pos == len(s)-1 {
				return nil, fmt.Errorf("invalid escape sequence")
			} else {
				pos++
				nextChar := s[pos]
				switch {
				case nextChar == '\\':
					result = append(result, &token{characters: "\\"})
				case unicode.IsDigit(nextChar):
					captureGroupNumber := int(s[pos] - '0')
					for ; pos < len(s)-1 && unicode.IsDigit(s[pos+1]); pos++ {
						captureGroupNumber = captureGroupNumber*10 + int(s[pos+1]-'0')
					}
					result = append(result, &token{captureGroupNumber: captureGroupNumber})
				case nextChar == 'k':
					if pos == len(s)-1 || s[pos+1] != '<' {
						return nil, fmt.Errorf("invalid escape sequence")
					}
					pos += 2
					captureGroupName := ""
					for ; pos < len(s) && s[pos] != '>'; pos++ {
						captureGroupName += string(s[pos])
					}
					if len(captureGroupName) == 0 {
						return nil, fmt.Errorf("invalid escape sequence")
					}
					result = append(result, &token{captureGroupName: captureGroupName})
				default:
					return nil, fmt.Errorf("invalid escape sequence")
				}
			}
		}
	}
	return result, nil
}
