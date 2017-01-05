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
	} {
		parsedTemplate, err := New(fmt.Sprintf("test%v", i), test.template)
		if err != nil {
			t.Fatalf("unexpected error in template %v: %v", i, err)
			return
		}
		assertArrayEqualsIgnoreOrder(t, parsedTemplate.ReferencedGrokFields(), test.expectedGrokFields)
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
