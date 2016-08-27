package exporter

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
	"unsafe"
)

var (
	// See the #define statements in oniguruma.h
	ONIG_ENCODING_UTF8 = &C.OnigEncodingUTF8
)

type OnigurumaLib struct {
	encoding C.OnigEncoding
}

type OnigurumaRegexp struct {
	regex                  C.OnigRegex
	cachedCaptureGroupNums map[string][]C.int
}

type OnigurumaMatchResult struct {
	match  bool
	regex  *OnigurumaRegexp
	region *C.OnigRegion
	input  string
}

// Warning: The Oniguruma library is not thread save, it should be used in a single thread.
func InitOnigurumaLib() (*OnigurumaLib, error) {
	result := &OnigurumaLib{
		encoding: ONIG_ENCODING_UTF8, // TODO: This is the encoding of the logfile. Should be configurable and default to the system encoding.
	}
	encodings := []C.OnigEncoding{result.encoding}
	ret := C.oniguruma_helper_initialize(&encodings[0], C.int(len(encodings)))
	if ret != 0 {
		return nil, errors.New("failed to initialize encoding for the Oniguruma regular expression library.")
	}
	return result, nil
}

func (o *OnigurumaLib) Version() string {
	return C.GoString(C.onig_version())
}

func (o *OnigurumaLib) Compile(pattern string) (*OnigurumaRegexp, error) {
	result := &OnigurumaRegexp{
		cachedCaptureGroupNums: make(map[string][]C.int),
	}
	patternStart, patternEnd := pointers(pattern)
	defer free(patternStart, patternEnd)
	var errorInfo C.OnigErrorInfo
	r := C.onig_new(&result.regex, patternStart, patternEnd, C.ONIG_OPTION_DEFAULT, o.encoding, C.ONIG_SYNTAX_DEFAULT, &errorInfo)
	if r != C.ONIG_NORMAL {
		return nil, errors.New(errMsgWithInfo(r, &errorInfo))
	}
	return result, nil
}

func (regex *OnigurumaRegexp) Free() {
	C.onig_free(regex.regex)
}

func (regex *OnigurumaRegexp) HasCaptureGroup(name string) bool {
	_, err := regex.getCaptureGroupNums(name)
	return err == nil
}

func (r *OnigurumaRegexp) getCaptureGroupNums(name string) ([]C.int, error) {
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

func (regex *OnigurumaRegexp) Match(input string) (*OnigurumaMatchResult, error) {
	region := C.onig_region_new()
	inputStart, inputEnd := pointers(input)
	defer free(inputStart, inputEnd)
	r := C.onig_match(regex.regex, inputStart, inputEnd, inputStart, region, C.ONIG_OPTION_NONE)
	if r == C.ONIG_MISMATCH {
		C.onig_region_free(region, 1)
		return &OnigurumaMatchResult{
			match: false,
		}, nil
	} else if r < 0 {
		C.onig_region_free(region, 1)
		return nil, errors.New(errMsg(r))
	} else {
		return &OnigurumaMatchResult{
			match:  true,
			regex:  regex,
			region: region,
			input:  input,
		}, nil
	}
}

func (m *OnigurumaMatchResult) Get(name string) (string, error) {
	if !m.match {
		return "", nil // no match -> no capture group
	}
	groupNums, err := m.regex.getCaptureGroupNums(name)
	if err != nil {
		return "", err
	}
	for _, groupNum := range groupNums {
		beg := getPos(m.region.beg, groupNum)
		end := getPos(m.region.end, groupNum)
		if beg > end || beg < 0 || int(end) > len(m.input) {
			return "", fmt.Errorf("%v: unexpected result when calling onig_name_to_group_numbers()", name)
		} else if beg == end {
			continue // return empty string unless there are other matches for that name.
		} else {
			return m.input[beg:end], nil
		}
	}
	return "", nil
}

func (m *OnigurumaMatchResult) IsMatch() bool {
	return m.match
}

func (m *OnigurumaMatchResult) Free() {
	if m.match {
		C.onig_region_free(m.region, 1)
	}
}

// returns a pointer to the start of the string and a pointer to the end of the string
func pointers(s string) (start, end *C.OnigUChar) {
	start = (*C.OnigUChar)(unsafe.Pointer(C.CString(s)))
	end = (*C.OnigUChar)(unsafe.Pointer(uintptr(unsafe.Pointer(start)) + uintptr(len(s))))
	return
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
