package perfmonitor

import (
	"github.com/fstab/grok_exporter/exporter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"strconv"
	"time"
)

type Metrics struct {
	buildInfo            *prometheus.GaugeVec
	timeSpent            *prometheus.CounterVec
	linesRead            *prometheus.CounterVec
	linesProcessed       *prometheus.CounterVec
	linesMatched         *prometheus.CounterVec
	lineProcessingErrors *prometheus.CounterVec
	bufferLoadMetric     *bufferLoadMetric
}

func New(registry prometheus.Registerer, log logrus.FieldLogger, lineLimitSet bool) *Metrics {
	result := &Metrics{
		buildInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "grok_exporter_build_info",
			Help: "A metric with a constant '1' value labeled by version, builddate, branch, revision, goversion, and platform on which grok_exporter was built.",
		}, []string{"version", "builddate", "branch", "revision", "goversion", "platform"}),
		timeSpent: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "grok_exporter_time_spent_total",
			Help: "Number of nanoseconds spent in each state by goroutine",
		}, []string{"goroutine", "state", "active", "metric_name"}),
		linesRead: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "grok_exporter_lines_read_total",
			Help: "Number of lines read from the input",
		}, []string{"source"}),
		linesProcessed: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "grok_exporter_lines_processed_total",
			Help: "Number of lines processed",
		}, []string{"match"}),
		linesMatched: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "grok_exporter_lines_matched_total",
			Help: "Number of lines matched for each metric. Note that one line can be matched by multiple Metrics.",
		}, []string{"match"}),
		lineProcessingErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "grok_exporter_line_processing_errors_total",
			Help: "Number of errors for each metric. If this is > 0 there is an error in the configuration file. Check grok_exporter's console output.",
		}, []string{"match"}),
		bufferLoadMetric: NewBufferLoadMetric(log, lineLimitSet, registry),
	}
	registry.MustRegister(result.buildInfo)
	registry.MustRegister(result.timeSpent)
	registry.MustRegister(result.linesRead)
	registry.MustRegister(result.linesProcessed)
	registry.MustRegister(result.linesMatched)
	registry.MustRegister(result.lineProcessingErrors)

	return result
}

func (m *Metrics) SetBuildInfo(version, buildDate, branch, revision, goversion, platform string) {
	m.buildInfo.WithLabelValues(version, buildDate, branch, revision, goversion, platform).Set(1)
}

func (m *Metrics) ObserveLineProcessed(matched bool) {
	m.linesProcessed.WithLabelValues(strconv.FormatBool(matched)).Inc()
}

func (m *Metrics) ObserveLineMatched(matchName string) {
	m.linesMatched.WithLabelValues(matchName).Inc()
}

func (m *Metrics) ObserverLineProcessingError(matchName string) {
	m.lineProcessingErrors.WithLabelValues(matchName).Inc()
}

func (m *Metrics) BufferLoadMetric() *bufferLoadMetric {
	return m.bufferLoadMetric
}

type FileSystemWatcherMonitor interface {
	WaitingForFileSystemEvent()
	ProcessingFileSystemEvent()
}

type LogLineBufferProducerMonitor interface {
	WaitingForLogLine()
	PuttingLogLIneIntoBuffer()
}

type LogLineBufferConsumerMonitor interface {
	WaitingForLogLine()
	ProcessingLogLine()
}

func (m *Metrics) ForFileSystemWatcher() FileSystemWatcherMonitor {
	return &fileSystemWatcherPerfMonitor{
		goroutineLocalEventMonitor{
			goroutine: "file system watcher",
			metric:    m.timeSpent,
		},
	}
}

func (m *Metrics) ForLogLineBufferProducer() LogLineBufferProducerMonitor {
	return &logLineBufferProducerPerfMonitor{
		goroutineLocalEventMonitor{
			goroutine: "log line buffer (producer)",
			metric:    m.timeSpent,
		},
	}
}

func (m *Metrics) ForLogLineBufferConsumer() LogLineBufferConsumerMonitor {
	return &logLineBufferConsumerPerfMonitor{
		goroutineLocalEventMonitor{
			goroutine: "log line buffer (consumer)",
			metric:    m.timeSpent,
		},
	}
}

func (m *Metrics) ForProcessor() *processorPerfMonitor {
	return &processorPerfMonitor{
		goroutineLocalEventMonitor{
			goroutine: "processor",
			metric:    m.timeSpent,
		},
	}
}

func (m *fileSystemWatcherPerfMonitor) WaitingForFileSystemEvent() {
	now := time.Now()
	m.observeState(now)
	m.state.name = "waiting for file system event"
	m.state.active = false
	m.state.start = now
}

func (m *fileSystemWatcherPerfMonitor) ProcessingFileSystemEvent() {
	now := time.Now()
	m.observeState(now)
	m.state.name = "processing file system event"
	m.state.active = true
	m.state.start = now
}

func (m *logLineBufferProducerPerfMonitor) WaitingForLogLine() {
	now := time.Now()
	m.observeState(now)
	m.state.name = "waiting for log line"
	m.state.active = false
	m.state.start = now
}

func (m *logLineBufferProducerPerfMonitor) PuttingLogLIneIntoBuffer() {
	now := time.Now()
	m.observeState(now)
	m.state.name = "putting log line into buffer"
	m.state.active = true
	m.state.start = now
}

func (m *Metrics) InitCounters(metrics []exporter.Metric) {
	m.timeSpent.WithLabelValues("file system watcher", "waiting for file system event", "false", "").Add(0)
	m.timeSpent.WithLabelValues("file system watcher", "processing file system event", "true", "").Add(0)
	m.timeSpent.WithLabelValues("log line buffer (producer)", "waiting for log line", "false", "").Add(0)
	m.timeSpent.WithLabelValues("log line buffer (producer)", "putting log line into buffer", "true", "").Add(0)
	m.timeSpent.WithLabelValues("log line buffer (consumer)", "waiting for log line", "true", "").Add(0)
	m.timeSpent.WithLabelValues("log line buffer (consumer)", "processing log line", "true", "").Add(0)
	m.timeSpent.WithLabelValues("processor", "waiting for log line", "false", "").Add(0)
	for _, metric := range metrics {
		m.timeSpent.WithLabelValues("processor", "processing match", "true", metric.Name()).Add(0)
		if metric.HasDeleteMatch() {
			m.timeSpent.WithLabelValues("processor", "processing delete match", "true", metric.Name()).Add(0)
		}
		m.linesMatched.WithLabelValues(metric.Name()).Add(0)
		m.lineProcessingErrors.WithLabelValues(metric.Name()).Add(0)
	}
	m.linesProcessed.WithLabelValues("true").Add(0)
	m.linesProcessed.WithLabelValues("false").Add(0)
}

func (m *logLineBufferConsumerPerfMonitor) WaitingForLogLine() {
	now := time.Now()
	m.observeState(now)
	m.state.name = "waiting for log line"
	m.state.active = false
	m.state.start = now
}

func (m *logLineBufferConsumerPerfMonitor) ProcessingLogLine() {
	now := time.Now()
	m.observeState(now)
	m.state.name = "processing log line"
	m.state.active = true
	m.state.start = now
}

func (m *processorPerfMonitor) WaitingForLogLine() {
	now := time.Now()
	m.observeState(now)
	m.state.name = "waiting for log line"
	m.state.active = false
	m.state.start = now
}

func (m *processorPerfMonitor) ProcessingMatch(name string) {
	now := time.Now()
	m.observeState(now)
	m.state.name = "processing match"
	m.state.active = true
	m.state.start = now
	m.state.matchName = name
}

func (m *processorPerfMonitor) ProcessingDeleteMatch(name string) {
	now := time.Now()
	m.observeState(now)
	m.state.name = "processing delete match"
	m.state.active = true
	m.state.start = now
	m.state.matchName = name
}

type goroutineLocalEventMonitor struct {
	goroutine string
	state     state
	metric    *prometheus.CounterVec
}

type fileSystemWatcherPerfMonitor struct {
	goroutineLocalEventMonitor
}

type logLineBufferProducerPerfMonitor struct {
	goroutineLocalEventMonitor
}

type logLineBufferConsumerPerfMonitor struct {
	goroutineLocalEventMonitor
}

type processorPerfMonitor struct {
	goroutineLocalEventMonitor
}

func (m goroutineLocalEventMonitor) observeState(end time.Time) {
	if len(m.state.name) > 0 {
		m.metric.WithLabelValues(m.goroutine, m.state.name, strconv.FormatBool(m.state.active), m.state.matchName).Add(end.Sub(m.state.start).Seconds())
	}
}

type state struct {
	name      string
	active    bool
	matchName string
	start     time.Time
}
