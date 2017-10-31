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
	"testing"
	"time"
)

func TestDeleteByLabels(t *testing.T) {
	tracker := NewLabelValueTracker([]string{"service", "user", "hostname", "country"})
	for _, labels := range []map[string]string{
		{
			"service":  "service a",
			"user":     "alice",
			"hostname": "localhost",
			"country":  "Finland",
		},
		{
			"service":  "service a",
			"user":     "alice",
			"hostname": "localhost",
			"country":  "Norway",
		},
		{
			"service":  "service b",
			"user":     "bob",
			"hostname": "remotehost",
			"country":  "Sweden",
		},
		{
			"service":  "service b",
			"user":     "bob",
			"hostname": "remotehost",
			"country":  "Denmark",
		},
		{
			"service":  "service a",
			"user":     "alice",
			"hostname": "remotehost",
			"country":  "Iceland",
		},
	} {
		tracker.Observe(labels)
	}
	empty, err := tracker.DeleteByLabels(map[string]string{ // does not exist, should delete nothing
		"service": "service a",
		"user":    "bob",
	})
	verify(t, empty, 0, tracker, 5, err)
	tracker.Observe(map[string]string{ // already seen, should change nothing
		"service":  "service b",
		"user":     "bob",
		"hostname": "remotehost",
		"country":  "Denmark",
	})
	if nEntries(t, tracker) != 5 {
		t.Fatalf("expected 5 entries, but got %v", nEntries(t, tracker))
	}
	deleted, err := tracker.DeleteByLabels(map[string]string{ // should delete 3 entries
		"service": "service a",
		"user":    "alice",
	})
	verify(t, deleted, 3, tracker, 2, err)
	deleted, err = tracker.DeleteByLabels(map[string]string{}) // wildcard -> delete all
	verify(t, deleted, 2, tracker, 0, err)
	deleted, err = tracker.DeleteByLabels(map[string]string{ // as the tracker is empty, this should do nothing
		"service": "service a",
		"user":    "alice",
	})
	verify(t, deleted, 0, tracker, 0, err)
}

func TestDeleteByRetention(t *testing.T) {
	tracker := NewLabelValueTracker([]string{"service", "user", "hostname", "country"})
	for _, labels := range []map[string]string{
		{
			"service":  "service a",
			"user":     "alice",
			"hostname": "localhost",
			"country":  "Finland",
		},
		{
			"service":  "service a",
			"user":     "alice",
			"hostname": "localhost",
			"country":  "Norway",
		},
		{
			"service":  "service a",
			"user":     "alice",
			"hostname": "localhost",
			"country":  "Sweden",
		},
	} {
		tracker.Observe(labels)
	}
	time.Sleep(500 * time.Millisecond)
	tracker.Observe(map[string]string{ // already known, should update the timestamp but not create a new entry
		"service":  "service a",
		"user":     "alice",
		"hostname": "localhost",
		"country":  "Norway",
	})
	verify(t, nil, 0, tracker, 3, nil)
	deleted := tracker.DeleteByRetention(250 * time.Millisecond) // remove all but the updated entry
	verify(t, deleted, 2, tracker, 1, nil)
	deleted = tracker.DeleteByRetention(250 * time.Millisecond) // should do nothing, because the remaining entry is newer
	verify(t, deleted, 0, tracker, 1, nil)
}

func verify(t *testing.T, deleted []map[string]string, nDeleted int, tracker LabelValueTracker, nRemaining int, err error) {
	if err != nil {
		t.Fatal("unexpected error", err)
	}
	if len(deleted) != nDeleted {
		t.Fatalf("expected %v deleted entries, but got %v", nDeleted, len(deleted))
	}
	if nEntries(t, tracker) != nRemaining {
		t.Fatalf("expected %v remaining entries, but got %v", nRemaining, nEntries(t, tracker))
	}
}

func nEntries(t *testing.T, tracker LabelValueTracker) int {
	trackerInternal, ok := tracker.(*observedLabels)
	if !ok {
		t.Fatal("Cannot cast tracker to *observedLabelValues")
		return 0
	} else {
		return len(trackerInternal.values)
	}
}
