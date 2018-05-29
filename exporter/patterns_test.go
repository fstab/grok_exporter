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

package exporter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPatternsLoadSuccessfully(t *testing.T) {
	loadPatternDir(t)
}

func loadPatternDir(t *testing.T) *Patterns {
	p := InitPatterns()
	if len(*p) != 0 {
		t.Errorf("Expected initial pattern list to be empty, but got len = %v\n", len(*p))
	}
	patternDir := filepath.Join(os.Getenv("GOPATH"), "src", "github.com", "fstab", "grok_exporter", "logstash-patterns-core", "patterns")
	err := p.AddDir(patternDir)
	if err != nil {
		t.Errorf("Unexpected error: %v", err.Error())
	}
	if len(*p) == 0 {
		t.Errorf("Patterns are still empty after loading the pattern directory %v. If the directory is empty, run 'git submodule update --init --recursive'.", patternDir)
	}
	return p
}

func TestOptionalLabels(t *testing.T) {
	for _, expected := range []struct {
		input string
		match bool
		foo   string
		bar   string
	}{
		{"foobar", true, "foo", "bar"},
		{"foobaz", true, "foo", ""},
		{"foo", true, "foo", ""},
		{"bar", false, "", ""},
	} {
		matchResult := matchFooBar(t, expected.input)
		if matchResult.IsMatch() != expected.match {
			t.Fatalf("Expected match(%v)=%v, but got %v", expected.input, expected.match, matchResult.IsMatch())
		}
		actualFoo, err := matchResult.Get("foo")
		if err != nil {
			t.Fatal(err.Error())
		}
		if actualFoo != expected.foo {
			t.Fatalf("Expected %v to return foo=%v, but got foo=%v", expected.input, expected.foo, actualFoo)
		}
		actualBar, err := matchResult.Get("bar")
		if err != nil {
			t.Fatal(err.Error())
		}
		if actualBar != expected.bar {
			t.Fatalf("Expected %v to return bar=%v, but got bar=%v", expected.input, expected.bar, actualBar)
		}
		matchResult.Free()
	}
}

func matchFooBar(t *testing.T, input string) *OnigurumaMatchResult {
	p := InitPatterns()
	p.AddPattern("FOO foo")
	p.AddPattern("BAR bar")
	p.AddPattern("FOOBAR %{FOO:foo}%{BAR:bar}?")

	libonig, err := InitOnigurumaLib()
	if err != nil {
		t.Fatal(err.Error())
	}
	regex, err := Compile("%{FOOBAR}", p, libonig)
	if err != nil {
		t.Fatal(err.Error())
	}
	matchResult, err := regex.Match(input)
	if err != nil {
		t.Fatal(err.Error())
	}
	return matchResult
}
