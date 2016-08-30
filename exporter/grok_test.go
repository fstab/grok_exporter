package exporter

import (
	"strings"
	"testing"
)

func TestAllRegexpsCompile(t *testing.T) {
	patterns := loadPatternDir(t)
	for pattern := range *patterns {
		_, err := Compile("%{"+pattern+"}", patterns)
		if err != nil {
			t.Errorf("%v", err.Error())
		}
	}
}

func TestUnknownGrokPattern(t *testing.T) {
	patterns := loadPatternDir(t)
	_, err := Compile("%{USER} [a-z] %{SOME_UNKNOWN_PATTERN}.*", patterns)
	if err == nil || !strings.Contains(err.Error(), "SOME_UNKNOWN_PATTERN") {
		t.Error("expected error message saying which pattern is undefined.")
	}
}

func TestInvalidRegexp(t *testing.T) {
	patterns := loadPatternDir(t)
	_, err := Compile("%{USER} [a-z] \\", patterns) // wrong because regex cannot end with backslash
	if err == nil || !strings.Contains(err.Error(), "%{USER} [a-z] \\") {
		t.Error("expected error message saying which pattern is invalid.")
	}
}

func TestNamedCaptureGroup(t *testing.T) {
	patterns := loadPatternDir(t)
	regex, err := Compile("User %{USER:user} has logged in.", patterns)
	if err != nil {
		t.Error(err)
	}
	found := regex.Gsub("User fabian has logged in.", "\\k<user>")
	if found != "fabian" {
		t.Errorf("Expected to capture 'fabian', but captured '%v'.", found)
	}
}
