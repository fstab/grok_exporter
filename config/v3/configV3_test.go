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
	"fmt"
	"strings"
	"testing"
	"time"
)

const counter_config = `
global:
    config_version: 3
input:
    type: file
    path: x/x/x
    fail_on_missing_logfile: false
    readall: true
metrics:
    - type: counter
      name: test_count_total
      help: Dummy help message.
      match: Some text here, then a %{DATE}.
      labels:
          label_a: '{{.some_grok_field_a}}'
          label_b: '{{.some_grok_field_b}}'
server:
    protocol: https
    port: 1111
`

const gauge_config = `
global:
    config_version: 3
input:
    type: file
    path: x/x/x
metrics:
    - type: gauge
      name: test_histogram
      help: Dummy help message.
      match: Some %{NUMBER:val} here, then a %{DATE}.
      value: '{{.val}}'
      cumulative: true
server:
    protocol: http
    host: localhost
    port: 9144
`

const histogram_config = `
global:
    config_version: 3
input:
    type: stdin
metrics:
    - type: histogram
      name: test_histogram
      help: Dummy help message.
      match: Some %{NUMBER:val} here, then a %{DATE}.
      value: '{{.val}}'
      buckets: $BUCKETS
server:
    protocol: http
    port: 9144
`

const summary_config = `
global:
    config_version: 3
input:
    type: stdin
metrics:
    - type: summary
      name: test_summary
      help: Dummy help message.
      match: Some %{NUMBER:val} here, then a %{DATE}.
      value: '{{.val}}'
      quantiles: $QUANTILES
server:
    protocol: http
    port: 9144
`

const delete_labels_config = `
global:
    config_version: 3
input:
    type: stdin
metrics:
    - type: counter
      name: test_count_total
      help: Dummy help message.
      match: Some text here, then a %{DATE}.
      labels:
          label_a: '{{.some_grok_field_a}}'
          label_b: '{{.some_grok_field_b}}'
      delete_match: Some shutdown message
      delete_labels:
          label_a: '{{.some_grok_field_a}}'
server:
    protocol: http
    port: 9144
`

const retention_config = `
global:
    config_version: 3
input:
    type: stdin
metrics:
    - type: counter
      name: test_count_total
      help: Dummy help message.
      match: Some text here, then a %{DATE:date}.
      retention: 2h45m0s
      labels:
          date: '{{.date}}'
server:
    protocol: http
    port: 9144
`

const multiple_paths_config = `
global:
    config_version: 3
input:
    type: file
    paths:
    - /tmp/dir1/*.log
    - /tmp/dir2/*.log
    fail_on_missing_logfile: false
    readall: true
metrics:
    - type: counter
      name: test_count_total
      help: Dummy help message.
      paths:
      - /tmp/dir1/*.log
      - /tmp/dir2/*.log
      match: Some text here, then a %{DATE}.
      labels:
          label_a: '{{.some_grok_field_a}}'
          label_b: '{{.some_grok_field_b}}'
server:
    protocol: https
    port: 1111
`

const empty_grok_section = `
global:
    config_version: 3
input:
    type: file
    path: /tmp/test/*.log
metrics:
    - type: counter
      name: errors_total
      help: Dummy help message.
      match: ERROR
server:
    protocol: http
    port: 9144
`

const config_with_imports = `
global:
    config_version: 3
input:
    type: stdin
imports:
    - type: metrics
      file: /etc/grok/metrics.d/*.yaml
      defaults:
          path: /var/log/syslog/*
          labels:
              logfile: '{{base .logfile}}'
grok_patterns:
    - WARN WARN
    - ERROR ERROR
metrics:
    - type: counter
      name: errors_total
      help: Dummy help message.
      match: ERROR
server:
    protocol: http
    port: 9144
`

const import_1 = `
    - type: counter
      name: errors_total_1
      help: Dummy help message.
      match: ERROR
`

const import_2 = `
    - type: counter
      name: errors_total_2
      help: Dummy help message.
      match: ERROR
`

type mockLoader struct {
	files []*ConfigFile
}

func (f *mockLoader) LoadDir(dir string) ([]*ConfigFile, error) {
	return f.files, nil
}

func (f *mockLoader) LoadGlob(globString string) ([]*ConfigFile, error) {
	return f.files, nil
}

func TestCounterValidConfig(t *testing.T) {
	loadOrFail(t, counter_config)
}

func TestGaugeValidConfig(t *testing.T) {
	loadOrFail(t, gauge_config)
}

func TestGaugeInvalidConfig(t *testing.T) {
	invalidCfg := strings.Replace(gauge_config, "      value: '{{.val}}'\n", "", 1)
	_, err := Unmarshal([]byte(invalidCfg))
	if err == nil || !strings.Contains(err.Error(), "'metrics.value' must not be empty") {
		t.Fatal("Expected error message saying that value is missing.")
	}
}

func TestGaugeCumulativeConfig(t *testing.T) {
	cfg := loadOrFail(t, gauge_config)
	if cfg.AllMetrics[0].Cumulative != true {
		t.Fatal("Expected 'true' as gauge cumulative option.")
	}
}

func TestGaugeDefaultCumulativeConfig(t *testing.T) {
	cfgString := strings.Replace(gauge_config, "      cumulative: true\n", "", 1)
	cfg := loadOrFail(t, cfgString)
	if cfg.AllMetrics[0].Cumulative != false {
		t.Fatal("Expected 'false' as default for gauge cumulative option.")
	}
}

func TestGaugeInvalidCumulativeConfig(t *testing.T) {
	invalidCfg := strings.Replace(gauge_config, "      cumulative: true\n", "      cumulative: dontknow\n", 1)
	_, err := Unmarshal([]byte(invalidCfg))
	if err == nil || !strings.Contains(err.Error(), "dontknow") {
		t.Fatal("Expected error message saying that 'dontknow' is invalid.", err)
	}
}

func TestHistogramValidConfig(t *testing.T) {
	validCfg := strings.Replace(histogram_config, "$BUCKETS", "[0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]", 1)
	cfg := loadOrFail(t, validCfg)
	metric := cfg.AllMetrics[0]
	if len(metric.Buckets) != 11 || metric.Buckets[0] != 0.005 || metric.Buckets[10] != 10 {
		t.Fatalf("Error parsing bucket list: Got %v", metric.Buckets)
	}
}

func TestHistogramInvalidConfig(t *testing.T) {
	invalidCfg := strings.Replace(histogram_config, "$BUCKETS", "[0.005, oops, 10]", 1)
	_, err := Unmarshal([]byte(invalidCfg))
	if err == nil || !strings.Contains(err.Error(), "oops") {
		t.Fatal("Expected error saying that 'oops' is not a valid number.")
	}
}

func TestSummaryValidConfig(t *testing.T) {
	validCfg := strings.Replace(summary_config, "$QUANTILES", "{0.5: 0.05, 0.9: 0.01, 0.99: 0.001}", 1)
	cfg := loadOrFail(t, validCfg)
	metric := cfg.AllMetrics[0]
	if len(metric.Quantiles) != 3 || metric.Quantiles[0.5] != 0.05 || metric.Quantiles[0.99] != 0.001 {
		t.Fatalf("Error parsing bucket list: Got %v", metric.Buckets)
	}
}

func TestSummaryInvalidConfig(t *testing.T) {
	invalidCfg := strings.Replace(summary_config, "$QUANTILES", "[0.005, 0.2, 10]", 1)
	_, err := Unmarshal([]byte(invalidCfg))
	if err == nil {
		t.Fatal("Expected error, because quantiles are a list and not a map.")
	}
}

func TestValueInvalidTemplate(t *testing.T) {
	invalidCfg := strings.Replace(gauge_config, "value: '{{.val}}'", "value: '{{val}}'", 1)
	_, err := Unmarshal([]byte(invalidCfg))
	if err == nil {
		t.Fatal("Expected error, because using {{val}} instead of {{.val}}.")
	}
}

func TestDeleteLabelConfig(t *testing.T) {
	cfg := loadOrFail(t, delete_labels_config)
	if len(cfg.AllMetrics) != 1 {
		t.Fatalf("Expected 1 metric, but found %v.", len(cfg.AllMetrics))
	}
	metric := cfg.AllMetrics[0]
	if len(metric.LabelTemplates) != 2 {
		t.Fatalf("Expected 2 label templates, but found %v.", len(metric.LabelTemplates))
	}
	if len(metric.DeleteLabelTemplates) != 1 {
		t.Fatalf("Expected 1 delete label template, but found %v.", len(metric.DeleteLabelTemplates))
	}
}

func TestRetentionValidConfig(t *testing.T) {
	cfg := loadOrFail(t, retention_config)
	if cfg.AllMetrics[0].Retention != 2*time.Hour+45*time.Minute {
		t.Fatalf("Error parsing retention, got %v", (cfg.AllMetrics)[0].Retention)
	}
}

func TestRetentionInvalidConfig(t *testing.T) {
	invalidCfg := strings.Replace(retention_config, "2h45m0s", "abc", 1)
	_, err := Unmarshal([]byte(invalidCfg))
	if err == nil || !strings.Contains(err.Error(), "abc") {
		t.Fatal("Expected error saying that 'abc' is not a valid duration.")
	}
}

func TestPathsValidConfig(t *testing.T) {
	loadOrFail(t, multiple_paths_config)
}

func TestDuplicateInputPaths(t *testing.T) {
	var s = `type: file
    path: /some/path/file.log`
	invalidCfg := strings.Replace(multiple_paths_config, "type: file", s, 1)
	_, err := Unmarshal([]byte(invalidCfg))
	if err == nil {
		t.Fatal("Expected error, but unmarshalling was successful.")
	}
	// Make sure it's the right error and not an error accidentally caused by incorrect indentation of the injected 'path' field.
	if !strings.Contains(err.Error(), "use either 'path' or 'paths' but not both") {
		t.Fatalf("Expected error message about path and paths being mutually exclusive, but got %v", err)
	}
}

func TestDuplicateMetricPaths(t *testing.T) {
	var s = `help: Dummy help message.
      path: /some/path/file.log`
	invalidCfg := strings.Replace(multiple_paths_config, "help: Dummy help message.", s, 1)
	_, err := Unmarshal([]byte(invalidCfg))
	if err == nil {
		t.Fatal("Expected error, but unmarshalling was successful.")
	}
	// Make sure it's the right error and not an error accidentally caused by incorrect indentation of the injected 'path' field.
	if !strings.Contains(err.Error(), "use either 'path' or 'paths' but not both") {
		t.Fatalf("Expected error message about path and paths being mutually exclusive, but got %v", err)
	}
}

func TestGlobsAreGenerated(t *testing.T) {
	cfg, err := Unmarshal([]byte(multiple_paths_config))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Input.Globs) != 2 {
		t.Fatalf("expected 2 Globs in input config, but found %v", len(cfg.Input.Globs))
	}
	if len(cfg.AllMetrics[0].Globs) != 2 {
		t.Fatalf("expected 2 Globs in metric config, but found %v", len(cfg.AllMetrics[0].Globs))
	}
}

func TestEmptyGrokSection(t *testing.T) {
	loadOrFail(t, empty_grok_section)
}

func TestConfigWithImports(t *testing.T) {
	fileLoader := &mockLoader{
		files: []*ConfigFile{
			{Path: "file1.yaml", Contents: import_1},
			{Path: "file2.yaml", Contents: import_2},
		},
	}
	cfg, err := unmarshal([]byte(config_with_imports), fileLoader)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err.Error())
	}
	err = equalsIgnoreIndentation(cfg.String(), config_with_imports)
	if err != nil {
		t.Fatalf("Expected:\n%v\nActual:\n%v\n%v", config_with_imports, cfg, err)
	}
	if len(cfg.AllMetrics) != 3 {
		t.Fatalf("expected 3 metrics, but found %v", len(cfg.AllMetrics))
	}
}

func loadOrFail(t *testing.T, cfgString string) *Config {
	cfg, err := Unmarshal([]byte(cfgString))
	if err != nil {
		t.Fatalf("Failed to read config: %v", err.Error())
	}
	err = equalsIgnoreIndentation(cfg.String(), cfgString)
	if err != nil {
		t.Fatalf("Expected:\n%v\nActual:\n%v\n%v", cfgString, cfg, err)
	}
	return cfg
}

func equalsIgnoreIndentation(actual, expected string) error {
	actualLines := stripEmptyLines(strings.Split(actual, "\n"))
	expectedLines := stripEmptyLines(strings.Split(expected, "\n"))
	length := len(actualLines)
	if len(expectedLines) < length {
		length = len(expectedLines)
	}
	for i := 0; i < length; i++ {
		if strings.TrimSpace(actualLines[i]) != strings.TrimSpace(expectedLines[i]) {
			return fmt.Errorf("line %v: expected '%v' but got '%v'", i, expectedLines[i], actualLines[i])
		}
	}
	if len(actualLines) != len(expectedLines) {
		return fmt.Errorf("expected %v non-empty lines, but got %v non-empty lines", len(expectedLines), len(actualLines))
	}
	return nil
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
