package exporter

import (
	"testing"
)

func TestOniguruma(t *testing.T) {
	libonig, err := InitOnigurumaLib()
	if err != nil {
		t.Fatal(err)
	}
	t.Run("valid patterns", func(t *testing.T) {
		testValidPatterns(t, libonig)
	})
	t.Run("invalid patterns", func(t *testing.T) {
		testInvalidPatterns(t, libonig)
	})
	t.Run("valid capture groups", func(t *testing.T) {
		testValidCaptureGroups(t, libonig)
	})
	t.Run("invalid capture groups", func(t *testing.T) {
		testInvalidCaptureGroups(t, libonig)
	})
}

func testInvalidPatterns(t *testing.T, libonig *OnigurumaLib) {
	for _, pattern := range []string{
		".*[a-z]([0-9]",        // missing closing )
		"some\\",               // ends with \
		"some (?<g>.*)(?<>.*)", // empty group name
		".*abc)",               // missing opening (
	} {
		_, err := libonig.Compile(pattern)
		if err == nil {
			t.Errorf("Oniguruma compiles invalid pattern '%v' w/o returning an error.", pattern)
		}
	}
}

func testValidPatterns(t *testing.T, libonig *OnigurumaLib) {
	for _, data := range [][]string{
		[]string{"^.*[a-z]([0-9])$", "abc7abc7", "abc7abc"},
		[]string{"^some .*test\\s.*$", "some test 3", "some test3"},
		[]string{"^is\\]this$", "is]this", "is\\]this"},
		[]string{"^abc(.*abc)+$", "abcabcabc", "abc"},
	} {
		regex, err := libonig.Compile(data[0])
		if err != nil {
			t.Error(err)
		}
		successfulMatch, err := regex.Match(data[1])
		if err != nil {
			t.Error(err)
		}
		if !successfulMatch.IsMatch() {
			t.Errorf("pattern '%v' didn't match string '%v'", data[0], data[1])
		}
		successfulMatch.Free()
		unsuccessfulMatch, err := regex.Match(data[2])
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

func testValidCaptureGroups(t *testing.T, libonig *OnigurumaLib) {
	regex, err := libonig.Compile("^1st user (?<user>[a-z]*) ?2nd user (?<user>[a-z]+) value (?<val>[0-9]+)$")
	if err != nil {
		t.Error(err)
	}
	for _, data := range [][]string{
		[]string{"1st user fabian 2nd user grok value 7", "fabian", "7"},
		[]string{"1st user 2nd user grok value 789", "grok", "789"},
		[]string{"1st user somebody 2nd user else value 123", "somebody", "123"},
	} {
		result, err := regex.Match(data[0])
		if err != nil {
			t.Error(err)
		}
		user, err := result.Get("user")
		if err != nil {
			t.Error(err)
		}
		if user != data[1] {
			t.Errorf("Expected user %v, but got %v", data[1], user)
		}
		val, err := result.Get("val")
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

func testInvalidCaptureGroups(t *testing.T, libonig *OnigurumaLib) {
	regex, err := libonig.Compile("^1st user (?<user>[a-z]*) ?2nd user (?<user>[a-z]+) (?<x>.*)(.*)value (?<val>[0-9]*)$")
	if err != nil {
		t.Error(err)
	}
	match, err := regex.Match("1st user fabian 2nd user grok value 789")
	if err != nil {
		t.Error(err)
	}
	if !match.IsMatch() {
		t.Error("expected a match")
	}
	for _, data := range [][]string{
		[]string{"void", ""},
		[]string{"", ""},
	} {
		_, err := match.Get(data[0])
		if err == nil {
			t.Error("Expected error, because used non-existing capture group name.")
		}
	}
	val, err := match.Get("x")
	if err != nil {
		t.Error(err)
	}
	if val != "" {
		t.Errorf("Expected empty string, but got %v", val)
	}
	match.Free()
}
