// Copyright 2016-2018 The grok_exporter Authors
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
    config_version: 2
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

func TestVersionDetection(t *testing.T) {
	expectVersion(t, exampleConfig, 2, false)
	expectVersion(t, strings.Replace(exampleConfig, "config_version: 2", "config_version: 1", 1), 1, false)
	expectVersion(t, strings.Replace(exampleConfig, "config_version: 2", "config_version:", 1), 1, true)
	expectVersion(t, strings.Replace(exampleConfig, "config_version: 2", "", 1), 1, true)
	_, _, err := findVersion(strings.Replace(exampleConfig, "config_version: 2", "config_version: a", 1))
	if err == nil {
		t.Fatalf("Expected error, because 'a' is not a number.")
	}
}

func expectVersion(t *testing.T, config string, expectedVersion int, warningExpected bool) {
	version, warn, err := findVersion(config)
	if err != nil {
		t.Fatalf("unexpected error while getting version info: %v", err.Error())
	}
	if warningExpected && len(warn) == 0 {
		t.Fatalf("didn't get warning for unversioned config file")
	}
	if !warningExpected && len(warn) > 0 {
		t.Fatalf("unexpected warning: %v", warn)
	}
	if version != expectedVersion {
		t.Fatalf("expected version %v, but found %v", expectedVersion, version)
	}
}
