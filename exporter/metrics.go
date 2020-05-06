// Copyright 2016-2020 The grok_exporter Authors
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
	configuration "github.com/fstab/grok_exporter/config/v3"
	"github.com/fstab/grok_exporter/oniguruma"
	"github.com/fstab/grok_exporter/tailer/glob"
	"github.com/fstab/grok_exporter/template"
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

	PathMatches(logfilePath string) bool
	// Returns the match if the line matched, and nil if the line didn't match.
	ProcessMatch(line string, additionalFields map[string]interface{}) (*Match, error)
	// Returns the match if the delete pattern matched, nil otherwise.
	ProcessDeleteMatch(line string, additionalFields map[string]interface{}) (*Match, error)
	// Remove old metrics
	ProcessRetention() error
}

// Common values for incMetric and observeMetric
type metric struct {
	name        string
	globs       []glob.Glob
	regex       *oniguruma.Regex
	deleteRegex *oniguruma.Regex
	retention   time.Duration
}

type observeMetric struct {
	metric
	valueTemplate template.Template
}

type metricWithLabels struct {
	metric
	labelTemplates       []template.Template
	deleteLabelTemplates []template.Template
	labelValueTracker    LabelValueTracker
}

type observeMetricWithLabels struct {
	metricWithLabels
	valueTemplate template.Template
}

type counterMetric struct {
	observeMetric
	counter prometheus.Counter
}

type counterVecMetric struct {
	observeMetricWithLabels
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

func (m *metric) PathMatches(logfilePath string) bool {
	if len(m.globs) == 0 {
		return true
	}
	for _, g := range m.globs {
		if g.Match(logfilePath) {
			return true
		}
	}
	return false
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

func (m *observeMetric) processMatch(line string, callback func(value float64) (bool, error)) (*Match, error) {
	searchResult, err := m.regex.Search(line)
	if err != nil {
		return nil, fmt.Errorf("error processing metric %v: %v", m.Name(), err.Error())
	}
	defer searchResult.Free()
	if searchResult.IsMatch() {
		floatVal, err := floatValue(m.Name(), searchResult, m.valueTemplate, nil)
		if err != nil {
			return nil, err
		}
		match, err := callback(floatVal)
		if err != nil {
			return nil, err
		}
		if match {
			return &Match{
				Value: floatVal,
			}, nil
		}
	}
	return nil, nil
}

func (m *observeMetricWithLabels) processMatch(line string, additionalFields map[string]interface{}, callback func(value float64, labels map[string]string) (bool, error)) (*Match, error) {
	searchResult, err := m.regex.Search(line)
	if err != nil {
		return nil, fmt.Errorf("error processing metric %v: %v", m.Name(), err.Error())
	}
	defer searchResult.Free()
	if searchResult.IsMatch() {
		floatVal, err := floatValue(m.Name(), searchResult, m.valueTemplate, additionalFields)
		if err != nil {
			return nil, err
		}
		labels, err := labelValues(m.Name(), searchResult, m.labelTemplates, additionalFields)
		if err != nil {
			return nil, err
		}
		m.labelValueTracker.Observe(labels)
		match, err := callback(floatVal, labels)
		if err != nil {
			return nil, err
		}
		if match {
			return &Match{
				Value:  floatVal,
				Labels: labels,
			}, nil
		}
	}
	return nil, nil
}

func (m *metric) ProcessDeleteMatch(line string, additionalFields map[string]interface{}) (*Match, error) {
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

func (m *metricWithLabels) processDeleteMatch(line string, vec deleterMetric, additionalFields map[string]interface{}) (*Match, error) {
	if m.deleteRegex == nil {
		return nil, nil
	}
	searchResult, err := m.deleteRegex.Search(line)
	if err != nil {
		return nil, fmt.Errorf("error processing metric %v: %v", m.name, err.Error())
	}
	defer searchResult.Free()
	if searchResult.IsMatch() {
		deleteLabels, err := labelValues(m.Name(), searchResult, m.deleteLabelTemplates, additionalFields)
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

func (m *counterMetric) ProcessMatch(line string, additionalFields map[string]interface{}) (*Match, error) {
	return m.processMatch(line, func(value float64) (bool, error) {
		if value < 0 {
			return false, fmt.Errorf("Negative value with metric counter")
		}
		m.counter.Add(value)
		return true, nil
	})
}

func (m *counterVecMetric) ProcessMatch(line string, additionalFields map[string]interface{}) (*Match, error) {
	return m.processMatch(line, additionalFields, func(value float64, labels map[string]string) (bool, error) {
		if value < 0 {
			return false, fmt.Errorf("Negative value with metric counter")
		}
		m.counterVec.With(labels).Add(value)
		return true, nil
	})
}

func (m *counterVecMetric) ProcessDeleteMatch(line string, additionalFields map[string]interface{}) (*Match, error) {
	return m.processDeleteMatch(line, m.counterVec, additionalFields)
}

func (m *counterVecMetric) ProcessRetention() error {
	return m.processRetention(m.counterVec)
}

func (m *gaugeMetric) ProcessMatch(line string, additionalFields map[string]interface{}) (*Match, error) {
	return m.processMatch(line, func(value float64) (bool, error) {
		if m.cumulative {
			m.gauge.Add(value)
		} else {
			m.gauge.Set(value)
		}
		return true, nil
	})
}

func (m *gaugeVecMetric) ProcessMatch(line string, additionalFields map[string]interface{}) (*Match, error) {
	return m.processMatch(line, additionalFields, func(value float64, labels map[string]string) (bool, error) {
		if m.cumulative {
			m.gaugeVec.With(labels).Add(value)
		} else {
			m.gaugeVec.With(labels).Set(value)
		}
		return true, nil
	})
}

func (m *gaugeVecMetric) ProcessDeleteMatch(line string, additionalFields map[string]interface{}) (*Match, error) {
	return m.processDeleteMatch(line, m.gaugeVec, additionalFields)
}

func (m *gaugeVecMetric) ProcessRetention() error {
	return m.processRetention(m.gaugeVec)
}

func (m *histogramMetric) ProcessMatch(line string, additionalFields map[string]interface{}) (*Match, error) {
	return m.processMatch(line, func(value float64) (bool, error) {
		m.histogram.Observe(value)
		return true, nil
	})
}

func (m *histogramVecMetric) ProcessMatch(line string, additionalFields map[string]interface{}) (*Match, error) {
	return m.processMatch(line, additionalFields, func(value float64, labels map[string]string) (bool, error) {
		m.histogramVec.With(labels).Observe(value)
		return true, nil
	})
}

func (m *histogramVecMetric) ProcessDeleteMatch(line string, additionalFields map[string]interface{}) (*Match, error) {
	return m.processDeleteMatch(line, m.histogramVec, additionalFields)
}

func (m *histogramVecMetric) ProcessRetention() error {
	return m.processRetention(m.histogramVec)
}

func (m *summaryMetric) ProcessMatch(line string, additionalFields map[string]interface{}) (*Match, error) {
	return m.processMatch(line, func(value float64) (bool, error) {
		m.summary.Observe(value)
		return true, nil
	})
}

func (m *summaryVecMetric) ProcessMatch(line string, additionalFields map[string]interface{}) (*Match, error) {
	return m.processMatch(line, additionalFields, func(value float64, labels map[string]string) (bool, error) {
		m.summaryVec.With(labels).Observe(value)
		return true, nil
	})
}

func (m *summaryVecMetric) ProcessDeleteMatch(line string, additionalFields map[string]interface{}) (*Match, error) {
	return m.processDeleteMatch(line, m.summaryVec, additionalFields)
}

func (m *summaryVecMetric) ProcessRetention() error {
	return m.processRetention(m.summaryVec)
}

func newMetric(cfg *configuration.MetricConfig, regex, deleteRegex *oniguruma.Regex) metric {
	return metric{
		name:        cfg.Name,
		globs:       cfg.Globs,
		regex:       regex,
		deleteRegex: deleteRegex,
		retention:   cfg.Retention,
	}
}

func newMetricWithLabels(cfg *configuration.MetricConfig, regex, deleteRegex *oniguruma.Regex) metricWithLabels {
	return metricWithLabels{
		metric:               newMetric(cfg, regex, deleteRegex),
		labelTemplates:       cfg.LabelTemplates,
		deleteLabelTemplates: cfg.DeleteLabelTemplates,
		labelValueTracker:    NewLabelValueTracker(prometheusLabels(cfg.LabelTemplates)),
	}
}

func newObserveMetric(cfg *configuration.MetricConfig, regex, deleteRegex *oniguruma.Regex) observeMetric {
	return observeMetric{
		metric:        newMetric(cfg, regex, deleteRegex),
		valueTemplate: cfg.ValueTemplate,
	}
}

func newObserveMetricWithLabels(cfg *configuration.MetricConfig, regex, deleteRegex *oniguruma.Regex) observeMetricWithLabels {
	return observeMetricWithLabels{
		metricWithLabels: newMetricWithLabels(cfg, regex, deleteRegex),
		valueTemplate:    cfg.ValueTemplate,
	}
}

func NewCounterMetric(cfg *configuration.MetricConfig, regex *oniguruma.Regex, deleteRegex *oniguruma.Regex) Metric {
	counterOpts := prometheus.CounterOpts{
		Name: cfg.Name,
		Help: cfg.Help,
	}
	if len(cfg.Labels) == 0 {
		return &counterMetric{
			observeMetric: newObserveMetric(cfg, regex, deleteRegex),
			counter:       prometheus.NewCounter(counterOpts),
		}
	} else {
		return &counterVecMetric{
			observeMetricWithLabels: newObserveMetricWithLabels(cfg, regex, deleteRegex),
			counterVec:              prometheus.NewCounterVec(counterOpts, prometheusLabels(cfg.LabelTemplates)),
		}
	}
}

func NewGaugeMetric(cfg *configuration.MetricConfig, regex *oniguruma.Regex, deleteRegex *oniguruma.Regex) Metric {
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

func NewHistogramMetric(cfg *configuration.MetricConfig, regex *oniguruma.Regex, deleteRegex *oniguruma.Regex) Metric {
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

func NewSummaryMetric(cfg *configuration.MetricConfig, regex *oniguruma.Regex, deleteRegex *oniguruma.Regex) Metric {
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

func labelValues(metricName string, searchResult *oniguruma.SearchResult, templates []template.Template, additionalFields map[string]interface{}) (map[string]string, error) {
	result := make(map[string]string, len(templates))
	for _, t := range templates {
		value, err := evalTemplate(searchResult, t, additionalFields)
		if err != nil {
			return nil, fmt.Errorf("error processing metric %v: %v", metricName, err.Error())
		}
		result[t.Name()] = value
	}
	return result, nil
}

func floatValue(metricName string, searchResult *oniguruma.SearchResult, valueTemplate template.Template, additionalFields map[string]interface{}) (float64, error) {
	stringVal, err := evalTemplate(searchResult, valueTemplate, additionalFields)
	if err != nil {
		return 0, fmt.Errorf("error processing metric %v: %v", metricName, err.Error())
	}
	floatVal, err := strconv.ParseFloat(stringVal, 64)
	if err != nil {
		return 0, fmt.Errorf("error processing metric %v: value matches '%v', which is not a valid number", metricName, stringVal)
	}
	return floatVal, nil
}

func evalTemplate(searchResult *oniguruma.SearchResult, t template.Template, additionalFields map[string]interface{}) (string, error) {
	var (
		values = make(map[string]interface{}, len(t.ReferencedGrokFields()))
		value  interface{}
		ok     bool
		err    error
		field  string
	)
	for _, field = range t.ReferencedGrokFields() {
		if value, ok = additionalFields[field]; !ok {
			value, err = searchResult.GetCaptureGroupByName(field)
			if err != nil {
				return "", err
			}
		}
		values[field] = value
	}
	return t.Execute(values)
}

func prometheusLabels(templates []template.Template) []string {
	promLabels := make([]string, 0, len(templates))
	for _, t := range templates {
		promLabels = append(promLabels, t.Name())
	}
	return promLabels
}
