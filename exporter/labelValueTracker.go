// Copyright 2017 The grok_exporter Authors
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
	"fmt"
)

// Keep track of labels values for a metric.
type LabelValueTracker interface {
	Observe(labels map[string]string) (bool, error)
	Delete(labels map[string]string) ([]map[string]string, error)
}

type observedLabelValues struct {
	keys   []string
	values [][]string
}

func NewLabelValueTracker(labelNames []string) LabelValueTracker {
	keys := make([]string, len(labelNames))
	copy(keys, labelNames)
	return &observedLabelValues{
		keys:   keys,
		values: make([][]string, 0),
	}
}

func (observed *observedLabelValues) Observe(labels map[string]string) (bool, error) {
	if len(observed.keys) != len(labels) {
		return false, fmt.Errorf("error observing label values: trying to match %v label(s), but the metric was initialized with %v label(s).", len(labels), observed.keys)
	}
	for key := range labels {
		if !containsString(observed.keys, key) {
			return false, fmt.Errorf("error observing label %v: this label is not defined for the metric.", key)
		}
	}
	values := make([]string, len(observed.keys))
	for i, key := range observed.keys {
		if len(labels[key]) == 0 {
			return false, fmt.Errorf("error observing label %v: empty value not supported.", key) // TODO should we support this?
		}
		values[i] = labels[key]
	}
	if containsList(observed.values, values) {
		return false, nil
	} else {
		observed.values = append(observed.values, values)
		return true, nil
	}
}

func (observed *observedLabelValues) Delete(labels map[string]string) ([]map[string]string, error) {
	for key := range labels {
		if !containsString(observed.keys, key) {
			return nil, fmt.Errorf("error deleting label %v: this label is not defined for the metric.", key)
		}
	}
	values := make([]string, len(observed.keys))
	for i, key := range observed.keys {
		values[i] = labels[key] // will be "" if delete label unspecified
	}
	result := make([]map[string]string, 0)
	remainingObservedValues := make([][]string, 0, len(observed.values))
	for _, observedValues := range observed.values {
		if equalsIgnoreEmpty(values, observedValues) {
			values := make(map[string]string)
			for i := range observedValues {
				values[observed.keys[i]] = observedValues[i]
			}
			result = append(result, values)
		} else {
			remainingObservedValues = append(remainingObservedValues, observedValues)
		}
	}
	observed.values = remainingObservedValues
	return result, nil
}

func equals(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// test if the strings in 'a' are the same as the strings in 'b', but treat empty strings as a wildcard
func equalsIgnoreEmpty(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i]) > 0 && len(b[i]) > 0 && a[i] != b[i] {
			return false
		}
	}
	return true
}

// test if the string 's' is contained in 'l'
func containsString(l []string, s string) bool {
	for i := range l {
		if l[i] == s {
			return true
		}
	}
	return false
}

// test if the list of strings 'a' is contained in 'l'
func containsList(l [][]string, a []string) bool {
	for i := range l {
		if equals(a, l[i]) {
			return true
		}
	}
	return false
}
