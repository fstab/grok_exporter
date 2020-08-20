package selfmonitoring

import (
	"github.com/fstab/grok_exporter/exporter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"strconv"
	"time"
)

type SelfMonitoring interface {
	SetBuildInfo(version, buildDate, branch, revision, goversion, platform string)
	ForProcessor() ProcessorMonitor
	ForFileSystemWatcher() FileSystemWatcherMonitor
	BufferLoadMetric() *bufferLoadMetric
	ForLogLineBufferProducer() LogLineBufferProducerMonitor
	ForLogLineBufferConsumer() LogLineBufferConsumerMonitor
	ObserverLineProcessingError(name string)
	ObserveLineMatched(name string)
	ObserveLineProcessed(matched bool)
}

type goroutineName string
type stateName string

const (
	fileSystemWatcher     goroutineName = "file system watcher"
	logLineBufferProducer goroutineName = "log line buffer (producer)"
	logLineBufferConsumer goroutineName = "log line buffer (consumer)"
	processor             goroutineName = "processor"
)

const (
	waitingForFileSystemEvent stateName = "waiting for file system event"
	processingFileSystemEvent stateName = "processing file system event"
	waitingForLogLine         stateName = "waiting for log line"
	puttingLogLineIntoBuffer  stateName = "putting log line into buffer"
	processingLogLine         stateName = "processing log line"
	processingMatch           stateName = "processing match"
	processingDeleteMatch     stateName = "processing delete match"
)

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

type ProcessorMonitor interface {
	WaitingForLogLine()
	ProcessingMatch(name string)
	ProcessingDeleteMatch(name string)
}

type state struct {
	goroutine goroutineName
	name      stateName
	active    bool
	matchName string
}

func (m *fileSystemWatcherMonitor) WaitingForFileSystemEvent() {
	m.stateChanges <- &state{
		name:      waitingForFileSystemEvent,
		goroutine: fileSystemWatcher,
		active:    false,
	}
}

func (m *fileSystemWatcherMonitor) ProcessingFileSystemEvent() {
	m.stateChanges <- &state{
		name:      processingFileSystemEvent,
		goroutine: fileSystemWatcher,
		active:    false,
	}
}

func (m *logLineBufferProducerMonitor) WaitingForLogLine() {
	m.stateChanges <- &state{
		name:      waitingForLogLine,
		goroutine: logLineBufferProducer,
		active:    false,
	}
}

func (m *logLineBufferProducerMonitor) PuttingLogLIneIntoBuffer() {
	m.stateChanges <- &state{
		name:      puttingLogLineIntoBuffer,
		goroutine: logLineBufferProducer,
		active:    true,
	}
}

func (m *logLineBufferConsumerMonitor) WaitingForLogLine() {
	m.stateChanges <- &state{
		name:      waitingForLogLine,
		goroutine: logLineBufferConsumer,
		active:    false,
	}
}

func (m *logLineBufferConsumerMonitor) ProcessingLogLine() {
	m.stateChanges <- &state{
		name:      processingLogLine,
		goroutine: logLineBufferConsumer,
		active:    true,
	}
}

func (m *processorMonitor) WaitingForLogLine() {
	m.stateChanges <- &state{
		name:      waitingForLogLine,
		goroutine: processor,
		active:    false,
	}
}

func (m *processorMonitor) ProcessingMatch(name string) {
	m.stateChanges <- &state{
		name:      processingMatch,
		goroutine: processor,
		matchName: name,
		active:    true,
	}
}

func (m *processorMonitor) ProcessingDeleteMatch(name string) {
	m.stateChanges <- &state{
		name:      processingDeleteMatch,
		goroutine: processor,
		matchName: name,
		active:    true,
	}
}

func (m *selfMonitoring) ForFileSystemWatcher() FileSystemWatcherMonitor {
	return &fileSystemWatcherMonitor{
		stateChanges: m.stateChanges,
	}
}

func (m *selfMonitoring) ForLogLineBufferProducer() LogLineBufferProducerMonitor {
	return &logLineBufferProducerMonitor{
		stateChanges: m.stateChanges,
	}
}

func (m *selfMonitoring) ForLogLineBufferConsumer() LogLineBufferConsumerMonitor {
	return &logLineBufferConsumerMonitor{
		stateChanges: m.stateChanges,
	}
}

func (m *selfMonitoring) ForProcessor() ProcessorMonitor {
	return &processorMonitor{
		stateChanges: m.stateChanges,
	}
}

type fileSystemWatcherMonitor struct {
	stateChanges chan *state
}

type logLineBufferProducerMonitor struct {
	stateChanges chan *state
}

type logLineBufferConsumerMonitor struct {
	stateChanges chan *state
}

type processorMonitor struct {
	stateChanges chan *state
}

type FileSystemWatcherState chan state

func (m *selfMonitoring) SetBuildInfo(version, buildDate, branch, revision, goversion, platform string) {
	m.buildInfo.WithLabelValues(version, buildDate, branch, revision, goversion, platform).Set(1)
}

func (m *selfMonitoring) ObserveLineProcessed(matched bool) {
	m.linesProcessed.WithLabelValues(strconv.FormatBool(matched)).Inc()
}

func (m *selfMonitoring) ObserveLineMatched(matchName string) {
	m.linesMatched.WithLabelValues(matchName).Inc()
}

func (m *selfMonitoring) ObserverLineProcessingError(matchName string) {
	m.lineProcessingErrors.WithLabelValues(matchName).Inc()
}

func (m *selfMonitoring) BufferLoadMetric() *bufferLoadMetric {
	return m.bufferLoadMetric
}

type selfMonitoring struct {
	buildInfo            *prometheus.GaugeVec
	timeSpent            *prometheus.CounterVec
	linesRead            *prometheus.CounterVec
	linesProcessed       *prometheus.CounterVec
	linesMatched         *prometheus.CounterVec
	lineProcessingErrors *prometheus.CounterVec
	bufferLoadMetric     *bufferLoadMetric
	stateChanges         chan *state
	done                 chan struct{}
	lastStateChangeEvent map[goroutineName]*stateChangeEvent
}

type stateChangeEvent struct {
	name      stateName
	active    bool
	matchName string
	start     time.Time
}

func Start(registry prometheus.Registerer, metrics []exporter.Metric, lineLimitSet bool, log logrus.FieldLogger) SelfMonitoring {
	result := &selfMonitoring{
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
			Help: "Number of lines matched for each metric. Note that one line can be matched by multiple selfMonitoring.",
		}, []string{"match"}),
		lineProcessingErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "grok_exporter_line_processing_errors_total",
			Help: "Number of errors for each metric. If this is > 0 there is an error in the configuration file. Check grok_exporter's console output.",
		}, []string{"match"}),
		bufferLoadMetric:     NewBufferLoadMetric(log, lineLimitSet, registry),
		stateChanges:         make(chan *state),
		done:                 make(chan struct{}),
		lastStateChangeEvent: make(map[goroutineName]*stateChangeEvent),
	}
	registry.MustRegister(result.buildInfo)
	registry.MustRegister(result.timeSpent)
	registry.MustRegister(result.linesRead)
	registry.MustRegister(result.linesProcessed)
	registry.MustRegister(result.linesMatched)
	registry.MustRegister(result.lineProcessingErrors)

	result.timeSpent.WithLabelValues("file system watcher", "waiting for file system event", "false", "").Add(0)
	result.timeSpent.WithLabelValues("file system watcher", "processing file system event", "true", "").Add(0)
	result.timeSpent.WithLabelValues("log line buffer (producer)", "waiting for log line", "false", "").Add(0)
	result.timeSpent.WithLabelValues("log line buffer (producer)", "putting log line into buffer", "true", "").Add(0)
	result.timeSpent.WithLabelValues("log line buffer (consumer)", "waiting for log line", "true", "").Add(0)
	result.timeSpent.WithLabelValues("log line buffer (consumer)", "processing log line", "true", "").Add(0)
	result.timeSpent.WithLabelValues("processor", "waiting for log line", "false", "").Add(0)
	for _, metric := range metrics {
		result.timeSpent.WithLabelValues("processor", "processing match", "true", metric.Name()).Add(0)
		if metric.HasDeleteMatch() {
			result.timeSpent.WithLabelValues("processor", "processing delete match", "true", metric.Name()).Add(0)
		}
		result.linesMatched.WithLabelValues(metric.Name()).Add(0)
		result.lineProcessingErrors.WithLabelValues(metric.Name()).Add(0)
	}
	result.linesProcessed.WithLabelValues("true").Add(0)
	result.linesProcessed.WithLabelValues("false").Add(0)

	result.lastStateChangeEvent[fileSystemWatcher] = &stateChangeEvent{
		name:   waitingForFileSystemEvent,
		active: false,
		start:  time.Now(),
	}
	result.lastStateChangeEvent[logLineBufferProducer] = &stateChangeEvent{
		name:   waitingForLogLine,
		active: false,
		start:  time.Now(),
	}
	result.lastStateChangeEvent[logLineBufferConsumer] = &stateChangeEvent{
		name:   waitingForLogLine,
		active: false,
		start:  time.Now(),
	}
	result.lastStateChangeEvent[processor] = &stateChangeEvent{
		name:   waitingForLogLine,
		active: false,
		start:  time.Now(),
	}

	go func() {
		tick := time.After(1 * time.Second)
		for {
			select {
			case <-result.done:
				return
			case <-tick:
				result.tick()
				tick = time.After(1 * time.Second)
			case newState := <-result.stateChanges:
				result.stateChange(newState)
			}
		}
	}()
	return result
}

func (m *selfMonitoring) Stop() {
	close(m.done)
}

func (m *selfMonitoring) stateChange(newState *state) {
	now := time.Now()
	goroutine := newState.goroutine
	prevState, exists := m.lastStateChangeEvent[goroutine]
	if exists {
		m.timeSpent.WithLabelValues(string(goroutine), string(prevState.name), strconv.FormatBool(prevState.active), prevState.matchName).Add(now.Sub(prevState.start).Seconds())
	}
	m.lastStateChangeEvent[goroutine] = &stateChangeEvent{
		name:      newState.name,
		active:    newState.active,
		matchName: newState.matchName,
		start:     now,
	}
}

func (m *selfMonitoring) tick() {
	for goroutine, cur := range m.lastStateChangeEvent {
		m.stateChange(&state{
			name:      cur.name,
			goroutine: goroutine,
			matchName: cur.matchName,
			active:    cur.active,
		})
	}
}
