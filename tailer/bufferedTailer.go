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
	"container/list"
	"github.com/fstab/grok_exporter/tailer/fswatcher"
	"log"
	"sync"
	"time"
)

// implements fswatcher.FileTailer
type bufferedTailer struct {
	out  chan fswatcher.Line
	orig fswatcher.FileTailer
}

func (b *bufferedTailer) Lines() chan fswatcher.Line {
	return b.out
}

func (b *bufferedTailer) Errors() chan fswatcher.Error {
	return b.orig.Errors()
}

func (b *bufferedTailer) Close() {
	b.orig.Close()
}

func BufferedTailer(orig fswatcher.FileTailer) fswatcher.FileTailer {
	return BufferedTailerWithMetrics(orig, &noopMetric{})
}

// Wrapper around a tailer that consumes the lines channel quickly.
// The idea is that the original tailer can continue reading lines from the logfile,
// and does not need to wait until the lines are processed.
// The number of buffered lines are exposed as a Prometheus metric, if lines are constantly
// produced faster than they are consumed, we will eventually run out of memory.
//
// ---
// The buffered tailer prevents the following error (this can be reproduced on Windows,
// where we don't keep the logfile open):
//
// Example test actions
// --------------------
//
// Sequence of actions simulated in fileTailer_test:
//
// 1) write line a
// 2) write line b
// 3) move the old logfile away and create a new logfile
// 4) write line c
//
// Good case event processing
// --------------------------
//
// How Events.Process() should process the file system events triggered by the actions above:
//
// 1) MODIFIED : Process() reads line a
// 2) MODIFIED : Process() reads line b
// 3) MOVED_FROM, CREATED : Process() resets the line reader and seeks the file to position 0
// 4) MODIFIED : Process() reads line c
//
// Bad case event processing
// -------------------------
//
// When Events.Process() receives a MODIFIED event, it does not know how many lines have been written.
// Therefore, it reads all new lines until EOF is reached.
// If line processing is slow (writing to the lines channel blocks until all grok patterns are processed),
// we might read 'line b' while we are still processing the first MODIFIED event:
//
// 1) MODIFIED : Process() reads 'line a' and 'line b'
//
// Meanwhile, the test continues with steps 3 and 4, moving the logfile away, creating a new logfile,
// and writing 'line c'. When the tailer receives the second MODIFIED event, it learns that the file
// has been truncated, seeks to position 0, and reads 'line c'.
//
// 2) MODIFIED : Process() detects the truncated file, seeks to position 0, reads 'line c'
//
// The tailer now receives MOVED_FROM, which makes it close the logfile, CREATED, which makes
// it open the logfile and start reading from position 0:
//
// 3) MOVED_FROM, CREATED : seek to position 0, read line c again !!!
//
// When the last MODIFIED event is processed, there are no more changes in the file:
//
// 4) MODIFIED : no changes in file
//
// As a result, we read 'line c' two times.
//
// To minimize the risk, use the buffered tailer to make sure file system events are handled
// as quickly as possible without waiting for the grok patterns to be processed.
func BufferedTailerWithMetrics(orig fswatcher.FileTailer, bufferLoadMetric BufferLoadMetric) fswatcher.FileTailer {
	buffer := list.New()
	bufferSync := sync.NewCond(&sync.Mutex{}) // coordinate producer and consumer
	out := make(chan fswatcher.Line)

	// producer
	go func() {
		bufferLoadMetric.Register()
		bufferLoadPeakValue := 0
		tick := time.NewTicker(1 * time.Second)
		for {
			select {
			case line, ok := <-orig.Lines():
				if ok {
					bufferSync.L.Lock()
					buffer.PushBack(line)
					if buffer.Len() > bufferLoadPeakValue {
						bufferLoadPeakValue = buffer.Len()
					}
					bufferSync.Signal()
					bufferSync.L.Unlock()
				} else {
					bufferSync.L.Lock()
					buffer = nil // make the consumer quit
					bufferSync.Signal()
					bufferSync.L.Unlock()
					bufferLoadMetric.Unregister()
					tick.Stop()
					return
				}
			case <-tick.C:
				bufferLoadMetric.Observe(float64(bufferLoadPeakValue))
				bufferLoadPeakValue = 0
			}
		}
	}()

	// consumer
	go func() {
		for {
			bufferSync.L.Lock()
			for buffer != nil && buffer.Len() == 0 {
				bufferSync.Wait()
			}
			if buffer == nil {
				bufferSync.L.Unlock()
				close(out)
				return
			}
			first := buffer.Front()
			buffer.Remove(first)
			bufferSync.L.Unlock()
			switch line := first.Value.(type) {
			case fswatcher.Line:
				out <- line
			default:
				// this cannot happen
				log.Fatal("unexpected type in tailer buffer")
			}
		}
	}()
	return &bufferedTailer{
		out:  out,
		orig: orig,
	}
}

type BufferLoadMetric interface {
	Register()
	Observe(currentLoad float64)
	Unregister()
}

type noopMetric struct{}

func (m *noopMetric) Register()                   {}
func (m *noopMetric) Observe(currentLoad float64) {}
func (m *noopMetric) Unregister()                 {}
