package exporter

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func loadPatterns(t *testing.T) *Patterns {
	patterns := InitPatterns()
	err := patterns.AddDir(filepath.Join(os.Getenv("GOPATH"), "src", "github.com", "fstab", "grok_exporter", "logstash-patterns-core", "patterns"))
	if err != nil {
		t.Errorf("Unexpected error: %v", err.Error())
	}
	return patterns
}

func TestAllRegexpsCompile(t *testing.T) {
	patterns := loadPatterns(t)
	for pattern, _ := range *patterns {
		_, err := Compile(fmt.Sprintf("%{%v}", pattern), patterns)
		if err != nil {
			t.Errorf("%v", err.Error())
		}
	}
}
