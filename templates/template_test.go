package templates

import (
	"fmt"
	"strconv"
	"testing"
)

func TestReferencedGrokFields(t *testing.T) {
	for i, test := range []struct {
		template           string
		expectedGrokFields []string
	}{
		{
			template:           "{{.count_total}} items are made of {{.material}}",
			expectedGrokFields: []string{"count_total", "material"},
		},
		{
			template:           "{{23 -}} < {{- 45}}",
			expectedGrokFields: []string{},
		},
		{
			template:           "{{.conca -}} < {{- .tenated}}",
			expectedGrokFields: []string{"conca", "tenated"},
		},
		{
			template:           "{{with $x := \"output\" | printf \"%q\"}}{{$x}}{{end}}{{.bla}}",
			expectedGrokFields: []string{"bla"},
		},
		{
			template:           "{{.hello}} world, {{timestamp \"02/01/2006 - 15:04:05.000\" .time}}",
			expectedGrokFields: []string{"hello", "time"},
		},
		{
			// Issue #10
			template:           "{{if eq .field \"value\"}}text{{end}}",
			expectedGrokFields: []string{"field"},
		},
		{
			// Issue #10
			template:           "{{if eq .field1 .field2}}{{.field3}}{{else}}{{.field4}}{{end}}",
			expectedGrokFields: []string{"field1", "field2", "field3", "field4"},
		},
		{
			// Issue #10
			template:           "{{if eq .field1 .field2}}{{.field3}}{{else if eq .field4 .field5}}{{.field6}}{{else}}{{.field7}}{{end}}",
			expectedGrokFields: []string{"field1", "field2", "field3", "field4", "field5", "field6", "field7"},
		},
		{
			// Issue #10
			template:           "{{define \"T1\"}} {{.field}} {{end}} {{template \"T1\" .}}",
			expectedGrokFields: []string{"field"},
		},
	} {
		parsedTemplate, err := New(fmt.Sprintf("test_1_%v", i), test.template)
		if err != nil {
			t.Fatalf("unexpected error in template %v: %v", i, err)
			return
		}
		assertArrayEqualsIgnoreOrder(t, test.expectedGrokFields, parsedTemplate.ReferencedGrokFields())
	}
}

func TestExecute(t *testing.T) {
	for i, test := range []struct {
		template       string
		grokValues     map[string]string
		expectedResult string
	}{
		{
			template: "{{.count_total}} items are made of {{.material}}",
			grokValues: map[string]string{
				"count_total": "3",
				"material":    "metal",
			},
			expectedResult: "3 items are made of metal",
		},
		{
			template: "{{define \"T1\"}}{{.field}}{{end}}{{template \"T1\" .}}",
			grokValues: map[string]string{
				"field": "hello",
			},
			expectedResult: "hello",
		},
	} {
		parsedTemplate, err := New(fmt.Sprintf("test_2_%v", i), test.template)
		if err != nil {
			t.Fatalf("unexpected error in template %v: %v", i, err)
			return
		}
		result, err := parsedTemplate.Execute(test.grokValues)
		if err != nil {
			t.Fatalf("unexpected error while executing template %v: %v", i, err)
			return
		}
		if result != test.expectedResult {
			t.Fatalf("Expected \"%v\", but got \"%v\".", test.expectedResult, result)
			return
		}
	}
}

func TestTimestampParser(t *testing.T) {
	// log4j examples: http://log4jtester.com
	template, err := New("date", "{{timestamp \"2006-01-02 15:04:05,000\" .date}}")
	if err != nil {
		t.Fatalf("unexpected error parsing template: %v", err)
		return
	}
	result1 := evalTimestamp(t, template, "2015-07-26 15:01:33,665")
	result2 := evalTimestamp(t, template, "2015-07-26 15:02:33,665")
	durationSeconds := int(result2 - result1)
	if durationSeconds != 60 {
		t.Fatalf("expected 60 seconds difference between the two timestamps, but got %v seconds", durationSeconds)
	}
}

func evalTimestamp(t *testing.T, template Template, value string) float64 {
	resultString, err := template.Execute(map[string]string{
		"date": value,
	})
	if err != nil {
		t.Fatalf("unexpected error parsing date: %v", err)
	}
	result, err := strconv.ParseFloat(resultString, 64)
	if err != nil {
		t.Fatalf("template returned invalid float64: %v", err)
	}
	return result
}

func TestCommaWarning(t *testing.T) {
	// 0 commas
	_, err := New("date", "{{timestamp \"2006-01-02 15:04:05.000\" .date}}")
	if err != nil {
		t.Fatalf("unexpected error parsing template: %v", err)
	}
	// 1 comma ok
	_, err = New("date", "{{timestamp \"2006-01-02 15:04:05,000\" .date}}")
	if err != nil {
		t.Fatalf("unexpected error parsing template: %v", err)
	}
	// 1 comma wrong
	_, err = New("date", "{{timestamp \"2006-01-02, 15:04:05\" .date}}")
	if err == nil {
		t.Fatal("expected error, but got ok.")
	}
	// more than 1 comma
	_, err = New("date", "{{timestamp \"2006-01-02 15:04:05,999,000\" .date}}")
	if err == nil {
		t.Fatal("expected error, but got ok.")
	}
}

func assertArrayEqualsIgnoreOrder(t *testing.T, expected, actual []string) {
	if len(expected) != len(actual) {
		t.Fatalf("expected: %v, actual: %v", expected, actual)
	}
	for _, act := range actual {
		found := false
		for _, exp := range expected {
			if act == exp {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected: %v, actual: %v", expected, actual)
		}
	}
}
