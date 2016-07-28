package exporter

import (
	"strings"
	"testing"
)

const counter_config = `
input:
    type: file
    path: x/x/x
    readall: true
grok:
    patterns_dir: b/c
metrics:
    - type: counter
      name: test_count_total
      help: Dummy help message.
      match: Some text here, then a %{DATE}.
      labels:
          - grok_field_name: a
            prometheus_label: b
          - grok_field_name: c
            prometheus_label: d
server:
    protocol: https
    port: 1111
`

const gauge_config = `
input:
    type: stdin
grok:
    patterns_dir: b/c
metrics:
    - type: gauge
      name: test_histogram
      help: Dummy help message.
      match: Some text here, then a %{DATE}.
      value: val
server:
    protocol: http
    port: 9144
`

const histogram_config = `
input:
    type: stdin
grok:
    patterns_dir: b/c
metrics:
    - type: histogram
      name: test_histogram
      help: Dummy help message.
      match: Some text here, then a %{DATE}.
      value: val
      buckets: $BUCKETS
server:
    protocol: http
    port: 9144
`

const summary_config = `
input:
    type: stdin
grok:
    patterns_dir: b/c
metrics:
    - type: summary
      name: test_summary
      help: Dummy help message.
      match: Some text here, then a %{DATE}.
      value: val
      quantiles: $QUANTILES
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
	invalidCfg := strings.Replace(gauge_config, "      value: val\n", "", 1)
	_, err := LoadConfigString([]byte(invalidCfg))
	if err == nil || !strings.Contains(err.Error(), "'metrics.value' must not be empty") {
		t.Fatal("Expected error message saying that value is missing.")
	}
}

func TestHistogramValidConfig(t *testing.T) {
	validCfg := strings.Replace(histogram_config, "$BUCKETS", "[0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10]", 1)
	cfg := loadOrFail(t, validCfg)
	metric := (*(cfg.Metrics))[0]
	if len(metric.Buckets) != 11 || metric.Buckets[0] != 0.005 || metric.Buckets[10] != 10 {
		t.Fatalf("Error parsing bucket list: Got %v", metric.Buckets)
	}
}

func TestHistogramInvalidConfig(t *testing.T) {
	invalidCfg := strings.Replace(histogram_config, "$BUCKETS", "[0.005, oops, 10]", 1)
	_, err := LoadConfigString([]byte(invalidCfg))
	if err == nil || !strings.Contains(err.Error(), "oops") {
		t.Fatal("Expected error saying that 'oops' is not a valid number.")
	}
}

func TestSummaryValidConfig(t *testing.T) {
	validCfg := strings.Replace(summary_config, "$QUANTILES", "{0.5: 0.05, 0.9: 0.01, 0.99: 0.001}", 1)
	cfg := loadOrFail(t, validCfg)
	metric := (*(cfg.Metrics))[0]
	if len(metric.Quantiles) != 3 || metric.Quantiles[0.5] != 0.05 || metric.Quantiles[0.99] != 0.001 {
		t.Fatalf("Error parsing bucket list: Got %v", metric.Buckets)
	}
}

func TestSummaryInvalidConfig(t *testing.T) {
	invalidCfg := strings.Replace(summary_config, "$QUANTILES", "[0.005, 0.2, 10]", 1)
	_, err := LoadConfigString([]byte(invalidCfg))
	if err == nil {
		t.Fatal("Expected error, because quantiles are a list and not a map.")
	}
}

func loadOrFail(t *testing.T, cfgString string) *Config {
	cfg, err := LoadConfigString([]byte(cfgString))
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
