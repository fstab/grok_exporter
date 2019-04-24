// Copyright 2018 The grok_exporter Authors
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

package template

import (
	"testing"
)

func TestGsubFunction(t *testing.T) {
	templateString := "{{gsub .url \".*id=([^&]*).*\" \"\\\\1\"}}"
	template, err := New("test1", templateString)
	if err != nil {
		t.Fatalf("unexpected error parsing template: %v", err)
		return
	}
	result, err := template.Execute(map[string]string{
		"url": "http://example.com/foo.asp?id=42&source=github&foo=bar",
	})
	if err != nil {
		t.Fatalf("error executing gsub test1 template: %v", err)
	}
	if result != "42" {
		t.Fatalf("unexpected result form gsub test1 template: %v", result)
	}
}

func TestNestedGsub(t *testing.T) {
	templateString := "{{gsub (gsub .message \"e\" \"a\") \"r\" \"x\"}}"
	template, err := New("test2", templateString)
	if err != nil {
		t.Fatalf("unexpected error parsing template: %v", err)
		return
	}
	result, err := template.Execute(map[string]string{
		"message": "Sender verify failed",
	})
	if err != nil {
		t.Fatalf("error executing gsub test2 template: %v", err)
	}
	if result != "Sandax vaxify failad" {
		t.Fatalf("unexpected result form gsub test2 template: %v", result)
	}
}
