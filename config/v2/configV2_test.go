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

package v2

import (
	"strings"
	"testing"
	"time"
)

const counter_config = `
global:
    config_version: 2
input:
    type: file
    path: x/x/x
    fail_on_missing_logfile: false
    readall: true
grok:
    patterns_dir: b/c
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
    config_version: 2
input:
    type: file
    path: x/x/x
grok:
    patterns_dir: b/c
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
    config_version: 2
input:
    type: stdin
grok:
    patterns_dir: b/c
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
    config_version: 2
input:
    type: stdin
grok:
    patterns_dir: b/c
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
    config_version: 2
input:
    type: stdin
grok:
    patterns_dir: xxx
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
    config_version: 2
input:
    type: stdin
grok:
    patterns_dir: b/c
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
	if cfg.Metrics[0].Cumulative != true {
		t.Fatal("Expected 'true' as gauge cumulative option.")
	}
}

func TestGaugeDefaultCumulativeConfig(t *testing.T) {
	cfgString := strings.Replace(gauge_config, "      cumulative: true\n", "", 1)
	cfg := loadOrFail(t, cfgString)
	if cfg.Metrics[0].Cumulative != false {
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
	metric := cfg.Metrics[0]
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
	metric := cfg.Metrics[0]
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
	if len(cfg.Metrics) != 1 {
		t.Fatalf("Expected 1 metric, but found %v.", len(cfg.Metrics))
	}
	metric := cfg.Metrics[0]
	if len(metric.LabelTemplates) != 2 {
		t.Fatalf("Expected 2 label templates, but found %v.", len(metric.LabelTemplates))
	}
	if len(metric.DeleteLabelTemplates) != 1 {
		t.Fatalf("Expected 1 delete label template, but found %v.", len(metric.DeleteLabelTemplates))
	}
}

func TestRetentionValidConfig(t *testing.T) {
	cfg := loadOrFail(t, retention_config)
	if cfg.Metrics[0].Retention != 2*time.Hour+45*time.Minute {
		t.Fatalf("Error parsing retention, got %v", (cfg.Metrics)[0].Retention)
	}
}

func TestRetentionInvalidConfig(t *testing.T) {
	invalidCfg := strings.Replace(retention_config, "2h45m0s", "abc", 1)
	_, err := Unmarshal([]byte(invalidCfg))
	if err == nil || !strings.Contains(err.Error(), "abc") {
		t.Fatal("Expected error saying that 'abc' is not a valid duration.")
	}
}

func loadOrFail(t *testing.T, cfgString string) *Config {
	cfg, err := Unmarshal([]byte(cfgString))
	if err != nil {
		t.Fatalf("Failed to read config: %v", err.Error())
	}
	if !equalsIgnoreIndentation(cfg.String(), cfgString) {
		t.Fatalf("Expected:\n%v\nActual:\n%v\n", cfgString, cfg)
	}
	return cfg
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
