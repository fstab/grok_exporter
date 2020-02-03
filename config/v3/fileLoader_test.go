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
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

const file1Contents = "Contents of file1.yaml"
const file2Contents = "Contents of file2.yaml"

func TestLoadDirSuccess(t *testing.T) {
	testDir := setUp(t)
	defer tearDown(t, testDir)
	result, err := NewFileLoader().LoadDir(testDir)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(result) != 2 {
		t.Fatalf("expected to read 2 files, but found %v", len(result))
	}
}

func TestLoadDirNotFound(t *testing.T) {
	dir := "/tmp/not/found"
	_, err := NewFileLoader().LoadDir(dir)
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
	_, err = NewFileLoader().LoadDir(testDir)
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
	files, err := NewFileLoader().LoadGlob(filepath.Join(testDir, "*.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 yaml files, but found %v", len(files))
	}
}

func TestLoadFile(t *testing.T) {
	testDir := setUp(t)
	defer tearDown(t, testDir)
	contents, err := NewFileLoader().LoadGlob(filepath.Join(testDir, "file1.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(contents) != 1 {
		t.Fatalf("expected 1 yaml files, but found %v", len(contents))
	}
	if !strings.Contains(contents[0].Contents, file1Contents) {
		t.Fatalf("unexpected contents of file1.yaml")
	}
}

func setUp(t *testing.T) string {
	dir, err := ioutil.TempDir("", "grok_exporter")
	if err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}
	err = ioutil.WriteFile(path.Join(dir, "file1.yaml"), []byte(file1Contents), 0644)
	if err != nil {
		t.Fatalf("unexpected error writing file1.yaml: %v", err)
	}
	err = ioutil.WriteFile(path.Join(dir, "file2.yaml"), []byte(file2Contents), 0644)
	if err != nil {
		t.Fatalf("unexpected error writing file2.yaml: %v", err)
	}
	return dir
}

func tearDown(t *testing.T, testDir string) {
	err := os.RemoveAll(testDir)
	if err != nil {
		t.Fatalf("unexpected error removing %v: %v", testDir, err)
	}
}
