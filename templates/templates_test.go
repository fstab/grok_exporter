// Copyright 2016-2017 The grok_exporter Authors
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

package templates

import (
	"fmt"
	"strconv"
	"testing"
)

func TestReferencedGrokFields(t *testing.T) {

	// Some template actions require arrays as parameters.
	// Provide a function returning an array so that we can test these actions.
	funcs["testarray"] = func(strings ...string) []string {
		return strings
	}

	for i, test := range []struct {
		template           string
		expectedGrokFields []string
		example            map[string]string
		expectedResult     string
	}{
		// The template examples below are from Go's text/template documentation
		// https://golang.org/pkg/text/template/
		{
			// {{/* a comment */}}
			template:           "{{/* a comment {.field} should be ignored */}}",
			expectedGrokFields: []string{},
			example:            map[string]string{},
			expectedResult:     "",
		},
		{
			// {{pipeline}} with fixed string
			template:           "42",
			expectedGrokFields: []string{},
			example:            map[string]string{},
			expectedResult:     "42",
		},
		{
			// {{pipeline}} with simple values
			template:           "{{.count_total}} items are made of {{.material}}",
			expectedGrokFields: []string{"count_total", "material"},
			example: map[string]string{
				"count_total": "3",
				"material":    "metal",
			},
			expectedResult: "3 items are made of metal",
		},
		{
			// {{pipeline}} with function call
			template:           "{{.count_total}} items are made {{printf \"of %v\" .material}}",
			expectedGrokFields: []string{"count_total", "material"},
			example: map[string]string{
				"count_total": "3",
				"material":    "metal",
			},
			expectedResult: "3 items are made of metal",
		},
		{
			// {{if pipeline}} T1 {{end}}
			template:           "{{if eq .field1 .field2}}{{.field3}}{{end}}",
			expectedGrokFields: []string{"field1", "field2", "field3"},
			example: map[string]string{
				"field1": "a",
				"field2": "b",
				"field3": "c",
			},
			expectedResult: "",
		},
		{
			// {{if pipeline}} T1 {{else}} T0 {{end}}
			template:           "{{if eq .field1 .field2}}{{.field3}}{{else}}{{.field4}}{{end}}",
			expectedGrokFields: []string{"field1", "field2", "field3", "field4"},
			example: map[string]string{
				"field1": "a",
				"field2": "b",
				"field3": "c",
				"field4": "d",
			},
			expectedResult: "d",
		},
		{
			// {{if pipeline}} T1 {{else if pipeline}} T0 {{end}}
			template:           "{{if eq .field1 .field2}}{{.field3}}{{else if eq .field4 .field5}}{{.field6}}{{end}}",
			expectedGrokFields: []string{"field1", "field2", "field3", "field4", "field5", "field6"},
			example: map[string]string{
				"field1": "a",
				"field2": "b",
				"field3": "c",
				"field4": "d",
				"field5": "e",
				"field6": "f",
			},
			expectedResult: "",
		},
		{
			// {{range pipeline}} T1 {{end}}
			template:           "{{range testarray .field1 \" vs \" .field2}}{{printf \"%v\" .}}{{end}}",
			expectedGrokFields: []string{"field1", "field2"},
			example: map[string]string{
				"field1": "23",
				"field2": "42",
			},
			expectedResult: "23 vs 42",
		},
		{
			// {{range pipeline}} T1 {{else}} T0 {{end}}
			template:           "{{range testarray \"42\"}}{{.}}{{else}}{{.field}}{{end}}",
			expectedGrokFields: []string{"field"},
			example: map[string]string{
				"field": "128",
			},
			expectedResult: "42",
		},
		{
			// {{template "name"}}
			template:           "{{define \"T1\"}}some constant{{end}}{{template \"T1\"}}",
			expectedGrokFields: []string{},
			example:            map[string]string{},
			expectedResult:     "some constant",
		},
		{
			// {{template "name" pipeline}}
			template:           "{{define \"T1\"}}{{print .field1 \".\" .field2}}{{end}}{{template \"T1\" .}}",
			expectedGrokFields: []string{"field1", "field2"},
			example: map[string]string{
				"field1": "3",
				"field2": "4",
			},
			expectedResult: "3.4",
		},
		{
			// {{block "name" pipeline}} T1 {{end}}
			template:           "{{block \"T1\" .}}{{print .field1}}{{end}}",
			expectedGrokFields: []string{"field1"},
			example: map[string]string{
				"field1": "17",
				"field2": "18",
			},
			expectedResult: "17",
		},
		{
			// {{with pipeline}} T1 {{end}}
			template:           "{{with .field}}{{.}}{{end}}",
			expectedGrokFields: []string{"field"},
			example: map[string]string{
				"field": "23",
			},
			expectedResult: "23",
		},
		{
			// {{with pipeline}} T1 {{else}} T0 {{end}}
			template:           "{{with .field1}}{{.}}{{else}}{{.field2}}{{end}}",
			expectedGrokFields: []string{"field1", "field2"},
			example: map[string]string{
				"field1": "23",
				"field2": "42",
			},
			expectedResult: "23",
		},
		// ---
		// examples from Issue #10
		// ---
		{
			template:           "{{if eq .field1 .field2}}{{.field3}}{{else if eq .field4 .field5}}{{.field6}}{{else}}{{.field7}}{{end}}",
			expectedGrokFields: []string{"field1", "field2", "field3", "field4", "field5", "field6", "field7"},
			example: map[string]string{
				"field1": "1",
				"field2": "2",
				"field3": "3",
				"field4": "99",
				"field5": "99",
				"field6": "6",
				"field7": "7",
			},
			expectedResult: "6",
		},
		{
			template:           "{{if eq .val2 \"test\"}}yes{{else}}no{{end}}",
			expectedGrokFields: []string{"val2"},
			example: map[string]string{
				"val2": "test",
			},
			expectedResult: "yes",
		},
		{
			template:           "{{with $x := .field}}This is $x: {{$x}}{{end}}",
			expectedGrokFields: []string{"field"},
			example: map[string]string{
				"field": "128",
			},
			expectedResult: "This is $x: 128",
		},
		{
			template:           "{{define \"T1\"}}{{.}}{{end}}{{template \"T1\" .field}}",
			expectedGrokFields: []string{"field"},
			example: map[string]string{
				"field": "77",
			},
			expectedResult: "77",
		},
	} {
		parsedTemplate, err := New(fmt.Sprintf("test_1_%v", i), test.template)
		if err != nil {
			t.Fatalf("unexpected error while parsing template %v: %v", i, err)
			return
		}
		assertArrayEqualsIgnoreOrder(t, test.expectedGrokFields, parsedTemplate.ReferencedGrokFields())
		result, err := parsedTemplate.Execute(test.example)
		if err != nil {
			t.Fatalf("unexpected error while executing template %v: %v", i, err)
			return
		}
		if result != test.expectedResult {
			t.Fatalf("Expected \"%v\", but got \"%v\".", test.expectedResult, result)
			return
		}
	}
}

func TestTimestampFunction(t *testing.T) {
	// log4j examples: http://log4jtester.com
	template, err := New("date", "{{timestamp \"2006-01-02 15:04:05,000\" .date}}")
	if err != nil {
		t.Fatalf("unexpected error parsing template: %v", err)
		return
	}
	result1 := evalTimestamp(t, template, "2015-07-26 15:01:33,665")
	result2 := evalTimestamp(t, template, "2015-07-26 15:02:33,665")
	durationSeconds := int(result2 - result1)
	if durationSeconds != 60 {
		t.Fatalf("expected 60 seconds difference between the two timestamps, but got %v seconds", durationSeconds)
	}
}

func TestTimestampCommaError(t *testing.T) {
	// 0 commas
	_, err := New("date", "{{timestamp \"2006-01-02 15:04:05.000\" .date}}")
	if err != nil {
		t.Fatalf("unexpected error parsing template: %v", err)
	}
	// 1 comma ok
	_, err = New("date", "{{timestamp \"2006-01-02 15:04:05,000\" .date}}")
	if err != nil {
		t.Fatalf("unexpected error parsing template: %v", err)
	}
	// 1 comma wrong
	_, err = New("date", "{{timestamp \"2006-01-02, 15:04:05\" .date}}")
	if err == nil {
		t.Fatal("expected error, but got no error.")
	}
	// more than 1 comma
	_, err = New("date", "{{timestamp \"2006-01-02 15:04:05,999,000\" .date}}")
	if err == nil {
		t.Fatal("expected error, but got no error.")
	}
}

func evalTimestamp(t *testing.T, template Template, value string) float64 {
	resultString, err := template.Execute(map[string]string{
		"date": value,
	})
	if err != nil {
		t.Fatalf("unexpected error parsing date: %v", err)
	}
	result, err := strconv.ParseFloat(resultString, 64)
	if err != nil {
		t.Fatalf("template returned invalid float64: %v", err)
	}
	return result
}

func assertArrayEqualsIgnoreOrder(t *testing.T, expected, actual []string) {
	if len(expected) != len(actual) {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
	for _, act := range actual {
		found := false
		for _, exp := range expected {
			if act == exp {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected: %v, actual: %v", expected, actual)
		}
	}
}
