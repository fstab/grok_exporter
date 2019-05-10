// Copyright 2016-2019 The grok_exporter Authors
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

package tailer

import (
	"fmt"
	"github.com/fstab/grok_exporter/config/v2"
	"strings"
	"testing"
)

func TestWebhookTextSingle(t *testing.T) {
	c := &v2.InputConfig{
		Type:                     "webhook",
		WebhookPath:              "/webhook",
		WebhookFormat:            "text_single",
		WebhookJsonSelector:      "",
		WebhookTextBulkSeparator: "",
	}

	message := "2016-04-18 09:33:27 H=(85.214.241.101) [114.37.190.56] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted"
	fmt.Printf("Sending Payload: %v", message)
	lines := WebhookProcessBody(c, []byte(message))
	if len(lines) != 1 {
		t.Fatal("Expected 1 line processed")
	}
	if lines[0] != message {
		t.Fatal("Expected line to match")
	}
}

func TestWebhookTextBulk(t *testing.T) {
	c := &v2.InputConfig{
		Type:                     "webhook",
		WebhookPath:              "/webhook",
		WebhookFormat:            "text_bulk",
		WebhookJsonSelector:      "",
		WebhookTextBulkSeparator: "\n\n",
	}

	messages := []string{
		"2016-04-18 09:33:27 H=(85.214.241.101) [114.37.190.56] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted",
		"2016-04-18 12:28:04 H=(85.214.241.101) [118.161.243.219] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted",
		"2016-04-18 19:16:30 H=(85.214.241.101) [114.24.5.12] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted",
	}
	payload := strings.Join(messages, c.WebhookTextBulkSeparator)
	fmt.Printf("Sending Payload: %v", payload)
	lines := WebhookProcessBody(c, []byte(payload))
	if len(lines) != len(messages) {
		t.Fatal("Expected number of lines to equal number of messages")
	}
	for i, _ := range messages {
		if messages[i] != lines[i] {
			t.Fatal("Expected line to match")
		}
	}
}

func TestWebhookTextBulkNegative(t *testing.T) {
	// Expected to fail because of WebhookTextbulkSeparator
	c := &v2.InputConfig{
		Type:                     "webhook",
		WebhookPath:              "/webhook",
		WebhookFormat:            "text_bulk",
		WebhookJsonSelector:      "",
		WebhookTextBulkSeparator: "\n\n",
	}

	messages := []string{
		"2016-04-18 09:33:27 H=(85.214.241.101) [114.37.190.56] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted",
		"2016-04-18 12:28:04 H=(85.214.241.101) [118.161.243.219] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted",
		"2016-04-18 19:16:30 H=(85.214.241.101) [114.24.5.12] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted",
	}
	payload := strings.Join(messages, "\t\t")
	fmt.Printf("Sending Payload: %v", payload)
	lines := WebhookProcessBody(c, []byte(payload))
	if len(lines) == len(messages) {
		t.Fatal("Expected number of lines to equal number of messages")
	}
}

func TestWebhookJsonSingle(t *testing.T) {
	// This test follows the format of Logstash HTTP Non-Bulk Output
	// https://www.elastic.co/guide/en/logstash/current/plugins-outputs-http.html
	// format="json"
	c := &v2.InputConfig{
		Type:                     "webhook",
		WebhookPath:              "/webhook",
		WebhookFormat:            "json_single",
		WebhookJsonSelector:      ".message",
		WebhookTextBulkSeparator: "",
	}

	message := "2016-04-18 09:33:27 H=(85.214.241.101) [114.37.190.56] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted"
	s := createJsonBlob(message)
	fmt.Printf("Sending Payload: %v", s)
	lines := WebhookProcessBody(c, []byte(s))
	if len(lines) != 1 {
		t.Fatal("Expected 1 line processed")
	}
	if lines[0] != message {
		t.Fatal("Expected line to match")
	}
}

func TestWebhookJsonSingleNegativeWebhookJsonSelector(t *testing.T) {
	// Expected to fail because of Mismatching WebhookJsonSelector
	c := &v2.InputConfig{
		Type:                     "webhook",
		WebhookPath:              "/webhook",
		WebhookFormat:            "json_single",
		WebhookJsonSelector:      ".messageMISMATCH",
		WebhookTextBulkSeparator: "",
	}

	message := "2016-04-18 09:33:27 H=(85.214.241.101) [114.37.190.56] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted"
	s := createJsonBlob(message)
	fmt.Printf("Sending Payload: %v", s)
	lines := WebhookProcessBody(c, []byte(s))
	if len(lines) != 0 {
		t.Fatal("Expected 1 line processed")
	}
}

func TestWebhookJsonSingleNegativeMalformedJson(t *testing.T) {
	// Expected to fail because of Mismatching WebhookJsonSelector
	c := &v2.InputConfig{
		Type:                     "webhook",
		WebhookPath:              "/webhook",
		WebhookFormat:            "json_single",
		WebhookJsonSelector:      ".messageMISMATCH",
		WebhookTextBulkSeparator: "",
	}

	message := "2016-04-18 09:33:27 H=(85.214.241.101) [114.37.190.56] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted"
	s := createMalformedJsonBlob(message)
	fmt.Printf("Sending Payload: %v", s)
	lines := WebhookProcessBody(c, []byte(s))
	if len(lines) != 0 {
		t.Fatal("Expected 0 lines processed")
	}
}

func TestWebhookJsonBulk(t *testing.T) {
	// This test follows the format of Logstash HTTP Non-Bulk Output
	// https://www.elastic.co/guide/en/logstash/current/plugins-outputs-http.html
	// format="json_batch"

	c := &v2.InputConfig{
		Type:                     "webhook",
		WebhookPath:              "/webhook",
		WebhookFormat:            "json_bulk",
		WebhookJsonSelector:      ".message",
		WebhookTextBulkSeparator: "",
	}

	messages := []string{
		"2016-04-18 09:33:27 H=(85.214.241.101) [114.37.190.56] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted",
		"2016-04-18 12:28:04 H=(85.214.241.101) [118.161.243.219] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted",
		"2016-04-18 19:16:30 H=(85.214.241.101) [114.24.5.12] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted",
	}

	blobs := []string{}
	for _, message := range messages {
		blobs = append(blobs, createJsonBlob(message))
	}
	s := "[\n" + strings.Join(blobs, ",\n") + "\n]"
	fmt.Printf("Sending Payload: %v", s)
	lines := WebhookProcessBody(c, []byte(s))
	if len(lines) != len(messages) {
		t.Fatal("Expected number of lines to equal number of messages")
	}
	for i, _ := range messages {
		if messages[i] != lines[i] {
			t.Fatal("Expected line to match")
		}
	}
}

func TestWebhookJsonBulkNegativeMalformedJson(t *testing.T) {
	// This test follows the format of Logstash HTTP Non-Bulk Output
	// https://www.elastic.co/guide/en/logstash/current/plugins-outputs-http.html
	// format="json_batch"

	c := &v2.InputConfig{
		Type:                     "webhook",
		WebhookPath:              "/webhook",
		WebhookFormat:            "json_bulk",
		WebhookJsonSelector:      ".message",
		WebhookTextBulkSeparator: "",
	}

	messages := []string{
		"2016-04-18 09:33:27 H=(85.214.241.101) [114.37.190.56] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted",
		"2016-04-18 12:28:04 H=(85.214.241.101) [118.161.243.219] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted",
		"2016-04-18 19:16:30 H=(85.214.241.101) [114.24.5.12] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted",
	}

	blobs := []string{}
	for _, message := range messages {
		blobs = append(blobs, createMalformedJsonBlob(message))
	}
	s := "[\n" + strings.Join(blobs, ",\n") + "\n]"
	fmt.Printf("Sending Payload: %v", s)
	lines := WebhookProcessBody(c, []byte(s))
	if len(lines) != 0 {
		t.Fatal("Expected 0 lines processed")
	}
}

func createJsonBlob(message string) string {
	s := fmt.Sprintf(`{
  "message": "%v",
  "host": "1.1.1.1",
  "document": {
    "apiVersion": "v1alpha1",
    "kind": "TestJsonOutputLogMessage"
  },
  "@version": "1",
  "@timestamp": "2010-01-01T01:01:01.101Z"
}`, message)
	return s
}

func createMalformedJsonBlob(message string) string {
	// Malformed because missing the `{` after document
	// "document": {
	s := fmt.Sprintf(`{
  "message": "%v",
  "host": "1.1.1.1",
  "document":
    "apiVersion": "v1alpha1",
    "kind": "TestJsonOutputLogMessage"
  },
  "@version": "1",
  "@timestamp": "2010-01-01T01:01:01.101Z"
}`, message)
	return s
}
