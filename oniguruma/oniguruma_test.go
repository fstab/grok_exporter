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

package oniguruma

import (
	"testing"
)

func TestInvalidPatterns(t *testing.T) {
	for _, pattern := range []string{
		".*[a-z]([0-9]",        // missing closing )
		"some\\",               // ends with \
		"some (?<g>.*)(?<>.*)", // empty group name
		".*abc)",               // missing opening (
	} {
		_, err := Compile(pattern)
		if err == nil {
			t.Errorf("Oniguruma compiles invalid pattern '%v' w/o returning an error.", pattern)
		}
	}
}

func TestValidPatterns(t *testing.T) {
	for _, data := range [][]string{
		{"^.*[a-z]([0-9])$", "abc7abc7", "abc7abc"},
		{"^some .*test\\s.*$", "some test 3", "some test3"},
		{"^is\\]this$", "is]this", "is\\]this"},
		{"^abc(.*abc)+$", "abcabcabc", "abc"},
	} {
		regex, err := Compile(data[0])
		if err != nil {
			t.Error(err)
		}
		successfulMatch, err := regex.Search(data[1])
		if err != nil {
			t.Error(err)
		}
		if !successfulMatch.IsMatch() {
			t.Errorf("pattern '%v' didn't match string '%v'", data[0], data[1])
		}
		successfulMatch.Free()
		unsuccessfulMatch, err := regex.Search(data[2])
		if err != nil {
			t.Error(err)
		}
		if unsuccessfulMatch.IsMatch() {
			t.Errorf("pattern '%v' matched string '%v'", data[0], data[2])
		}
		unsuccessfulMatch.Free()
		regex.Free()
	}
}

func TestValidCaptureGroups(t *testing.T) {
	regex, err := Compile("^1st user (?<user>[a-z]*) ?2nd user (?<user>[a-z]+) value (?<val>[0-9]+)$")
	if err != nil {
		t.Error(err)
	}
	for _, data := range [][]string{
		{"1st user fabian 2nd user grok value 7", "fabian", "7"},
		{"1st user 2nd user grok value 789", "grok", "789"},
		{"1st user somebody 2nd user else value 123", "somebody", "123"},
	} {
		result, err := regex.Search(data[0])
		if err != nil {
			t.Error(err)
		}
		user, err := result.GetCaptureGroupByName("user")
		if err != nil {
			t.Error(err)
		}
		if user != data[1] {
			t.Errorf("Expected user %v, but got %v", data[1], user)
		}
		val, err := result.GetCaptureGroupByName("val")
		if err != nil {
			t.Error(err)
		}
		if val != data[2] {
			t.Errorf("Expected val %v, but got %v", data[2], val)
		}
		result.Free()
	}
	regex.Free()
}

func TestInvalidCaptureGroups(t *testing.T) {
	regex, err := Compile("^1st user (?<user>[a-z]*) ?2nd user (?<user>[a-z]+) (?<x>.*)(.*)value (?<val>[0-9]*)$")
	if err != nil {
		t.Error(err)
	}
	match, err := regex.Search("1st user fabian 2nd user grok value 789")
	if err != nil {
		t.Error(err)
	}
	if !match.IsMatch() {
		t.Error("expected a match")
	}
	for _, data := range [][]string{
		{"void", ""},
		{"", ""},
	} {
		_, err := match.GetCaptureGroupByName(data[0])
		if err == nil {
			t.Error("Expected error, because used non-existing capture group name.")
		}
	}
	val, err := match.GetCaptureGroupByName("x")
	if err != nil {
		t.Error(err)
	}
	if val != "" {
		t.Errorf("Expected empty string, but got %v", val)
	}
	match.Free()
}
