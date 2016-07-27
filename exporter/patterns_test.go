package exporter

import (
	"fmt"
	"os"
	"testing"
)

func TestLoadPatternFiles(t *testing.T) {
	p := InitPatterns()
	path := fmt.Sprintf("%v/src/github.com/fstab/grok_exporter/logstash-patterns-core/patterns", os.Getenv("GOPATH"))
	err := p.AddDir(path)
	if err != nil {
		t.Errorf("Unexpected error: %v", err.Error())
	}
}
