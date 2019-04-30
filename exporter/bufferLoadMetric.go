package exporter

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

var (
	minLabel = prometheus.Labels{
		"value":    "min",
		"interval": "1m",
	}
	maxLabel = prometheus.Labels{
		"value":    "max",
		"interval": "1m",
	}
)

// We measure the minimum and maximum buffer load in the last minute.
// The one minute window is a tumbling window being pushed forward every 15 seconds.
type bufferLoadMetric struct {
	cur                            int64
	min15s, min30s, min45s, min60s int64
	max15s, max30s, max45s, max60s int64
	bufferLoad                     *prometheus.GaugeVec
	mutex                          *sync.Cond
	tick                           *time.Ticker
	log                            logrus.FieldLogger
	lineLimitSet                   bool
}

func NewBufferLoadMetric(log logrus.FieldLogger, lineLimitSet bool) *bufferLoadMetric {
	m := &bufferLoadMetric{
		mutex:        sync.NewCond(&sync.Mutex{}),
		log:          log,
		lineLimitSet: lineLimitSet,
	}
	return m
}

func (m *bufferLoadMetric) Start() {
	m.start(time.NewTicker(15*time.Second), nil)
}

// Ticker should tick every 15 seconds, except for the test where we speed things up for testing.
// The tickProcessed channel is just for testing, it signals to the test when a tick was processed.
func (m *bufferLoadMetric) start(ticker *time.Ticker, tickProcessed chan struct{}) {
	m.tick = ticker
	m.bufferLoad = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "grok_exporter_line_buffer_load",
		Help: "Number of lines that are read from the logfile and waiting to be processed.",
	}, []string{"value", "interval"})
	prometheus.MustRegister(m.bufferLoad)
	m.bufferLoad.With(minLabel).Set(0)
	m.bufferLoad.With(maxLabel).Set(0)
	go func() {
		var ticksSinceLastLog = 0
		for range m.tick.C {
			func() {
				m.mutex.L.Lock()
				defer m.mutex.L.Unlock()

				ticksSinceLastLog++
				if ticksSinceLastLog >= 4 { // every minute
					if m.min60s > 1000 && !m.lineLimitSet {
						m.log.Warnf("Log lines are written faster than grok_exporter processes them. In the last minute there were constantly more than %d log lines in the buffer waiting to be processed. Check the built-in grok_exporter_lines_processing_time_microseconds_total metric to learn which metric takes most of the processing time.", m.min60s)
					}
					ticksSinceLastLog = 0
				}

				m.bufferLoad.With(minLabel).Set(float64(m.min60s))

				m.min60s = m.min45s
				m.min45s = m.min30s
				m.min30s = m.min15s
				m.min15s = m.cur

				m.bufferLoad.With(maxLabel).Set(float64(m.max60s))

				m.max60s = m.max45s
				m.max45s = m.max30s
				m.max30s = m.max15s
				m.max15s = m.cur

			}()

			if tickProcessed != nil {
				tickProcessed <- struct{}{}
			}
		}
	}()
}

func (m *bufferLoadMetric) Stop() {
	m.tick.Stop()
	prometheus.Unregister(m.bufferLoad)
}

func (m *bufferLoadMetric) Inc() {
	m.mutex.L.Lock()
	defer m.mutex.L.Unlock()
	m.cur++
	m.updateMax()
}

func (m *bufferLoadMetric) Dec() {
	m.mutex.L.Lock()
	defer m.mutex.L.Unlock()
	m.cur--
	m.updateMin()
}

func (m *bufferLoadMetric) updateMin() {
	if m.min15s > m.cur {
		m.min15s = m.cur
	}
	if m.min30s > m.cur {
		m.min30s = m.cur
	}
	if m.min45s > m.cur {
		m.min45s = m.cur
	}
	if m.min60s > m.cur {
		m.min60s = m.cur
	}
}

func (m *bufferLoadMetric) updateMax() {
	if m.max15s < m.cur {
		m.max15s = m.cur
	}
	if m.max30s < m.cur {
		m.max30s = m.cur
	}
	if m.max45s < m.cur {
		m.max45s = m.cur
	}
	if m.max60s < m.cur {
		m.max60s = m.cur
	}
}

func (m *bufferLoadMetric) Set(value int64) {
	m.mutex.L.Lock()
	defer m.mutex.L.Unlock()
	m.cur = value
	m.updateMin()
	m.updateMax()
}
