// Copyright 2016-2020 The grok_exporter Authors
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

package config

import (
	"strings"
	"testing"
)

const exampleConfig = `
global:
    PLACEHOLDER
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
server:
    protocol: https
    port: 1111
`

const globalMissing = `
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
server:
    protocol: https
    port: 1111
`

const wrongFile = `
some random
other content
because the user might accidentally
use the wrong file as command line parameter
`

func TestVersionOk(t *testing.T) {
	expectVersion(t, "config_version: 1", 1, false, false)
	expectVersion(t, "config_version: 2", 2, false, false)
	expectVersion(t, "config_version: 3", 3, false, false)
}

func TestVersionInvalid(t *testing.T) {
	expectVersion(t, "config_version: a", 0, false, true)
	expectVersion(t, "config_version", 0, false, true)
}

func TestVersionGlobalMissing(t *testing.T) {
	_, _, err := findVersion(globalMissing)
	if err == nil {
		t.Fatalf("didn't get error while testing config with missing global section")
	}
}

func TestVersionWrongFile(t *testing.T) {
	_, _, err := findVersion(wrongFile)
	if err == nil {
		t.Fatalf("didn't get error while testing config with missing global section")
	}
}

func expectVersion(t *testing.T, placeholderReplacement string, expectedVersion int, warningExpected bool, errorExpected bool) {
	config := strings.Replace(exampleConfig, "PLACEHOLDER", placeholderReplacement, 1)
	version, warn, err := findVersion(config)
	switch {
	case errorExpected:
		if err == nil {
			t.Fatalf("didn't get error while testing version %q", placeholderReplacement)
		}
		return
	case err != nil:
		t.Fatalf("unexpected error while testing version %q: %v", placeholderReplacement, err.Error())
	case warningExpected:
		if len(warn) == 0 {
			t.Fatalf("didn't get warning while testing version %q", placeholderReplacement)
		}
		return
	case len(warn) > 0:
		t.Fatalf("unexpected warning while testing version %q: %v", placeholderReplacement, warn)
	case version != expectedVersion:
		t.Fatalf("expected version %v but found %v while testing version %q", expectedVersion, version, placeholderReplacement)
	}
}
