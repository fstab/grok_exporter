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
	Process(line string) error
}

type counterMetric struct {
	name    string
	regex   *rubex.Regexp
	counter prometheus.Counter
}

type counterVecMetric struct {
	name       string
	regex      *rubex.Regexp
	labels     []Label
	counterVec *prometheus.CounterVec
}

type gaugeMetric struct {
	name  string
	regex *rubex.Regexp
	value string
	gauge prometheus.Gauge
}

type gaugeVecMetric struct {
	name     string
	regex    *rubex.Regexp
	labels   []Label
	value    string
	gaugeVec *prometheus.GaugeVec
}

type histogramMetric struct {
	name      string
	regex     *rubex.Regexp
	value     string
	histogram prometheus.Histogram
}

type histogramVecMetric struct {
	name         string
	regex        *rubex.Regexp
	labels       []Label
	value        string
	histogramVec *prometheus.HistogramVec
}

type summaryMetric struct {
	name    string
	regex   *rubex.Regexp
	value   string
	summary prometheus.Summary
}

type summaryVecMetric struct {
	name       string
	regex      *rubex.Regexp
	labels     []Label
	value      string
	summaryVec *prometheus.SummaryVec
}

func prometheusLabels(labels []Label) []string {
	promLabels := make([]string, 0, len(labels))
	for _, label := range labels {
		promLabels = append(promLabels, label.PrometheusLabel)
	}
	return promLabels
}

func NewCounterMetric(cfg *MetricConfig, regex *rubex.Regexp) Metric {
	counterOpts := prometheus.CounterOpts{
		Name: cfg.Name,
		Help: cfg.Help,
	}
	if len(cfg.Labels) == 0 { // regular counter
		return &counterMetric{
			name:    cfg.Name,
			regex:   regex,
			counter: prometheus.NewCounter(counterOpts),
		}
	} else { // counterVec
		return &counterVecMetric{
			name:       cfg.Name,
			regex:      regex,
			labels:     cfg.Labels,
			counterVec: prometheus.NewCounterVec(counterOpts, prometheusLabels(cfg.Labels)),
		}
	}
}

func NewGaugeMetric(cfg *MetricConfig, regex *rubex.Regexp) Metric {
	gaugeOpts := prometheus.GaugeOpts{
		Name: cfg.Name,
		Help: cfg.Help,
	}
	if len(cfg.Labels) == 0 { // regular gauge
		return &gaugeMetric{
			name:  cfg.Name,
			regex: regex,
			value: cfg.Value,
			gauge: prometheus.NewGauge(gaugeOpts),
		}
	} else { // gaugeVec
		return &gaugeVecMetric{
			name:     cfg.Name,
			regex:    regex,
			labels:   cfg.Labels,
			value:    cfg.Value,
			gaugeVec: prometheus.NewGaugeVec(gaugeOpts, prometheusLabels(cfg.Labels)),
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
		return &histogramMetric{
			name:      cfg.Name,
			regex:     regex,
			value:     cfg.Value,
			histogram: prometheus.NewHistogram(histogramOpts),
		}
	} else { // histogramVec
		return &histogramVecMetric{
			name:         cfg.Name,
			regex:        regex,
			labels:       cfg.Labels,
			value:        cfg.Value,
			histogramVec: prometheus.NewHistogramVec(histogramOpts, prometheusLabels(cfg.Labels)),
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
		return &summaryMetric{
			name:    cfg.Name,
			regex:   regex,
			value:   cfg.Value,
			summary: prometheus.NewSummary(summaryOpts),
		}
	} else { // summaryVec
		return &summaryVecMetric{
			name:       cfg.Name,
			regex:      regex,
			labels:     cfg.Labels,
			value:      cfg.Value,
			summaryVec: prometheus.NewSummaryVec(summaryOpts, prometheusLabels(cfg.Labels)),
		}
	}
}

func (m *counterMetric) Name() string {
	return m.name
}

func (m *counterVecMetric) Name() string {
	return m.name
}

func (m *gaugeMetric) Name() string {
	return m.name
}

func (m *gaugeVecMetric) Name() string {
	return m.name
}

func (m *histogramMetric) Name() string {
	return m.name
}

func (m *histogramVecMetric) Name() string {
	return m.name
}

func (m *summaryMetric) Name() string {
	return m.name
}

func (m *summaryVecMetric) Name() string {
	return m.name
}

func (m *counterMetric) Collector() prometheus.Collector {
	return m.counter
}

func (m *counterVecMetric) Collector() prometheus.Collector {
	return m.counterVec
}

func (m *gaugeMetric) Collector() prometheus.Collector {
	return m.gauge
}

func (m *gaugeVecMetric) Collector() prometheus.Collector {
	return m.gaugeVec
}

func (m *histogramMetric) Collector() prometheus.Collector {
	return m.histogram
}

func (m *histogramVecMetric) Collector() prometheus.Collector {
	return m.histogramVec
}

func (m *summaryMetric) Collector() prometheus.Collector {
	return m.summary
}

func (m *summaryVecMetric) Collector() prometheus.Collector {
	return m.summaryVec
}

func (m *counterMetric) Process(line string) error {
	if m.regex.MatchString(line) {
		m.counter.Inc()
	}
	return nil
}

func (m *counterVecMetric) Process(line string) error {
	if m.regex.MatchString(line) {
		m.counterVec.WithLabelValues(labelValues(line, m.regex, m.labels)...).Inc()
	}
	return nil
}

func (m *gaugeMetric) Process(line string) error {
	return process(line, m.name, m.value, m.regex, m.gauge.Add)
}

func (m *gaugeVecMetric) Process(line string) error {
	observeFunc := func(val float64) {
		m.gaugeVec.WithLabelValues(labelValues(line, m.regex, m.labels)...).Add(val)
	}
	return process(line, m.name, m.value, m.regex, observeFunc)
}

func (m *histogramMetric) Process(line string) error {
	return process(line, m.name, m.value, m.regex, m.histogram.Observe)
}

func (m *histogramVecMetric) Process(line string) error {
	observeFunc := func(val float64) {
		m.histogramVec.WithLabelValues(labelValues(line, m.regex, m.labels)...).Observe(val)
	}
	return process(line, m.name, m.value, m.regex, observeFunc)
}

func (m *summaryMetric) Process(line string) error {
	return process(line, m.name, m.value, m.regex, m.summary.Observe)
}

func (m *summaryVecMetric) Process(line string) error {
	observeFunc := func(val float64) {
		m.summaryVec.WithLabelValues(labelValues(line, m.regex, m.labels)...).Observe(val)
	}
	return process(line, m.name, m.value, m.regex, observeFunc)
}

func process(line, name, value string, regex *rubex.Regexp, observeFunc func(float64)) error {
	if regex.MatchString(line) {
		stringVal := regex.Gsub(line, fmt.Sprintf("\\k<%v>", value))
		floatVal, err := strconv.ParseFloat(stringVal, 64)
		if err != nil {
			return fmt.Errorf("error processing log line with metric %v: value '%v' matches '%v', which is not a valid number.", name, value, stringVal)
		}
		observeFunc(floatVal)
	}
	return nil
}

func labelValues(line string, regex *rubex.Regexp, labels []Label) []string {
	values := make([]string, 0, len(labels))
	for _, field := range labels {
		value := regex.Gsub(line, fmt.Sprintf("\\k<%v>", field.GrokFieldName))
		values = append(values, value)
	}
	return values
}
