package exporter

import (
	"gopkg.in/yaml.v2"
	"strings"
	"testing"
)

func TestGrok(t *testing.T) {
	libonig, err := InitOnigurumaLib()
	if err != nil {
		t.Fatal(err)
	}
	patterns := loadPatternDir(t)
	t.Run("compile all patterns", func(t *testing.T) {
		testCompileAllPatterns(t, patterns, libonig)
	})
	t.Run("compile unknown pattern", func(t *testing.T) {
		testCompileUnknownPattern(t, patterns, libonig)
	})
	t.Run("compile invalid regexp", func(t *testing.T) {
		testCompileInvalidRegexp(t, patterns, libonig)
	})
	t.Run("verify capture group", func(t *testing.T) {
		testVerifyCaptureGroup(t, patterns, libonig)
	})
}

func testCompileAllPatterns(t *testing.T, patterns *Patterns, libonig *OnigurumaLib) {
	for pattern := range *patterns {
		_, err := Compile("%{"+pattern+"}", patterns, libonig)
		if err != nil {
			t.Errorf("%v", err.Error())
		}
	}
}

func testCompileUnknownPattern(t *testing.T, patterns *Patterns, libonig *OnigurumaLib) {
	_, err := Compile("%{USER} [a-z] %{SOME_UNKNOWN_PATTERN}.*", patterns, libonig)
	if err == nil || !strings.Contains(err.Error(), "SOME_UNKNOWN_PATTERN") {
		t.Error("expected error message saying which pattern is undefined.")
	}
}

func testCompileInvalidRegexp(t *testing.T, patterns *Patterns, libonig *OnigurumaLib) {
	_, err := Compile("%{USER} [a-z] \\", patterns, libonig) // wrong because regex cannot end with backslash
	if err == nil || !strings.Contains(err.Error(), "%{USER} [a-z] \\") {
		t.Error("expected error message saying which pattern is invalid.")
	}
}

func testVerifyCaptureGroup(t *testing.T, patterns *Patterns, libonig *OnigurumaLib) {
	regex, err := Compile("host %{HOSTNAME:host} user %{USER:user} value %{NUMBER:val}.", patterns, libonig)
	if err != nil {
		t.Fatal(err)
	}
	expectOK(t, regex, `
            name: test
            value: val
            labels:
            - grok_field_name: user
              prometheus_label: user
            - grok_field_name: host
              prometheus_label: something`)
	expectOK(t, regex, `
            name: test`)
	expectError(t, regex, `
            name: test
            value: value
            labels:
            - grok_field_name: user
              prometheus_label: user`)
	expectError(t, regex, `
            name: test
            value: val
            labels:
            - grok_field_name: user2
              prometheus_label: user`)
	regex.Free()
}

func expectOK(t *testing.T, regex *OnigurumaRegexp, config string) {
	expect(t, regex, config, false)
}

func expectError(t *testing.T, regex *OnigurumaRegexp, config string) {
	expect(t, regex, config, true)
}

func expect(t *testing.T, regex *OnigurumaRegexp, config string, isErrorExpected bool) {
	cfg := &MetricConfig{}
	err := yaml.Unmarshal([]byte(config), cfg)
	if err != nil {
		t.Error(err)
	}
	err = VerifyFieldNames(cfg, regex)
	if isErrorExpected && err == nil {
		t.Error("Expected error, but got no error.")
	}
	if !isErrorExpected && err != nil {
		t.Error("Expected ok, but got error.")
	}
}
