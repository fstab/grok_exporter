package exporter

import (
	"fmt"
	"github.com/moovweb/rubex"
	"github.com/prometheus/client_golang/prometheus"
)

type Metric interface {
	Name() string
	Collector() prometheus.Collector
	Process(line string)
}

type counterMetric struct {
	name    string
	regex   *rubex.Regexp
	counter prometheus.Counter
}

type counterVecMetric struct {
	name       string
	labels     []Label
	regex      *rubex.Regexp
	counterVec *prometheus.CounterVec
}

func NewCounterMetric(cfg *MetricConfig, regex *rubex.Regexp) Metric {
	if len(cfg.Labels) == 0 { // regular counter
		return &counterMetric{
			name:  cfg.Name,
			regex: regex,
			counter: prometheus.NewCounter(prometheus.CounterOpts{
				Name: cfg.Name,
				Help: cfg.Help,
			}),
		}
	} else { // counterVec
		prometheusLabels := make([]string, 0, len(cfg.Labels))
		for _, label := range cfg.Labels {
			prometheusLabels = append(prometheusLabels, label.PrometheusLabel)
		}
		return &counterVecMetric{
			name:   cfg.Name,
			labels: cfg.Labels,
			regex:  regex,
			counterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
				Name: cfg.Name,
				Help: cfg.Help,
			}, prometheusLabels),
		}
	}
}

func (m *counterMetric) Collector() prometheus.Collector {
	return m.counter
}

func (m *counterMetric) matches(line string) bool {
	return m.regex.MatchString(line)
}

func (m *counterMetric) Name() string {
	return m.name
}

func (m *counterMetric) Process(line string) {
	if m.matches(line) {
		m.counter.Inc()
	}
}

func (m *counterVecMetric) Collector() prometheus.Collector {
	return m.counterVec
}

func (m *counterVecMetric) matches(line string) bool {
	return m.regex.MatchString(line)
}

func (m *counterVecMetric) Name() string {
	return m.name
}

func (m *counterVecMetric) Process(line string) {
	if m.matches(line) {
		values := make([]string, 0, len(m.labels))
		for _, field := range m.labels {
			value := m.regex.Gsub(line, fmt.Sprintf("\\k<%v>", field.GrokFieldName))
			values = append(values, value)
		}
		m.counterVec.WithLabelValues(values...).Inc()
	}
}
