// Copyright 2016 The grok_exporter Authors
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

package exporter

import (
	"github.com/fstab/grok_exporter/config/v2"
	"gopkg.in/yaml.v2"
	"strings"
	"testing"
	"text/template"
)

func TestGrok(t *testing.T) {
	libonig, err := InitOnigurumaLib()
	if err != nil {
		t.Fatal(err)
	}
	patterns := loadPatternDir(t)
	run(t, "compile all patterns", func(t *testing.T) {
		testCompileAllPatterns(t, patterns, libonig)
	})
	run(t, "compile unknown pattern", func(t *testing.T) {
		testCompileUnknownPattern(t, patterns, libonig)
	})
	run(t, "compile invalid regexp", func(t *testing.T) {
		testCompileInvalidRegexp(t, patterns, libonig)
	})
	run(t, "verify capture group", func(t *testing.T) {
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
            value: '{{.val}}'
            labels:
              user: '{{.user}}'
              host: '{{.host}}'`)
	expectOK(t, regex, `
            name: test`)
	expectError(t, regex, `
            name: test
            value: '{{.value}}'
            labels:
              user: '{{.user}}'`)
	expectError(t, regex, `
            name: test
            value: '{{.val}}'
            labels:
              user: '{{.user2}}'`)
	regex.Free()
}

func expectOK(t *testing.T, regex *OnigurumaRegexp, config string) {
	expect(t, regex, config, false)
}

func expectError(t *testing.T, regex *OnigurumaRegexp, config string) {
	expect(t, regex, config, true)
}

func expect(t *testing.T, regex *OnigurumaRegexp, config string, isErrorExpected bool) {
	cfg := &v2.MetricConfig{}
	err := yaml.Unmarshal([]byte(config), cfg)
	if err != nil {
		t.Fatal(err)
	}
	err = cfg.InitTemplates()
	if err != nil {
		t.Fatal(err)
	}
	err = VerifyFieldNames(cfg, regex)
	if isErrorExpected && err == nil {
		t.Fatal("Expected error, but got no error.")
	}
	if !isErrorExpected && err != nil {
		t.Fatal("Expected ok, but got error.")
	}
}

func TestReferencedGrokFields(t *testing.T) {
	grokFieldTest(t, "test1", "{{.count_total}} items are made of {{.material}}", "count_total", "material")
	grokFieldTest(t, "test2", "{{23 -}} < {{- 45}}")
	grokFieldTest(t, "test3", "{{.conca -}} < {{- .tenated}}", "conca", "tenated")
	grokFieldTest(t, "test4", "{{with $x := \"output\" | printf \"%q\"}}{{$x}}{{end}}{{.bla}}", "bla")
	grokFieldTest(t, "test5", "")
	// Templates not supported yet.
	// grokFieldTest(t, "test6", `
	//	{{define "T1"}}{{.value1_total}}{{end}}
	//	{{define "T2"}}{{.value2_total}}{{end}}
	//	{{define "T3"}}{{template "T1"}} / {{template "T2"}}{{end}}
	//	{{template "T3"}}`, "value1_total", "value2_total")
}

func grokFieldTest(t *testing.T, name, tmplt string, expectedFields ...string) {
	parsedTemplate, err := template.New(name).Parse(tmplt)
	if err != nil {
		t.Fatalf("%v: error parsing template: %v", name, err.Error())
	}
	actualFields := referencedGrokFields(parsedTemplate)
	if len(actualFields) != len(expectedFields) {
		t.Fatalf("%v: expected: %v, actual: %v", name, expectedFields, actualFields)
	}
	for _, actualField := range actualFields {
		found := false
		for _, expectedField := range expectedFields {
			if expectedField == actualField {
				found = true
			}
		}
		if !found {
			t.Fatalf("%v: expected: %v, actual: %v", name, expectedFields, actualFields)
		}
	}
}
