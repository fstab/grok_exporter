package exporter

import (
	"fmt"
	"regexp"
	"strings"
)

// Compile a grok pattern string into a regular expression.
func Compile(pattern string, patterns *Patterns, libonig *OnigurumaLib) (*OnigurumaRegexp, error) {
	regex, err := expand(pattern, patterns)
	if err != nil {
		return nil, err
	}
	result, err := libonig.Compile(regex)
	if err != nil {
		return nil, fmt.Errorf("failed to compile pattern %v: error in regular expression %v: %v", pattern, regex, err.Error())
	}
	return result, nil
}

func VerifyFieldNames(m *MetricConfig, regex *OnigurumaRegexp) error {
	for _, label := range m.Labels {
		if !regex.HasCaptureGroup(label.GrokFieldName) {
			return fmt.Errorf("grok field %v not found in match pattern", label.GrokFieldName)
		}
	}
	if m.Value != "" {
		if !regex.HasCaptureGroup(m.Value) {
			return fmt.Errorf("grok field %v not found in match pattern", m.Value)
		}
	}
	return nil
}

// PATTERN_RE matches the %{..} patterns. There are three possibilities:
// 1) %{USER}               - grok pattern
// 2) %{IP:clientip}        - grok pattern with name
// 3) %{INT:clientport:int} - grok pattern with name and type (type is currently ignored)
const PATTERN_RE = `%{(.+?)}`

// Expand recursively resolves all grok patterns %{..} and returns a regular expression.
func expand(pattern string, patterns *Patterns) (string, error) {
	result := pattern
	for i := 0; i < 1000; i++ { // After 1000 replacements, we assume this is an infinite loop and abort.
		match := regexp.MustCompile(PATTERN_RE).FindStringSubmatch(result)
		if match == nil {
			// No match means all grok patterns %{..} are expanded. We are done.
			return result, nil
		}
		parts := strings.Split(match[1], ":")
		regex, exists := patterns.Find(parts[0])
		if !exists {
			return "", fmt.Errorf("Pattern %v not defined.", match[0])
		}
		var replacement string
		switch {
		case len(parts) == 1:
			// If the grok pattern has no name, we don't need to capture, so we use ?:
			replacement = fmt.Sprintf("(?:%v)", regex)
		case len(parts) == 2 || len(parts) == 3:
			// If the grok pattern has a name, we create a named capturing group with ?<>
			replacement = fmt.Sprintf("(?<%v>%v)", parts[1], regex)
		default:
			return "", fmt.Errorf("%v is not a valid pattern.", match[0])
		}
		result = strings.Replace(result, match[0], replacement, -1)
	}
	return "", fmt.Errorf("Deep recursion while expanding pattern '%v'.", pattern)
}
