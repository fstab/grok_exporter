// Copyright 2016-2018 The grok_exporter Authors
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

/*
#cgo CFLAGS: -I/usr/local/include
#cgo LDFLAGS: -L/usr/local/lib -lonig
#include <stdlib.h>
#include <string.h>
#include <oniguruma.h>
#include "oniguruma_helper.h"
*/
import "C"
import (
	"errors"
	"fmt"
	"os"
	"unsafe"
)

// TODO: This is the encoding of the logfile. Should be configurable and default to the system encoding.
var encoding = &C.OnigEncodingUTF8 // See the #define statements in oniguruma.h

type Regex struct {
	regex                  C.OnigRegex
	cachedCaptureGroupNums map[string][]C.int
}

type SearchResult struct {
	match  bool
	regex  *Regex
	region *C.OnigRegion
	input  string
}

// Warning: The Oniguruma library is not thread save, it should be used in a single thread.
func init() {
	encodings := []C.OnigEncoding{
		encoding,
	}
	ret := C.oniguruma_helper_initialize(&encodings[0], C.int(len(encodings)))
	if ret != 0 {
		fmt.Fprintf(os.Stderr, "Failed to start grok_exporter: Unexpected error while initializing the Oniguruma regular expression library.\n")
		os.Exit(-1)
	}
}

func Version() string {
	return C.GoString(C.onig_version())
}

func Compile(pattern string) (*Regex, error) {
	result := &Regex{
		cachedCaptureGroupNums: make(map[string][]C.int),
	}
	patternStart, patternEnd := pointers(pattern)
	defer free(patternStart, patternEnd)
	var errorInfo C.OnigErrorInfo
	r := C.onig_new(&result.regex, patternStart, patternEnd, C.ONIG_OPTION_DEFAULT, encoding, C.ONIG_SYNTAX_DEFAULT, &errorInfo)
	if r != C.ONIG_NORMAL {
		return nil, errors.New(errMsgWithInfo(r, &errorInfo))
	}
	return result, nil
}

func (regex *Regex) Free() {
	C.onig_free(regex.regex)
	// Set fields nil so we get an error if regex is used after Free().
	regex.regex = nil
	regex.cachedCaptureGroupNums = nil
}

func (regex *Regex) HasCaptureGroup(name string) bool {
	_, err := regex.getCaptureGroupNums(name)
	return err == nil
}

func (r *Regex) getCaptureGroupNums(name string) ([]C.int, error) {
	cached, ok := r.cachedCaptureGroupNums[name]
	if ok {
		return cached, nil
	}
	nameStart, nameEnd := pointers(name)
	defer free(nameStart, nameEnd)
	var groupNums *C.int
	n := C.onig_name_to_group_numbers(r.regex, nameStart, nameEnd, &groupNums)
	if n <= 0 {
		return nil, fmt.Errorf("%v: no such capture group in pattern", name)
	}
	result := make([]C.int, 0, int(n))
	for i := 0; i < int(n); i++ {
		result = append(result, getPos(groupNums, C.int(i)))
	}
	r.cachedCaptureGroupNums[name] = result
	return result, nil
}

func (regex *Regex) Search(input string) (*SearchResult, error) {
	return regex.searchWithOffset(input, 0)
}

func (regex *Regex) searchWithOffset(input string, offset int) (*SearchResult, error) {
	region := C.onig_region_new()
	inputStart, inputEnd := pointers(input)
	defer free(inputStart, inputEnd)
	searchStart := offsetPointer(inputStart, offset)
	r := C.onig_search(regex.regex, inputStart, inputEnd, searchStart, inputEnd, region, C.ONIG_OPTION_NONE)
	if r == C.ONIG_MISMATCH {
		C.onig_region_free(region, 1)
		return &SearchResult{
			match: false,
		}, nil
	} else if r < 0 {
		C.onig_region_free(region, 1)
		return nil, errors.New(errMsg(r))
	} else {
		return &SearchResult{
			match:  true,
			regex:  regex,
			region: region,
			input:  input,
		}, nil
	}
}

func (m *SearchResult) IsMatch() bool {
	return m.match
}

func (m *SearchResult) Free() {
	if m.match {
		C.onig_region_free(m.region, 1)
	}
}

func (m *SearchResult) GetCaptureGroupByName(name string) (string, error) {
	if !m.match {
		return "", nil // no match -> no capture group
	}
	groupNums, err := m.regex.getCaptureGroupNums(name)
	if err != nil {
		return "", err
	}
	for _, groupNum := range groupNums {
		result, err := m.getCaptureGroupByNumber(groupNum)
		if err != nil {
			return "", err
		}
		if len(result) > 0 {
			return result, nil
		}
	}
	return "", nil
}

func (m *SearchResult) GetCaptureGroupByNumber(groupNum int) (string, error) {
	return m.getCaptureGroupByNumber(C.int(groupNum))
}

func (m *SearchResult) getCaptureGroupByNumber(groupNum C.int) (string, error) {
	if !m.match {
		return "", nil // no match -> no capture group
	}
	beg := getPos(m.region.beg, groupNum)
	end := getPos(m.region.end, groupNum)
	if beg == -1 && end == -1 {
		// optional capture, like (x)?, and no match
		return "", nil
	} else if beg > end || beg < 0 || int(end) > len(m.input) {
		return "", fmt.Errorf("unexpected result when calling oniguruma.getPos()")
	} else if beg == end {
		return "", nil
	} else {
		return m.input[beg:end], nil
	}
}

func (m *SearchResult) startPos() int {
	beg := getPos(m.region.beg, 0)
	return int(beg)
}

func (m *SearchResult) endPos() int {
	end := getPos(m.region.end, 0)
	return int(end)
}

// returns a pointer to the start of the string and a pointer to the end of the string
func pointers(s string) (start, end *C.OnigUChar) {
	start = (*C.OnigUChar)(unsafe.Pointer(C.CString(s)))
	end = (*C.OnigUChar)(unsafe.Pointer(uintptr(unsafe.Pointer(start)) + uintptr(len(s))))
	return
}

func offsetPointer(start *C.OnigUChar, offset int) *C.OnigUChar {
	return (*C.OnigUChar)(unsafe.Pointer(uintptr(unsafe.Pointer(start)) + uintptr(offset)))
}

// returns p[i]
func getPos(p *C.int, i C.int) C.int {
	return *(*C.int)(unsafe.Pointer(uintptr(unsafe.Pointer(p)) + uintptr(i)*unsafe.Sizeof(C.int(0))))
}

// free the pointers returned by pointers()
func free(start *C.OnigUChar, end *C.OnigUChar) {
	// The memset call is for debugging: We zero the content, such that the program crashes if the content was used after calling free()
	C.memset(unsafe.Pointer(start), C.int(0), C.size_t(uintptr(unsafe.Pointer(end))-uintptr(unsafe.Pointer(start))))
	C.free(unsafe.Pointer(start))
}

func errMsgWithInfo(returnCode C.int, errorInfo *C.OnigErrorInfo) string {
	msg := make([]byte, C.ONIG_MAX_ERROR_MESSAGE_LEN)
	l := C.oniguruma_helper_error_code_with_info_to_str((*C.UChar)(&msg[0]), returnCode, errorInfo)
	if l <= 0 {
		return "unknown error"
	} else {
		return string(msg[:l])
	}
}

func errMsg(returnCode C.int) string {
	msg := make([]byte, C.ONIG_MAX_ERROR_MESSAGE_LEN)
	l := C.oniguruma_helper_error_code_to_str((*C.UChar)(&msg[0]), returnCode)
	if l <= 0 {
		return "unknown error"
	} else {
		return string(msg[:l])
	}
}
