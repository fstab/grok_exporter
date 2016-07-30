package exporter

import (
	"fmt"
	"github.com/moovweb/rubex"
	"github.com/prometheus/client_golang/prometheus"
	"strconv"
)

type Metric interface {
	Name() string
	Collector() prometheus.Collector

	// Returns true if the line matched, and false if the line didn't match.
	Process(line string) (error, bool)
}

// Metric for the number of matching log lines.
type counterMetric struct {
	name      string
	regex     *rubex.Regexp
	labels    []Label
	collector prometheus.Collector
	incFunc   func(line string)
}

// Metric for a value that is parsed from the matching log lines.
type valueMetric struct {
	name        string
	regex       *rubex.Regexp
	value       string
	labels      []Label
	collector   prometheus.Collector
	observeFunc func(line string, val float64)
}

func NewCounterMetric(cfg *MetricConfig, regex *rubex.Regexp) Metric {
	counterOpts := prometheus.CounterOpts{
		Name: cfg.Name,
		Help: cfg.Help,
	}
	if len(cfg.Labels) == 0 { // regular counter
		counter := prometheus.NewCounter(counterOpts)
		return &counterMetric{
			name:      cfg.Name,
			regex:     regex,
			collector: counter,
			incFunc: func(_ string) {
				counter.Inc()
			},
		}
	} else { // counterVec
		counterVec := prometheus.NewCounterVec(counterOpts, prometheusLabels(cfg.Labels))
		return &counterMetric{
			name:      cfg.Name,
			regex:     regex,
			labels:    cfg.Labels,
			collector: counterVec,
			incFunc: func(line string) {
				counterVec.WithLabelValues(labelValues(line, regex, cfg.Labels)...).Inc()
			},
		}
	}
}

func NewGaugeMetric(cfg *MetricConfig, regex *rubex.Regexp) Metric {
	gaugeOpts := prometheus.GaugeOpts{
		Name: cfg.Name,
		Help: cfg.Help,
	}
	if len(cfg.Labels) == 0 { // regular gauge
		gauge := prometheus.NewGauge(gaugeOpts)
		return &valueMetric{
			name:      cfg.Name,
			regex:     regex,
			value:     cfg.Value,
			collector: gauge,
			observeFunc: func(_ string, val float64) {
				gauge.Add(val)
			},
		}
	} else { // gaugeVec
		gaugeVec := prometheus.NewGaugeVec(gaugeOpts, prometheusLabels(cfg.Labels))
		return &valueMetric{
			name:      cfg.Name,
			regex:     regex,
			value:     cfg.Value,
			collector: gaugeVec,
			labels:    cfg.Labels,
			observeFunc: func(line string, val float64) {
				gaugeVec.WithLabelValues(labelValues(line, regex, cfg.Labels)...).Add(val)
			},
		}
	}
}

func NewHistogramMetric(cfg *MetricConfig, regex *rubex.Regexp) Metric {
	histogramOpts := prometheus.HistogramOpts{
		Name: cfg.Name,
		Help: cfg.Help,
	}
	if len(cfg.Buckets) > 0 {
		histogramOpts.Buckets = cfg.Buckets
	}
	if len(cfg.Labels) == 0 { // regular histogram
		histogram := prometheus.NewHistogram(histogramOpts)
		return &valueMetric{
			name:      cfg.Name,
			regex:     regex,
			value:     cfg.Value,
			collector: histogram,
			observeFunc: func(_ string, val float64) {
				histogram.Observe(val)
			},
		}
	} else { // histogramVec
		histogramVec := prometheus.NewHistogramVec(histogramOpts, prometheusLabels(cfg.Labels))
		return &valueMetric{
			name:      cfg.Name,
			regex:     regex,
			value:     cfg.Value,
			collector: histogramVec,
			labels:    cfg.Labels,
			observeFunc: func(line string, val float64) {
				histogramVec.WithLabelValues(labelValues(line, regex, cfg.Labels)...).Observe(val)
			},
		}
	}
}

func NewSummaryMetric(cfg *MetricConfig, regex *rubex.Regexp) Metric {
	summaryOpts := prometheus.SummaryOpts{
		Name: cfg.Name,
		Help: cfg.Help,
	}
	if len(cfg.Quantiles) > 0 {
		summaryOpts.Objectives = cfg.Quantiles
	}
	if len(cfg.Labels) == 0 { // regular summary
		summary := prometheus.NewSummary(summaryOpts)
		return &valueMetric{
			name:      cfg.Name,
			regex:     regex,
			value:     cfg.Value,
			collector: summary,
			observeFunc: func(_ string, val float64) {
				summary.Observe(val)
			},
		}
	} else { // summaryVec
		summaryVec := prometheus.NewSummaryVec(summaryOpts, prometheusLabels(cfg.Labels))
		return &valueMetric{
			name:      cfg.Name,
			regex:     regex,
			value:     cfg.Value,
			collector: summaryVec,
			labels:    cfg.Labels,
			observeFunc: func(line string, val float64) {
				summaryVec.WithLabelValues(labelValues(line, regex, cfg.Labels)...).Observe(val)
			},
		}
	}
}

func (m *counterMetric) Process(line string) (error, bool) {
	if m.regex.MatchString(line) {
		m.incFunc(line)
		return nil, true
	} else {
		return nil, false
	}
}

func (m *valueMetric) Process(line string) (error, bool) {
	if m.regex.MatchString(line) {
		stringVal := m.regex.Gsub(line, fmt.Sprintf("\\k<%v>", m.value))
		floatVal, err := strconv.ParseFloat(stringVal, 64)
		if err != nil {
			return fmt.Errorf("error processing log line with metric %v: value '%v' matches '%v', which is not a valid number.", m.name, m.value, stringVal), true
		}
		m.observeFunc(line, floatVal)
		return nil, true
	} else {
		return nil, false
	}
}

func (m *counterMetric) Name() string {
	return m.name
}

func (m *valueMetric) Name() string {
	return m.name
}

func (m *counterMetric) Collector() prometheus.Collector {
	return m.collector
}

func (m *valueMetric) Collector() prometheus.Collector {
	return m.collector
}

func labelValues(line string, regex *rubex.Regexp, labels []Label) []string {
	values := make([]string, 0, len(labels))
	for _, field := range labels {
		value := regex.Gsub(line, fmt.Sprintf("\\k<%v>", field.GrokFieldName))
		values = append(values, value)
	}
	return values
}

func prometheusLabels(labels []Label) []string {
	promLabels := make([]string, 0, len(labels))
	for _, label := range labels {
		promLabels = append(promLabels, label.PrometheusLabel)
	}
	return promLabels
}
