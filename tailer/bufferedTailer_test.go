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

package tailer

import (
	"fmt"
	"github.com/fstab/grok_exporter/tailer/fswatcher"
	"github.com/sirupsen/logrus"
	"math/rand"
	"sync"
	"testing"
	"time"
)

const nTestLines = 10000

var log = logrus.New()

type sourceTailer struct {
	lines chan *fswatcher.Line
}

func (tail *sourceTailer) Lines() chan *fswatcher.Line {
	return tail.lines
}

func (tail *sourceTailer) Errors() chan fswatcher.Error {
	return nil
}

func (tail *sourceTailer) Close() {
	close(tail.lines)
}

// TODO: As we separated lineBuffer and the metrics, this test is now partially copy-and-paste from lineBuffer_test
func TestLineBufferSequential_withMetrics(t *testing.T) {
	src := &sourceTailer{lines: make(chan *fswatcher.Line)}
	metric := &peakLoadMetric{}
	buffered := BufferedTailerWithMetrics(src, metric, log, 0)
	for i := 1; i <= nTestLines; i++ {
		src.lines <- &fswatcher.Line{Line: fmt.Sprintf("This is line number %v.", i)}
	}
	for i := 1; i <= nTestLines; i++ {
		line := <-buffered.Lines()
		if line.Line != fmt.Sprintf("This is line number %v.", i) {
			t.Errorf("Expected 'This is line number %v', but got '%v'.", i, line)
		}
	}
	buffered.Close()
	_, stillOpen := <-buffered.Lines()
	if stillOpen {
		t.Error("Buffered tailer was not closed.")
	}
	_, stillOpen = <-src.Lines()
	if stillOpen {
		t.Error("Source tailer was not closed.")
	}
	if !metric.startCalled {
		t.Error("metric.Start() not called.")
	}
	if !metric.stopCalled {
		t.Error("metric.Stop() not called.")
	}
	// The peak load should be 1 or two less than nTestLines, depending on how quick
	// the consumer loop started reading
	fmt.Printf("peak load (should be 1 or 2 less than %v): %v\n", nTestLines, metric.peakLoad)
}

// TODO: As we separated lineBuffer and the metrics, this test is now partially copy-and-paste from lineBuffer_test
func TestLineBufferParallel_withMetrics(t *testing.T) {
	src := &sourceTailer{lines: make(chan *fswatcher.Line)}
	metric := &peakLoadMetric{}
	buffered := BufferedTailerWithMetrics(src, metric, log, 0)
	var wg sync.WaitGroup
	go func() {
		start := time.Now()
		for i := 1; i <= nTestLines; i++ {
			src.lines <- &fswatcher.Line{Line: fmt.Sprintf("This is line number %v.", i)}
			if rand.Int()%64 == 0 { // Sleep from time to time
				time.Sleep(10 * time.Millisecond)
			}
		}
		fmt.Printf("Producer took %v.\n", time.Since(start))
		wg.Done()
	}()
	go func() {
		start := time.Now()
		for i := 1; i <= nTestLines; i++ {
			line := <-buffered.Lines()
			if line.Line != fmt.Sprintf("This is line number %v.", i) {
				t.Errorf("Expected 'This is line number %v', but got '%v'.", i, line)
			}
			if rand.Int()%64 == 0 { // Sleep from time to time
				time.Sleep(10 * time.Millisecond)
			}
		}
		fmt.Printf("Consumer took %v.\n", time.Since(start))
		wg.Done()
	}()
	wg.Add(2)
	wg.Wait()
	// wait until peak load is observed (buffered tailer observes the max of each 1 Sec interval)
	time.Sleep(1100 * time.Millisecond)
	buffered.Close()
	_, stillOpen := <-buffered.Lines()
	if stillOpen {
		t.Error("Buffered tailer was not closed.")
	}
	_, stillOpen = <-src.Lines()
	if stillOpen {
		t.Error("Source tailer was not closed.")
	}
	if !metric.startCalled {
		t.Error("metric.Register() not called.")
	}
	if !metric.stopCalled {
		t.Error("metric.Unregister() not called.")
	}
	// Should be much less than nTestLines, because consumer and producer work in parallel.
	fmt.Printf("peak load (should be an order of magnitude less than %v): %v\n", nTestLines, metric.peakLoad)
}

type peakLoadMetric struct {
	startCalled, stopCalled bool
	peakLoad                int64
	currentLoad             int64
}

func (m *peakLoadMetric) Start() {
	m.startCalled = true
}

func (m *peakLoadMetric) Inc() {
	m.currentLoad++
	if m.peakLoad < m.currentLoad {
		m.peakLoad = m.currentLoad
	}
}

func (m *peakLoadMetric) Dec() {
	m.currentLoad--
}

func (m *peakLoadMetric) Set(value int64) {
	m.currentLoad = value
}

func (m *peakLoadMetric) Stop() {
	m.stopCalled = true
}
