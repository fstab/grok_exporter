package exporter

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"strconv"
)

type Metric interface {
	Name() string
	Collector() prometheus.Collector

	// Returns true if the line matched, and false if the line didn't match.
	Process(line string) (bool, error)
}

// Represents a Prometheus Counter
type incMetric struct {
	name      string
	regex     *OnigurumaRegexp
	labels    []Label
	collector prometheus.Collector
	incFunc   func(m *OnigurumaMatchResult) error
}

// Represents a Prometheus Gauge, Histogram, or Summary
type observeMetric struct {
	name        string
	regex       *OnigurumaRegexp
	value       string
	labels      []Label
	collector   prometheus.Collector
	observeFunc func(m *OnigurumaMatchResult, val float64) error
}

func NewCounterMetric(cfg *MetricConfig, regex *OnigurumaRegexp) Metric {
	counterOpts := prometheus.CounterOpts{
		Name: cfg.Name,
		Help: cfg.Help,
	}
	if len(cfg.Labels) == 0 { // regular counter
		counter := prometheus.NewCounter(counterOpts)
		return &incMetric{
			name:      cfg.Name,
			regex:     regex,
			collector: counter,
			incFunc: func(_ *OnigurumaMatchResult) error {
				counter.Inc()
				return nil
			},
		}
	} else { // counterVec
		counterVec := prometheus.NewCounterVec(counterOpts, prometheusLabels(cfg.Labels))
		return &incMetric{
			name:      cfg.Name,
			regex:     regex,
			labels:    cfg.Labels,
			collector: counterVec,
			incFunc: func(m *OnigurumaMatchResult) error {
				vals, err := labelValues(m, cfg.Labels)
				if err == nil {
					counterVec.WithLabelValues(vals...).Inc()
				}
				return err
			},
		}
	}
}

func NewGaugeMetric(cfg *MetricConfig, regex *OnigurumaRegexp) Metric {
	gaugeOpts := prometheus.GaugeOpts{
		Name: cfg.Name,
		Help: cfg.Help,
	}
	if len(cfg.Labels) == 0 { // regular gauge
		gauge := prometheus.NewGauge(gaugeOpts)
		return &observeMetric{
			name:      cfg.Name,
			regex:     regex,
			value:     cfg.Value,
			collector: gauge,
			observeFunc: func(_ *OnigurumaMatchResult, val float64) error {
				gauge.Add(val)
				return nil
			},
		}
	} else { // gaugeVec
		gaugeVec := prometheus.NewGaugeVec(gaugeOpts, prometheusLabels(cfg.Labels))
		return &observeMetric{
			name:      cfg.Name,
			regex:     regex,
			value:     cfg.Value,
			collector: gaugeVec,
			labels:    cfg.Labels,
			observeFunc: func(m *OnigurumaMatchResult, val float64) error {
				vals, err := labelValues(m, cfg.Labels)
				if err == nil {
					gaugeVec.WithLabelValues(vals...).Add(val)
				}
				return err
			},
		}
	}
}

func NewHistogramMetric(cfg *MetricConfig, regex *OnigurumaRegexp) Metric {
	histogramOpts := prometheus.HistogramOpts{
		Name: cfg.Name,
		Help: cfg.Help,
	}
	if len(cfg.Buckets) > 0 {
		histogramOpts.Buckets = cfg.Buckets
	}
	if len(cfg.Labels) == 0 { // regular histogram
		histogram := prometheus.NewHistogram(histogramOpts)
		return &observeMetric{
			name:      cfg.Name,
			regex:     regex,
			value:     cfg.Value,
			collector: histogram,
			observeFunc: func(_ *OnigurumaMatchResult, val float64) error {
				histogram.Observe(val)
				return nil
			},
		}
	} else { // histogramVec
		histogramVec := prometheus.NewHistogramVec(histogramOpts, prometheusLabels(cfg.Labels))
		return &observeMetric{
			name:      cfg.Name,
			regex:     regex,
			value:     cfg.Value,
			collector: histogramVec,
			labels:    cfg.Labels,
			observeFunc: func(m *OnigurumaMatchResult, val float64) error {
				vals, err := labelValues(m, cfg.Labels)
				if err == nil {
					histogramVec.WithLabelValues(vals...).Observe(val)
				}
				return err
			},
		}
	}
}

func NewSummaryMetric(cfg *MetricConfig, regex *OnigurumaRegexp) Metric {
	summaryOpts := prometheus.SummaryOpts{
		Name: cfg.Name,
		Help: cfg.Help,
	}
	if len(cfg.Quantiles) > 0 {
		summaryOpts.Objectives = cfg.Quantiles
	}
	if len(cfg.Labels) == 0 { // regular summary
		summary := prometheus.NewSummary(summaryOpts)
		return &observeMetric{
			name:      cfg.Name,
			regex:     regex,
			value:     cfg.Value,
			collector: summary,
			observeFunc: func(_ *OnigurumaMatchResult, val float64) error {
				summary.Observe(val)
				return nil
			},
		}
	} else { // summaryVec
		summaryVec := prometheus.NewSummaryVec(summaryOpts, prometheusLabels(cfg.Labels))
		return &observeMetric{
			name:      cfg.Name,
			regex:     regex,
			value:     cfg.Value,
			collector: summaryVec,
			labels:    cfg.Labels,
			observeFunc: func(m *OnigurumaMatchResult, val float64) error {
				vals, err := labelValues(m, cfg.Labels)
				if err == nil {
					summaryVec.WithLabelValues(vals...).Observe(val)
				}
				return err
			},
		}
	}
}

// Return: true if the line matched, false if it didn't match.
func (m *incMetric) Process(line string) (bool, error) {
	matchResult, err := m.regex.Match(line)
	if err != nil {
		return false, fmt.Errorf("error while processing metric %v: %v", m.name, err.Error())
	}
	defer matchResult.Free()
	if matchResult.IsMatch() {
		err = m.incFunc(matchResult)
		return true, err
	} else {
		return false, nil
	}
}

// Return: true if the line matched, false if it didn't match.
func (m *observeMetric) Process(line string) (bool, error) {
	matchResult, err := m.regex.Match(line)
	if err != nil {
		return false, fmt.Errorf("error while processing metric %v: %v", m.name, err.Error())
	}
	defer matchResult.Free()
	if matchResult.IsMatch() {
		stringVal, err := matchResult.Get(m.value)
		if err != nil {
			return true, fmt.Errorf("error while processing metric %v: %v", m.name, err.Error())
		}
		floatVal, err := strconv.ParseFloat(stringVal, 64)
		if err != nil {
			return true, fmt.Errorf("error while processing metric %v: value '%v' matches '%v', which is not a valid number.", m.name, m.value, stringVal)
		}
		err = m.observeFunc(matchResult, floatVal)
		return true, err
	} else {
		return false, nil
	}
}

func (m *incMetric) Name() string {
	return m.name
}

func (m *observeMetric) Name() string {
	return m.name
}

func (m *incMetric) Collector() prometheus.Collector {
	return m.collector
}

func (m *observeMetric) Collector() prometheus.Collector {
	return m.collector
}

func labelValues(matchResult *OnigurumaMatchResult, labels []Label) ([]string, error) {
	values := make([]string, 0, len(labels))
	for _, field := range labels {
		value, err := matchResult.Get(field.GrokFieldName)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func prometheusLabels(labels []Label) []string {
	promLabels := make([]string, 0, len(labels))
	for _, label := range labels {
		promLabels = append(promLabels, label.PrometheusLabel)
	}
	return promLabels
}
