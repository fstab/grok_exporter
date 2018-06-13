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

package exporter

import (
	"github.com/fstab/grok_exporter/oniguruma"
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
		searchResult := matchFooBar(t, expected.input)
		if searchResult.IsMatch() != expected.match {
			t.Fatalf("Expected match(%v)=%v, but got %v", expected.input, expected.match, searchResult.IsMatch())
		}
		actualFoo, err := searchResult.GetCaptureGroupByName("foo")
		if err != nil {
			t.Fatal(err.Error())
		}
		if actualFoo != expected.foo {
			t.Fatalf("Expected %v to return foo=%v, but got foo=%v", expected.input, expected.foo, actualFoo)
		}
		actualBar, err := searchResult.GetCaptureGroupByName("bar")
		if err != nil {
			t.Fatal(err.Error())
		}
		if actualBar != expected.bar {
			t.Fatalf("Expected %v to return bar=%v, but got bar=%v", expected.input, expected.bar, actualBar)
		}
		searchResult.Free()
	}
}

func matchFooBar(t *testing.T, input string) *oniguruma.SearchResult {
	p := InitPatterns()
	p.AddPattern("FOO foo")
	p.AddPattern("BAR bar")
	p.AddPattern("FOOBAR %{FOO:foo}%{BAR:bar}?")

	regex, err := Compile("%{FOOBAR}", p)
	if err != nil {
		t.Fatal(err.Error())
	}
	searchResult, err := regex.Search(input)
	if err != nil {
		t.Fatal(err.Error())
	}
	return searchResult
}

// The nginx example is taken from https://github.com/fstab/grok_exporter/issues/33
func TestNginxExample(t *testing.T) {
	p := loadPatternDir(t)
	p.AddPattern("ERRORDATE %{YEAR}/%{MONTHNUM}/%{MONTHDAY} %{TIME}")
	p.AddPattern("METHOD (OPTIONS|GET|HEAD|POST|PUT|DELETE|TRACE|CONNECT)")
	p.AddPattern("REQUEST_START %{METHOD:method} %{DATA:path} HTTP/%{DATA:http_version}")
	p.AddPattern("ADDITIONAL_INFO client: %{URIHOST:client}|server: %{URIHOST:server}|request: \"%{REQUEST_START:request}\"|upstream: \"%{URI:upstream}\"|host: \"%{URIHOST:host}\"|referrer: \"%{URI:referrer}\"")
	p.AddPattern("NGINX_ERROR ^%{ERRORDATE:time_local} \\[%{LOGLEVEL:level}\\] %{INT:process_id}#%{INT:thread_id}: \\*(%{INT:connection_id})? %{DATA:errormessage}(, %{ADDITIONAL_INFO})*$")

	regex, err := Compile("%{NGINX_ERROR}", p)
	if err != nil {
		t.Fatal(err.Error())
	}
	for _, testData := range []struct {
		input  string
		labels map[string]string
	}{
		{
			"2018/05/29 15:33:58 [error] 50#50: *5338007 no live upstreams while connecting to upstream, client: 5.128.43.14, server: example.com, request: \"GET /backend/api/event/events?_limit=0&startTime=2018-05-29T04:00:00%2B07:00 HTTP/2.0\", upstream: \"http://example.com/backend/api/event/events?_limit=0&startTime=2018-05-29T04:00:00%2B07:00\", host: \"example.com\", referrer: \"https://example.com/platform\"",
			map[string]string{
				"client":   "5.128.43.14",
				"server":   "example.com",
				"request":  "GET /backend/api/event/events?_limit=0&startTime=2018-05-29T04:00:00%2B07:00 HTTP/2.0",
				"upstream": "http://example.com/backend/api/event/events?_limit=0&startTime=2018-05-29T04:00:00%2B07:00",
				"host":     "example.com",
				"referrer": "https://example.com/platform",
			},
		},
		{"2018/05/29 15:35:33 [error] 50#50: *5340036 upstream prematurely closed connection while sending to client, client: 188.162.213.93, server: example.com, request: \"GET /backend/api/ HTTP/2.0\", upstream: \"http://172.19.0.8:80/backend/api\", host: \"example.com\", referrer: \"https://example.com/event\"",
			map[string]string{
				"client":   "188.162.213.93",
				"server":   "example.com",
				"request":  "GET /backend/api/ HTTP/2.0",
				"upstream": "http://172.19.0.8:80/backend/api",
				"host":     "example.com",
				"referrer": "https://example.com/event",
			},
		},
		{"2018/05/29 18:54:03 [crit] 50#50: *5411664 SSL_do_handshake() failed (SSL: error:1417D18C:SSL routines:tls_process_client_hello:version too low) while SSL handshaking, client: 208.93.213.176, server: 0.0.0.0:443",
			map[string]string{
				"client": "208.93.213.176",
				"server": "0.0.0.0:443",
				// all others empty
			},
		},
	} {
		searchResult, err := regex.Search(testData.input)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !searchResult.IsMatch() {
			t.Fatalf("The following line didn't match the NGINX_ERROR pattern: %v", testData.input)
		}
		for _, labelName := range []string{
			"client",
			"server",
			"request",
			"upstream",
			"host",
			"referrer",
		} {
			value, err := searchResult.GetCaptureGroupByName(labelName)
			if err != nil {
				t.Fatal(err.Error())
			}
			if value != testData.labels[labelName] {
				t.Fatalf("Expected label value '%v' but got '%v'.", testData.labels[labelName], value)
			}
		}
		searchResult.Free()
	}
}
