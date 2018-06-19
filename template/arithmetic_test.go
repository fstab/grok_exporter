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

package template

import (
	"testing"
)

func TestArithmeticFunctions(t *testing.T) {
	for _, data := range []struct {
		template       string
		val            string
		expectedResult string
		parseError     bool
		execError      bool
	}{
		{"{{add 1 .val}}", "2", "3", false, false},
		{"{{subtract .val 3}}", "2", "-1", false, false},
		{"{{multiply 2.5 .val}}", "1.e2", "250", false, false},
		{"{{divide \"2.5\" .val}}", "2.5", "1", false, false},
		{"{{divide 3.0 .val}}", "0", "", false, true},
		{"{{multiply 3i .val}}", "2", "", true, false},
		{"{{multiply 0 true}}", "2", "", false, true},
	} {
		template, err := New("test", data.template)
		if data.parseError {
			if err == nil {
				t.Fatalf("expected error parsing template %v, but got no error", data.template)
			}
			continue
		}
		if err != nil {
			t.Fatalf("unexpected error parsing template %v: %v", data.template, err)
		}
		result, err := template.Execute(map[string]string{
			"val": data.val,
		})
		if data.execError {
			if err == nil {
				t.Fatalf("expected error executing template %v, but got no error", data.template)
			}
			continue
		}
		if err != nil {
			t.Fatalf("error executing template %v: %v", data.template, err)
		}
		if result != data.expectedResult {
			t.Fatalf("unexpected result executing template %v: expected %v but got %v", data.template, data.expectedResult, result)
		}
	}
}
