package exporter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPatternDir(t *testing.T) {
	p := InitPatterns()
	if len(*p) != 0 {
		t.Errorf("Expected initial pattern list to be empty, but got len = %v\n", len(*p))
	}
	loadPatternDir(t, p)
}

func loadPatternDir(t *testing.T, p *Patterns) {
	patternDir := filepath.Join(os.Getenv("GOPATH"), "src", "github.com", "fstab", "grok_exporter", "logstash-patterns-core", "patterns")
	err := p.AddDir(patternDir)
	if err != nil {
		t.Errorf("Unexpected error: %v", err.Error())
	}
	if len(*p) == 0 {
		t.Errorf("Patterns are still empty after loading the pattern directory %v. If the directory is empty, run 'git submodule update --init --recursive'.", patternDir)
	}
}
