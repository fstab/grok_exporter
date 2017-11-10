// Copyright 2016-2017 The grok_exporter Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package exporter

import (
	"fmt"
	configuration "github.com/fstab/grok_exporter/config/v2"
	"github.com/fstab/grok_exporter/templates"
	"github.com/prometheus/client_golang/prometheus"
	"strconv"
	"time"
)

type Match struct {
	Labels map[string]string
	Value  float64
}

type Metric interface {
	Name() string
	Collector() prometheus.Collector

	// Returns the match if the line matched, and nil if the line didn't match.
	ProcessMatch(line string) (*Match, error)
	// Returns the match if the delete pattern matched, nil otherwise.
	ProcessDeleteMatch(line string) (*Match, error)
	// Remove old metrics
	ProcessRetention() error
}

// Common values for incMetric and observeMetric
type metric struct {
	name        string
	regex       *OnigurumaRegexp
	deleteRegex *OnigurumaRegexp
	retention   time.Duration
}

type observeMetric struct {
	metric
	valueTemplate templates.Template
}

type metricWithLabels struct {
	metric
	labelTemplates       []templates.Template
	deleteLabelTemplates []templates.Template
	labelValueTracker    LabelValueTracker
}

type observeMetricWithLabels struct {
	metricWithLabels
	valueTemplate templates.Template
}

type counterMetric struct {
	metric
	counter prometheus.Counter
}

type counterVecMetric struct {
	metricWithLabels
	counterVec *prometheus.CounterVec
}

type gaugeMetric struct {
	observeMetric
	cumulative bool
	gauge      prometheus.Gauge
}

type gaugeVecMetric struct {
	observeMetricWithLabels
	cumulative bool
	gaugeVec   *prometheus.GaugeVec
}

type histogramMetric struct {
	observeMetric
	histogram prometheus.Histogram
}

type histogramVecMetric struct {
	observeMetricWithLabels
	histogramVec *prometheus.HistogramVec
}

type summaryMetric struct {
	observeMetric
	summary prometheus.Summary
}

type summaryVecMetric struct {
	observeMetricWithLabels
	summaryVec *prometheus.SummaryVec
}

type deleterMetric interface {
	Delete(prometheus.Labels) bool
}

func (m *metric) Name() string {
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

func (m *metric) processMatch(line string, cb func()) (*Match, error) {
	matchResult, err := m.regex.Match(line)
	if err != nil {
		return nil, fmt.Errorf("error processing metric %v: %v", m.Name(), err.Error())
	}
	defer matchResult.Free()
	if matchResult.IsMatch() {
		cb()
		return &Match{
			Value: 1.0,
		}, nil
	} else {
		return nil, nil
	}
}

func (m *observeMetric) processMatch(line string, cb func(value float64)) (*Match, error) {
	matchResult, err := m.regex.Match(line)
	if err != nil {
		return nil, fmt.Errorf("error processing metric %v: %v", m.Name(), err.Error())
	}
	defer matchResult.Free()
	if matchResult.IsMatch() {
		floatVal, err := floatValue(m.Name(), matchResult, m.valueTemplate)
		if err != nil {
			return nil, err
		}
		cb(floatVal)
		return &Match{
			Value: floatVal,
		}, nil
	} else {
		return nil, nil
	}
}

func (m *metricWithLabels) processMatch(line string, cb func(labels map[string]string)) (*Match, error) {
	matchResult, err := m.regex.Match(line)
	if err != nil {
		return nil, fmt.Errorf("error while processing metric %v: %v", m.Name(), err.Error())
	}
	defer matchResult.Free()
	if matchResult.IsMatch() {
		labels, err := labelValues(m.Name(), matchResult, m.labelTemplates)
		if err != nil {
			return nil, err
		}
		m.labelValueTracker.Observe(labels)
		cb(labels)
		return &Match{
			Value:  1.0,
			Labels: labels,
		}, nil
	} else {
		return nil, nil
	}
}

func (m *observeMetricWithLabels) processMatch(line string, cb func(value float64, labels map[string]string)) (*Match, error) {
	matchResult, err := m.regex.Match(line)
	if err != nil {
		return nil, fmt.Errorf("error processing metric %v: %v", m.Name(), err.Error())
	}
	defer matchResult.Free()
	if matchResult.IsMatch() {
		floatVal, err := floatValue(m.Name(), matchResult, m.valueTemplate)
		if err != nil {
			return nil, err
		}
		labels, err := labelValues(m.Name(), matchResult, m.labelTemplates)
		if err != nil {
			return nil, err
		}
		m.labelValueTracker.Observe(labels)
		cb(floatVal, labels)
		return &Match{
			Value:  floatVal,
			Labels: labels,
		}, nil
	} else {
		return nil, nil
	}
}

func (m *metric) ProcessDeleteMatch(line string) (*Match, error) {
	if m.deleteRegex == nil {
		return nil, nil
	}
	return nil, fmt.Errorf("error processing metric %v: delete_match is currently only supported for metrics with labels.", m.Name())
}

func (m *metric) ProcessRetention() error {
	if m.retention == 0 {
		return nil
	}
	return fmt.Errorf("error processing metric %v: retention is currently only supported for metrics with labels.", m.Name())
}

func (m *metricWithLabels) processDeleteMatch(line string, vec deleterMetric) (*Match, error) {
	if m.deleteRegex == nil {
		return nil, nil
	}
	matchResult, err := m.deleteRegex.Match(line)
	if err != nil {
		return nil, fmt.Errorf("error processing metric %v: %v", m.name, err.Error())
	}
	defer matchResult.Free()
	if matchResult.IsMatch() {
		deleteLabels, err := labelValues(m.Name(), matchResult, m.deleteLabelTemplates)
		if err != nil {
			return nil, err
		}
		matchingLabels, err := m.labelValueTracker.DeleteByLabels(deleteLabels)
		if err != nil {
			return nil, err
		}
		for _, matchingLabel := range matchingLabels {
			vec.Delete(matchingLabel)
		}
		return &Match{
			Labels: deleteLabels,
		}, nil
	} else {
		return nil, nil
	}
}

func (m *metricWithLabels) processRetention(vec deleterMetric) error {
	if m.retention != 0 {
		for _, label := range m.labelValueTracker.DeleteByRetention(m.retention) {
			vec.Delete(label)
		}
	}
	return nil
}

func (m *counterMetric) ProcessMatch(line string) (*Match, error) {
	return m.processMatch(line, func() {
		m.counter.Inc()
	})
}

func (m *counterVecMetric) ProcessMatch(line string) (*Match, error) {
	return m.processMatch(line, func(labels map[string]string) {
		m.counterVec.With(labels).Inc()
	})
}

func (m *counterVecMetric) ProcessDeleteMatch(line string) (*Match, error) {
	return m.processDeleteMatch(line, m.counterVec)
}

func (m *counterVecMetric) ProcessRetention() error {
	return m.processRetention(m.counterVec)
}

func (m *gaugeMetric) ProcessMatch(line string) (*Match, error) {
	return m.processMatch(line, func(value float64) {
		if m.cumulative {
			m.gauge.Add(value)
		} else {
			m.gauge.Set(value)
		}
	})
}

func (m *gaugeVecMetric) ProcessMatch(line string) (*Match, error) {
	return m.processMatch(line, func(value float64, labels map[string]string) {
		if m.cumulative {
			m.gaugeVec.With(labels).Add(value)
		} else {
			m.gaugeVec.With(labels).Set(value)
		}
	})
}

func (m *gaugeVecMetric) ProcessDeleteMatch(line string) (*Match, error) {
	return m.processDeleteMatch(line, m.gaugeVec)
}

func (m *gaugeVecMetric) ProcessRetention() error {
	return m.processRetention(m.gaugeVec)
}

func (m *histogramMetric) ProcessMatch(line string) (*Match, error) {
	return m.processMatch(line, func(value float64) {
		m.histogram.Observe(value)
	})
}

func (m *histogramVecMetric) ProcessMatch(line string) (*Match, error) {
	return m.processMatch(line, func(value float64, labels map[string]string) {
		m.histogramVec.With(labels).Observe(value)
	})
}

func (m *histogramVecMetric) ProcessDeleteMatch(line string) (*Match, error) {
	return m.processDeleteMatch(line, m.histogramVec)
}

func (m *histogramVecMetric) ProcessRetention() error {
	return m.processRetention(m.histogramVec)
}

func (m *summaryMetric) ProcessMatch(line string) (*Match, error) {
	return m.processMatch(line, func(value float64) {
		m.summary.Observe(value)
	})
}

func (m *summaryVecMetric) ProcessMatch(line string) (*Match, error) {
	return m.processMatch(line, func(value float64, labels map[string]string) {
		m.summaryVec.With(labels).Observe(value)
	})
}

func (m *summaryVecMetric) ProcessDeleteMatch(line string) (*Match, error) {
	return m.processDeleteMatch(line, m.summaryVec)
}

func (m *summaryVecMetric) ProcessRetention() error {
	return m.processRetention(m.summaryVec)
}

func newMetric(cfg *configuration.MetricConfig, regex, deleteRegex *OnigurumaRegexp) metric {
	return metric{
		name:        cfg.Name,
		regex:       regex,
		deleteRegex: deleteRegex,
		retention:   cfg.Retention,
	}
}

func newMetricWithLabels(cfg *configuration.MetricConfig, regex, deleteRegex *OnigurumaRegexp) metricWithLabels {
	return metricWithLabels{
		metric:               newMetric(cfg, regex, deleteRegex),
		labelTemplates:       cfg.LabelTemplates,
		deleteLabelTemplates: cfg.DeleteLabelTemplates,
		labelValueTracker:    NewLabelValueTracker(prometheusLabels(cfg.LabelTemplates)),
	}
}

func newObserveMetric(cfg *configuration.MetricConfig, regex, deleteRegex *OnigurumaRegexp) observeMetric {
	return observeMetric{
		metric:        newMetric(cfg, regex, deleteRegex),
		valueTemplate: cfg.ValueTemplate,
	}
}

func newObserveMetricWithLabels(cfg *configuration.MetricConfig, regex, deleteRegex *OnigurumaRegexp) observeMetricWithLabels {
	return observeMetricWithLabels{
		metricWithLabels: newMetricWithLabels(cfg, regex, deleteRegex),
		valueTemplate:    cfg.ValueTemplate,
	}
}

func NewCounterMetric(cfg *configuration.MetricConfig, regex *OnigurumaRegexp, deleteRegex *OnigurumaRegexp) Metric {
	counterOpts := prometheus.CounterOpts{
		Name: cfg.Name,
		Help: cfg.Help,
	}
	if len(cfg.Labels) == 0 {
		return &counterMetric{
			metric:  newMetric(cfg, regex, deleteRegex),
			counter: prometheus.NewCounter(counterOpts),
		}
	} else {
		return &counterVecMetric{
			metricWithLabels: newMetricWithLabels(cfg, regex, deleteRegex),
			counterVec:       prometheus.NewCounterVec(counterOpts, prometheusLabels(cfg.LabelTemplates)),
		}
	}
}

func NewGaugeMetric(cfg *configuration.MetricConfig, regex *OnigurumaRegexp, deleteRegex *OnigurumaRegexp) Metric {
	gaugeOpts := prometheus.GaugeOpts{
		Name: cfg.Name,
		Help: cfg.Help,
	}
	if len(cfg.Labels) == 0 {
		return &gaugeMetric{
			observeMetric: newObserveMetric(cfg, regex, deleteRegex),
			cumulative:    cfg.Cumulative,
			gauge:         prometheus.NewGauge(gaugeOpts),
		}
	} else {
		return &gaugeVecMetric{
			observeMetricWithLabels: newObserveMetricWithLabels(cfg, regex, deleteRegex),
			cumulative:              cfg.Cumulative,
			gaugeVec:                prometheus.NewGaugeVec(gaugeOpts, prometheusLabels(cfg.LabelTemplates)),
		}
	}
}

func NewHistogramMetric(cfg *configuration.MetricConfig, regex *OnigurumaRegexp, deleteRegex *OnigurumaRegexp) Metric {
	histogramOpts := prometheus.HistogramOpts{
		Name: cfg.Name,
		Help: cfg.Help,
	}
	if len(cfg.Buckets) > 0 {
		histogramOpts.Buckets = cfg.Buckets
	}
	if len(cfg.Labels) == 0 {
		return &histogramMetric{
			observeMetric: newObserveMetric(cfg, regex, deleteRegex),
			histogram:     prometheus.NewHistogram(histogramOpts),
		}
	} else {
		return &histogramVecMetric{
			observeMetricWithLabels: newObserveMetricWithLabels(cfg, regex, deleteRegex),
			histogramVec:            prometheus.NewHistogramVec(histogramOpts, prometheusLabels(cfg.LabelTemplates)),
		}
	}
}

func NewSummaryMetric(cfg *configuration.MetricConfig, regex *OnigurumaRegexp, deleteRegex *OnigurumaRegexp) Metric {
	summaryOpts := prometheus.SummaryOpts{
		Name: cfg.Name,
		Help: cfg.Help,
	}
	if len(cfg.Quantiles) > 0 {
		summaryOpts.Objectives = cfg.Quantiles
	}
	if len(cfg.Labels) == 0 {
		return &summaryMetric{
			observeMetric: newObserveMetric(cfg, regex, deleteRegex),
			summary:       prometheus.NewSummary(summaryOpts),
		}
	} else {
		return &summaryVecMetric{
			observeMetricWithLabels: newObserveMetricWithLabels(cfg, regex, deleteRegex),
			summaryVec:              prometheus.NewSummaryVec(summaryOpts, prometheusLabels(cfg.LabelTemplates)),
		}
	}
}

func labelValues(metricName string, matchResult *OnigurumaMatchResult, templates []templates.Template) (map[string]string, error) {
	result := make(map[string]string, len(templates))
	for _, t := range templates {
		value, err := evalTemplate(matchResult, t)
		if err != nil {
			return nil, fmt.Errorf("error processing metric %v: %v", metricName, err.Error())
		}
		result[t.Name()] = value
	}
	return result, nil
}

func floatValue(metricName string, matchResult *OnigurumaMatchResult, valueTemplate templates.Template) (float64, error) {
	stringVal, err := evalTemplate(matchResult, valueTemplate)
	if err != nil {
		return 0, fmt.Errorf("error processing metric %v: %v", metricName, err.Error())
	}
	floatVal, err := strconv.ParseFloat(stringVal, 64)
	if err != nil {
		return 0, fmt.Errorf("error processing metric %v: value matches '%v', which is not a valid number.", metricName, stringVal)
	}
	return floatVal, nil
}

func evalTemplate(matchResult *OnigurumaMatchResult, t templates.Template) (string, error) {
	grokValues := make(map[string]string, len(t.ReferencedGrokFields()))
	for _, field := range t.ReferencedGrokFields() {
		value, err := matchResult.Get(field)
		if err != nil {
			return "", err
		}
		grokValues[field] = value
	}
	return t.Execute(grokValues)
}

func prometheusLabels(templates []templates.Template) []string {
	promLabels := make([]string, 0, len(templates))
	for _, t := range templates {
		promLabels = append(promLabels, t.Name())
	}
	return promLabels
}
