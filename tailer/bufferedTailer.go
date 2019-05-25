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
	"github.com/fstab/grok_exporter/tailer/fswatcher"
	"github.com/sirupsen/logrus"
)

// implements fswatcher.FileTailer
type bufferedTailer struct {
	out  chan *fswatcher.Line
	orig fswatcher.FileTailer
	done chan struct{}
}

func (b *bufferedTailer) Lines() chan *fswatcher.Line {
	return b.out
}

func (b *bufferedTailer) Errors() chan fswatcher.Error {
	return b.orig.Errors()
}

func (b *bufferedTailer) Close() {
	b.orig.Close()
	close(b.done)
}

func BufferedTailer(orig fswatcher.FileTailer) fswatcher.FileTailer {
	return BufferedTailerWithMetrics(orig, &noopMetric{}, logrus.New(), 0)
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
func BufferedTailerWithMetrics(orig fswatcher.FileTailer, bufferLoadMetric BufferLoadMetric, log logrus.FieldLogger, maxLinesInBuffer int) fswatcher.FileTailer {
	buffer := NewLineBuffer()
	out := make(chan *fswatcher.Line)
	done := make(chan struct{})

	// producer
	go func() {
		bufferLoadMetric.Start()
		for {
			line, ok := <-orig.Lines()
			if ok {
				if maxLinesInBuffer > 0 && buffer.Len() > maxLinesInBuffer-1 {
					log.Warnf("Line buffer reached limit of %v lines. Dropping lines in buffer.", maxLinesInBuffer)
					buffer.Clear()
					bufferLoadMetric.Set(0)
				}
				buffer.Push(line)
				bufferLoadMetric.Inc()
			} else {
				buffer.Close()
				bufferLoadMetric.Stop()
				return
			}
		}
	}()

	// consumer
	go func() {
		for {
			line := buffer.BlockingPop()
			if line == nil {
				// buffer closed
				close(out)
				return
			}
			bufferLoadMetric.Dec()
			select {
			case out <- line:
			case <-done:
			}
		}
	}()
	return &bufferedTailer{
		out:  out,
		orig: orig,
		done: done,
	}
}

type BufferLoadMetric interface {
	Start()
	Inc()            // put a log line into the buffer
	Dec()            // take a log line from the buffer
	Set(value int64) // set the current number of lines in the buffer
	Stop()
}

type noopMetric struct{}

func (m *noopMetric) Start()          {}
func (m *noopMetric) Inc()            {}
func (m *noopMetric) Dec()            {}
func (m *noopMetric) Set(value int64) {}
func (m *noopMetric) Stop()           {}
