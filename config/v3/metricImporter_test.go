// Copyright 2020 The grok_exporter Authors
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

package v3

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

const config = `
global:
    config_version: 3
input:
    type: stdin
metrics:
    - type: counter
      name: metric_defined_in_config_yaml
      help: Metric defined in config.yaml
      match: ERROR
imports:
    - type: metrics
      file: PLACEHOLDER
      defaults:
          path: /var/log/syslog/*
          labels:
              logfile: '{{base .logfile}}'
`

const file_1_yaml = `
- type: counter
  name: metric_defined_in_file_1_yaml
  help: Metric defined in file1.yaml
  match: WARN
`

const file_2_yaml = `
- type: counter
  name: metric_1_defined_in_file_2_yaml
  help: Metric 1 defined in file2.yaml
  match: WARN
- type: counter
  name: metric_2_defined_in_file_2_yaml
  help: Metric 2 defined in file2.yaml
  match: WARN
`

func TestLoadDirSuccess(t *testing.T) {
	testDir := setUp(t)
	defer tearDown(t, testDir)
	result, err := loadDir(testDir)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(result) != 2 {
		t.Fatalf("expected to read 2 files, but found %v", len(result))
	}
}

func TestLoadDirNotFound(t *testing.T) {
	dir := "/tmp/not/found"
	_, err := loadDir(dir)
	if err == nil || !os.IsNotExist(err) {
		t.Fatal("expected file not found error")
	}
}

func TestLoadDirError(t *testing.T) {
	testDir := setUp(t)
	defer tearDown(t, testDir)
	notRegularFile := filepath.Join(testDir, "not-regular-file")
	err := os.Mkdir(notRegularFile, 0755)
	if err != nil {
		t.Fatalf("unexpected error creating %v: %v", notRegularFile, err)
	}
	_, err = loadDir(testDir)
	if err == nil || !strings.Contains(err.Error(), notRegularFile) {
		t.Fatalf("expected error message regarding %v not being a regular file", notRegularFile)
	}
}

func TestLoadGlob(t *testing.T) {
	testDir := setUp(t)
	defer tearDown(t, testDir)
	notRegularFile := filepath.Join(testDir, "not-regular-file")
	err := os.Mkdir(notRegularFile, 0755)
	if err != nil {
		t.Fatalf("unexpected error creating %v: %v", notRegularFile, err)
	}
	contents, err := loadGlob(filepath.Join(testDir, "*.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contents) != 2 {
		t.Fatalf("expected 2 yaml files, but found %v", len(contents))
	}
}

func TestLoadFile(t *testing.T) {
	testDir := setUp(t)
	defer tearDown(t, testDir)
	contents, err := loadGlob(filepath.Join(testDir, "file1.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contents) != 1 {
		t.Fatalf("expected 1 yaml files, but found %v", len(contents))
	}
	if !strings.Contains(contents[0].contents, "Metric defined in file1.yaml") {
		t.Fatalf("unexpected contents of file1.yaml")
	}
}

func TestImportMetrics(t *testing.T) {
	testDir := setUp(t)
	defer tearDown(t, testDir)
	files := path.Join(testDir, "*.yaml")
	cfg := &Config{}
	err := yaml.Unmarshal([]byte(strings.Replace(config, "PLACEHOLDER", files, 1)), &cfg)
	if err != nil {
		t.Fatalf("unexpeced error while unmarshalling config: %v", err)
	}
	imported, err := importMetrics(cfg.Imports)
	if err != nil {
		t.Fatalf("unexpected error while importing metric configs: %v", err)
	}
	if len(imported) != 3 {
		t.Fatalf("expected 3 imported metrics, but found %v", len(imported))
	}
}

func setUp(t *testing.T) string {
	dir, err := ioutil.TempDir("", "grok_exporter")
	if err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}
	err = ioutil.WriteFile(path.Join(dir, "file1.yaml"), []byte(file_1_yaml), 0644)
	if err != nil {
		t.Fatalf("unexpected error writing file1.yaml: %v", err)
	}
	err = ioutil.WriteFile(path.Join(dir, "file2.yaml"), []byte(file_2_yaml), 0644)
	if err != nil {
		t.Fatalf("unexpected error writing file2.yaml: %v", err)
	}
	return dir
}

func tearDown(t *testing.T, dir string) {
	// todo: rm -r dir
}
