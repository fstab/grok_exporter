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
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type matchTest struct {
	pattern, s string
	match      bool
	valid      bool
	err        error
}

// Examples taken from golang's path/filepath/match_test.go
var matchTests = []matchTest{
	{"abc", "abc", true, true, nil},
	{"*", "abc", true, true, nil},
	{"*c", "abc", true, true, nil},
	{"a*", "a", true, true, nil},
	{"a*", "abc", true, true, nil},
	{"a*", "ab/c", false, true, nil},
	{"a*/b", "abc/b", true, true, nil},
	{"a*/b", "a/c/b", false, true, nil},
	{"a*b*c*d*e*/f", "axbxcxdxe/f", true, true, nil},
	{"a*b*c*d*e*/f", "axbxcxdxexxx/f", true, true, nil},
	{"a*b*c*d*e*/f", "axbxcxdxe/xxx/f", false, true, nil},
	{"a*b*c*d*e*/f", "axbxcxdxexxx/fff", false, true, nil},
	{"a*b?c*x", "abxbbxdbxebxczzx", true, true, nil},
	{"a*b?c*x", "abxbbxdbxebxczzy", false, true, nil},
	{"ab[c]", "abc", true, true, nil},
	{"ab[b-d]", "abc", true, true, nil},
	{"ab[e-g]", "abc", false, true, nil},
	{"ab[^c]", "abc", false, true, nil},
	{"ab[^b-d]", "abc", false, true, nil},
	{"ab[^e-g]", "abc", true, true, nil},
	{"a\\*b", "a*b", true, true, nil},
	{"a\\*b", "ab", false, true, nil},
	{"a?b", "a☺b", true, true, nil},
	{"a[^a]b", "a☺b", true, true, nil},
	{"a???b", "a☺b", false, true, nil},
	{"a[^a][^a][^a]b", "a☺b", false, true, nil},
	{"[a-ζ]*", "α", true, true, nil},
	{"*[a-ζ]", "A", false, true, nil},
	{"a?b", "a/b", false, true, nil},
	{"a*b", "a/b", false, true, nil},
	{"[\\]a]", "]", true, true, nil},
	{"[\\-]", "-", true, true, nil},
	{"[x\\-]", "x", true, true, nil},
	{"[x\\-]", "-", true, true, nil},
	{"[x\\-]", "z", false, true, nil},
	{"[\\-x]", "x", true, true, nil},
	{"[\\-x]", "-", true, true, nil},
	{"[\\-x]", "a", false, true, nil},
	{"[]a]", "]", false, false, filepath.ErrBadPattern},
	{"[-]", "-", false, false, filepath.ErrBadPattern},
	{"[x-]", "x", false, false, filepath.ErrBadPattern},
	{"[x-]", "-", false, false, filepath.ErrBadPattern},
	{"[x-]", "z", false, false, filepath.ErrBadPattern},
	{"[-x]", "x", false, false, filepath.ErrBadPattern},
	{"[-x]", "-", false, false, filepath.ErrBadPattern},
	{"[-x]", "a", false, false, filepath.ErrBadPattern},
	{"\\", "a", false, false, filepath.ErrBadPattern},
	{"[a-b-c]", "a", false, false, filepath.ErrBadPattern},
	{"[", "a", false, false, filepath.ErrBadPattern},
	{"[^", "a", false, false, filepath.ErrBadPattern},
	{"[^bc", "a", false, false, filepath.ErrBadPattern},
	{"a[", "a", false, false, nil},
	{"a[", "ab", false, false, filepath.ErrBadPattern},
	{"*x", "xxx", true, true, nil},
}

func TestIsPatternValid(t *testing.T) {
	for _, testData := range matchTests {
		if runtime.GOOS == "windows" && strings.Contains(testData.pattern, "\\") {
			// no escape allowed on windows.
			// original golang sources also skip tests in that case, see path/filepath/match_test.go
			continue
		}
		valid := IsPatternValid(testData.pattern)
		if valid && !testData.valid || !valid && testData.valid {
			t.Errorf("IsPatternValid(%#q) returned %t", testData.pattern, testData.valid)
		}
	}
}
