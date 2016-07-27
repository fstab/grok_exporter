package exporter

import (
	"strings"
	"testing"
)

const config = `
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

func TestLoadConfig(t *testing.T) {
	cfg, err := LoadConfigString([]byte(config))
	if err != nil {
		t.Errorf("Failed to read config: %v", err.Error())
	}
	if !equalsIgnoreIndentation(cfg.String(), config) {
		t.Errorf("Expected:\n%v\nActual:\n%v\n", config, cfg)
	}
}

func equalsIgnoreIndentation(a string, b string) bool {
	aLines := stripEmptyLines(strings.Split(a, "\n"))
	bLines := stripEmptyLines(strings.Split(b, "\n"))
	if len(aLines) != len(bLines) {
		return false
	}
	for i, _ := range aLines {
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
