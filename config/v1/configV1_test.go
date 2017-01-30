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

package v1

import (
	"strings"
	"testing"
)

const v1cfg = `
input:
    type: file
    path: x/x/x
    readall: true
grok:
    patterns_dir: b/c
metrics:
    - type: counter
      name: test_count_total
      help: Dummy help message for counter.
      match: Some text here, then a %{DATE}.
      labels:
          - grok_field_name: grok_field_a
            prometheus_label: prom_label_a
          - grok_field_name: grok_field_b
            prometheus_label: prom_label_b
    - type: gauge
      name: test_gauge
      help: Dummy help message for gauge.
      match: '%{DATE} %{TIME} %{USER:user} %{NUMBER:val}'
      value: val
      cumulative: true
      labels:
          - grok_field_name: user
            prometheus_label: user
    - type: histogram
      name: test_histogram
      help: Dummy help message for histogram.
      match: '%{DATE} %{TIME} %{USER:user} %{NUMBER:val}'
      value: val
      buckets: [1, 2, 3]
      labels:
          - grok_field_name: user
            prometheus_label: user
    - type: summary
      name: test_summary
      help: Dummy help message for summary.
      match: '%{DATE} %{TIME} %{USER:user} %{NUMBER:val}'
      value: val
      quantiles: {0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
      labels:
          - grok_field_name: user
            prometheus_label: user
server:
    protocol: https
    port: 1111
`

const expected = `
global:
    config_version: 2
input:
    type: file
    path: x/x/x
    readall: true
grok:
    patterns_dir: b/c
metrics:
    - type: counter
      name: test_count_total
      help: Dummy help message for counter.
      match: Some text here, then a %{DATE}.
      labels:
        prom_label_a: '{{.grok_field_a}}'
        prom_label_b: '{{.grok_field_b}}'
    - type: gauge
      name: test_gauge
      help: Dummy help message for gauge.
      match: '%{DATE} %{TIME} %{USER:user} %{NUMBER:val}'
      value: '{{.val}}'
      cumulative: true
      labels:
        user: '{{.user}}'
    - type: histogram
      name: test_histogram
      help: Dummy help message for histogram.
      match: '%{DATE} %{TIME} %{USER:user} %{NUMBER:val}'
      value: '{{.val}}'
      buckets: [1, 2, 3]
      labels:
        user: '{{.user}}'
    - type: summary
      name: test_summary
      help: Dummy help message for summary.
      match: '%{DATE} %{TIME} %{USER:user} %{NUMBER:val}'
      value: '{{.val}}'
      quantiles: {0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
      labels:
        user: '{{.user}}'
server:
    protocol: https
    port: 1111
`

func TestImportV1(t *testing.T) {
	cfg, err := Unmarshal([]byte(v1cfg))
	if err != nil {
		t.Fatalf("Failed to read config: %v", err.Error())
	}
	if !equalsIgnoreIndentation(cfg.String(), expected) {
		t.Fatalf("Expected:\n%v\nActual:\n%v\n", expected, cfg)
	}
}

func equalsIgnoreIndentation(a string, b string) bool {
	aLines := stripEmptyLines(strings.Split(a, "\n"))
	bLines := stripEmptyLines(strings.Split(b, "\n"))
	if len(aLines) != len(bLines) {
		return false
	}
	for i := range aLines {
		if strings.TrimSpace(aLines[i]) != strings.TrimSpace(bLines[i]) {
			return false
		}
	}
	return true
}

func stripEmptyLines(lines []string) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}
