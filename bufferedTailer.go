package main

import (
	"container/list"
	"github.com/fstab/grok_exporter/tailer"
	"github.com/prometheus/client_golang/prometheus"
	"log"
	"sync"
)

type bufferedTailerWithMetrics struct {
	out        chan string
	origTailer tailer.Tailer
}

func (b *bufferedTailerWithMetrics) LineChan() chan string {
	return b.out
}

func (b *bufferedTailerWithMetrics) ErrorChan() chan error {
	return b.origTailer.ErrorChan()
}

func (b *bufferedTailerWithMetrics) Close() {
	b.origTailer.Close()
}

func BufferedTailerWithMetrics(origTailer tailer.Tailer) tailer.Tailer {
	buffer := list.New()
	bufferSync := sync.NewCond(&sync.Mutex{}) // coordinate producer and consumer
	out := make(chan string)

	// producer
	go func() {
		for line := range origTailer.LineChan() {
			bufferSync.L.Lock()
			buffer.PushBack(line)
			bufferSync.Signal()
			bufferSync.L.Unlock()
		}
		bufferSync.L.Lock()
		buffer = nil // make the consumer quit
		bufferSync.Signal()
		bufferSync.L.Unlock()
		close(out)
	}()

	// consumer
	go func() {
		bufferSizeMetric := prometheus.NewSummary(prometheus.SummaryOpts{
			Name: "grok_exporter_line_buffer_load",
			Help: "Number of log lines that are read from the logfile but not yet processed by grok_exporter.",
		})
		prometheus.MustRegister(bufferSizeMetric)
		for {
			bufferSync.L.Lock()
			for buffer != nil && buffer.Len() == 0 {
				bufferSync.Wait()
			}
			if buffer == nil {
				bufferSync.L.Unlock()
				prometheus.Unregister(bufferSizeMetric)
				return
			}
			first := buffer.Front()
			buffer.Remove(first)
			bufferSizeMetric.Observe(float64(buffer.Len())) // inside lock, because we use buffer.Len()
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
		out:        out,
		origTailer: origTailer,
	}
}
