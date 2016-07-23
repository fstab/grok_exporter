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
		linesRead := prometheus.NewCounter(prometheus.CounterOpts{
			Name: "grok_exporter_lines_read_total",
			Help: "Number of lines that are read from the logfile, including lines that are not yet processed by the configured metrics.",
		})
		prometheus.MustRegister(linesRead)
		for line := range origTailer.LineChan() {
			bufferSync.L.Lock()
			buffer.PushBack(line)
			bufferSync.Signal()
			bufferSync.L.Unlock()
			linesRead.Inc()
		}
		bufferSync.L.Lock()
		buffer = nil // make the consumer quit
		bufferSync.Signal()
		bufferSync.L.Unlock()
		prometheus.Unregister(linesRead)
		close(out)
	}()

	// consumer
	go func() {
		linesProcessed := prometheus.NewCounter(prometheus.CounterOpts{
			Name: "grok_exporter_lines_processed_total",
			Help: "Number of lines that are handed over to the configured metrics for evaluation.",
		})
		prometheus.MustRegister(linesProcessed)
		for {
			bufferSync.L.Lock()
			for buffer != nil && buffer.Len() == 0 {
				bufferSync.Wait()
			}
			if buffer == nil {
				bufferSync.L.Unlock()
				prometheus.Unregister(linesProcessed)
				return
			}
			first := buffer.Front()
			buffer.Remove(first)
			bufferSync.L.Unlock()
			switch line := first.Value.(type) {
			case string:
				out <- line
				linesProcessed.Inc()
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
