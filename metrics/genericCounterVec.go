package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/fstab/grok_prometheus_exporter/config"
	"github.com/moovweb/rubex"
	"fmt"
)

type genericCounterVecMetric struct {
	name string
	labels []config.Label
	regex *rubex.Regexp
	counter *prometheus.CounterVec
}

func CreateGenericCounterVecMetric(cfg *config.MetricConfig, regex *rubex.Regexp) Metric {
	prometheusLabels := make([]string, 0, len(cfg.Labels))
	for _, label := range cfg.Labels {
		prometheusLabels = append(prometheusLabels, label.PrometheusLabel)
	}
	return &genericCounterVecMetric{
		name: cfg.Name,
		labels: cfg.Labels,
		regex: regex,
		counter: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: cfg.Name,
			Help: cfg.Help,
		}, prometheusLabels),
	}
}

func (m *genericCounterVecMetric) Collector() prometheus.Collector {
	return m.counter
}

func (m *genericCounterVecMetric) Matches(line string) bool {
	return m.regex.MatchString(line)
}

func (m *genericCounterVecMetric) Name() string {
	return m.name
}

func (m *genericCounterVecMetric) Process(line string) {
	values := make([]string, 0, len(m.labels))
	for _, field := range m.labels {
		value := m.regex.Gsub(line, fmt.Sprintf("\\k<%v>", field.GrokFieldName))
		values = append(values, value)
	}
	m.counter.WithLabelValues(values...).Inc()
}
