// Copyright 2020 The grok_exporter Authors
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

package v3

import (
	"github.com/fstab/grok_exporter/config/v2"
	"gopkg.in/yaml.v2"
	"testing"
)

const empty_v2 = ``

const empty_v3 = `
global:
    config_version: 3
input:
    line_delimiter:
`

const full_v2 = `
global:
    config_version: 2
    retention_check_interval: 3s
input:
    type: file
    line_delimiter: \n
    paths:
      - /path/to/file1.log
      - /dir/with/*.log
    fail_on_missing_logfile: false
    readall: true
    poll_interval_seconds: 13
    max_lines_in_buffer: 1024
    webhook_path: /webhook
    webhook_format: json_bulk
    webhook_json_selector: .message
    webhook_text_bulk_separator: \n\n
grok:
    patterns_dir: /path/to/patterns
    additional_patterns:
      - 'EXIM_MESSAGE [a-zA-Z ]*'
      - 'SIMPLE_DATE [0-9]{4}-[0-9]{2}-[0-9]{2}'
metrics:
    - type: counter
      name: test_count_total
      help: Dummy help message.
      match: Some text here, then a %{DATE}.
      labels:
          label_a: '{{.some_grok_field_a}}'
          label_b: '{{.some_grok_field_b}}'
    - type: summary
      name: invalid_test_metric
      help: This is actually not a valid metric definition
      paths:
        - /var/log/*.log
        - /var/log/*.txt
      match: ERROR %{DATE}
      retention: 3m30s
      value: '{{ .val }}'
      cumulative: true
      buckets: [2, 4, 6, 8, 16, 32, 64]
      quantiles: {0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
      labels:
          a: '{{.a}}'
          b: '{{.b}}'
      delete_match: ERROR %{DATE}
      delete_labels:
          a: '{{.a}}'
server:
    protocol: https
    port: 1111
    path: /secret_metrics
    cert: /path/to/cert
    key: /path/to/key
`

const full_v3 = `
global:
    config_version: 3
    retention_check_interval: 3s
input:
    type: file
    line_delimiter: \n
    paths:
      - /path/to/file1.log
      - /dir/with/*.log
    fail_on_missing_logfile: false
    readall: true
    poll_interval: 13s
    max_lines_in_buffer: 1024
    webhook_path: /webhook
    webhook_format: json_bulk
    webhook_json_selector: .message
    webhook_text_bulk_separator: \n\n
imports:
    - type: grok_patterns
      dir: /path/to/patterns
grok_patterns:
    - EXIM_MESSAGE [a-zA-Z ]*
    - SIMPLE_DATE [0-9]{4}-[0-9]{2}-[0-9]{2}
metrics:
    - type: counter
      name: test_count_total
      help: Dummy help message.
      match: Some text here, then a %{DATE}.
      labels:
          label_a: '{{.some_grok_field_a}}'
          label_b: '{{.some_grok_field_b}}'
    - type: summary
      name: invalid_test_metric
      help: This is actually not a valid metric definition
      paths:
        - /var/log/*.log
        - /var/log/*.txt
      match: ERROR %{DATE}
      retention: 3m30s
      value: '{{ .val }}'
      cumulative: true
      buckets: [2, 4, 6, 8, 16, 32, 64]
      quantiles: {0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
      labels:
          a: '{{.a}}'
          b: '{{.b}}'
      delete_match: ERROR %{DATE}
      delete_labels:
          a: '{{.a}}'
server:
    protocol: https
    port: 1111
    path: /secret_metrics
    cert: /path/to/cert
    key: /path/to/key
`

func TestConvert(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", empty_v2, empty_v3},
		{"full", full_v2, full_v3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			v2cfg := &v2.Config{}
			err := yaml.Unmarshal([]byte(test.input), v2cfg)
			if err != nil {
				t.Fatalf("error unmarshalling input: %v", err)
			}
			v3cfg := convert(v2cfg)
			err = equalsIgnoreIndentation(v3cfg.String(), test.expected)
			if err != nil {
				t.Fatalf("Expected:\n%v\nActual:\n%v\n%v", test.expected, v3cfg, err)
			}
		})
	}
}
