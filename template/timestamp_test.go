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
	"strconv"
	"testing"
)

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
