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

package exporter

import (
	"container/list"
	"github.com/fstab/grok_exporter/tailer"
	"github.com/prometheus/client_golang/prometheus"
	"log"
	"sync"
	"time"
)

// implements tailer.Tailer
type bufferedTailerWithMetrics struct {
	out  chan string
	orig tailer.Tailer
}

func (b *bufferedTailerWithMetrics) Lines() chan string {
	return b.out
}

func (b *bufferedTailerWithMetrics) Errors() chan tailer.Error {
	return b.orig.Errors()
}

func (b *bufferedTailerWithMetrics) Close() {
	b.orig.Close()
}

// Wrapper around a tailer that consumes the lines channel quickly.
// The idea is that the original tailer can continue reading lines from the logfile,
// and does not need to wait until the lines are processed.
// The number of buffered lines are exposed as a Prometheus metric, if lines are constantly
// produced faster than they are consumed, we will eventually run out of memory.
func BufferedTailerWithMetrics(orig tailer.Tailer) tailer.Tailer {
	buffer := list.New()
	bufferSync := sync.NewCond(&sync.Mutex{}) // coordinate producer and consumer
	out := make(chan string)

	// producer
	go func() {
		bufferLoad := prometheus.NewSummary(prometheus.SummaryOpts{
			Name: "grok_exporter_line_buffer_peak_load",
			Help: "Number of lines that are read from the logfile and waiting to be processed. Peak value per second.",
		})
		prometheus.MustRegister(bufferLoad)
		bufferLoadPeakValue := 0
		tick := time.NewTicker(1 * time.Second)
		for {
			select {
			case line, ok := <-orig.Lines():
				if ok {
					bufferSync.L.Lock()
					if buffer.Len() > bufferLoadPeakValue {
						bufferLoadPeakValue = buffer.Len()
					}
					buffer.PushBack(line)
					bufferSync.Signal()
					bufferSync.L.Unlock()
				} else {
					bufferSync.L.Lock()
					buffer = nil // make the consumer quit
					bufferSync.Signal()
					bufferSync.L.Unlock()
					prometheus.Unregister(bufferLoad)
					tick.Stop()
					return
				}
			case <-tick.C:
				bufferLoad.Observe(float64(bufferLoadPeakValue))
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
			case string:
				out <- line
			default:
				// this cannot happen
				log.Fatal("unexpected type in tailer buffer")
			}
		}
	}()
	return &bufferedTailerWithMetrics{
		out:  out,
		orig: orig,
	}
}
