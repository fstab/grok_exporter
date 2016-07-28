package exporter

import (
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
