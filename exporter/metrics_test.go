// Copyright 2016-2018 The grok_exporter Authors
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
	configuration "github.com/fstab/grok_exporter/config/v2"
	"github.com/fstab/grok_exporter/oniguruma"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_model/go"
	"reflect"
	"testing"
)

func TestCounterVec(t *testing.T) {
	regex := initCounterRegex(t)
	counterCfg := newMetricConfig(t, &configuration.MetricConfig{
		Name: "exim_rejected_rcpt_total",
		Labels: map[string]string{
			"error_message": "{{.message}}",
		},
	})
	counter := NewCounterMetric(counterCfg, regex, nil)
	counter.ProcessMatch("some unrelated line")
	counter.ProcessMatch("2016-04-26 10:19:57 H=(85.214.241.101) [36.224.138.227] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted")
	counter.ProcessMatch("2016-04-26 12:31:39 H=(186-90-8-31.genericrev.cantv.net) [186.90.8.31] F=<Hans.Krause9@cantv.net> rejected RCPT <ug2seeng-admin@example.com>: Unrouteable address")
	counter.ProcessMatch("2016-04-26 10:19:57 H=(85.214.241.101) [36.224.138.227] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted")

	switch c := counter.Collector().(type) {
	case *prometheus.CounterVec:
		m := io_prometheus_client.Metric{}
		c.WithLabelValues("relay not permitted").Write(&m)
		if *m.Counter.Value != float64(2) {
			t.Errorf("Expected 2 matches, but got %v matches.", *m.Counter.Value)
		}
		c.WithLabelValues("Unrouteable address").Write(&m)
		if *m.Counter.Value != float64(1) {
			t.Errorf("Expected 1 match, but got %v matches.", *m.Counter.Value)
		}
	default:
		t.Errorf("Unexpected type of metric: %v", reflect.TypeOf(c))
	}
}

func TestCounter(t *testing.T) {
	regex := initCounterRegex(t)
	counterCfg := newMetricConfig(t, &configuration.MetricConfig{
		Name: "exim_rejected_rcpt_total",
	})
	counter := NewCounterMetric(counterCfg, regex, nil)

	counter.ProcessMatch("some unrelated line")
	counter.ProcessMatch("2016-04-26 10:19:57 H=(85.214.241.101) [36.224.138.227] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted")
	counter.ProcessMatch("2016-04-26 12:31:39 H=(186-90-8-31.genericrev.cantv.net) [186.90.8.31] F=<Hans.Krause9@cantv.net> rejected RCPT <ug2seeng-admin@example.com>: Unrouteable address")
	counter.ProcessMatch("2016-04-26 10:19:57 H=(85.214.241.101) [36.224.138.227] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted")

	switch c := counter.Collector().(type) {
	case prometheus.Counter:
		m := io_prometheus_client.Metric{}
		c.Write(&m)
		if *m.Counter.Value != float64(3) {
			t.Errorf("Expected 3 matches, but got %v matches.", *m.Counter.Value)
		}
	default:
		t.Errorf("Unexpected type of metric: %v", reflect.TypeOf(c))
	}
}

func initCounterRegex(t *testing.T) *oniguruma.Regex {
	patterns := loadPatternDir(t)
	err := patterns.AddPattern("EXIM_MESSAGE [a-zA-Z ]*")
	if err != nil {
		t.Error(err)
	}
	regex, err := Compile("%{EXIM_DATE} %{EXIM_REMOTE_HOST} F=<%{EMAILADDRESS}> rejected RCPT <%{EMAILADDRESS}>: %{EXIM_MESSAGE:message}", patterns)
	if err != nil {
		t.Error(err)
	}
	return regex
}

func TestGauge(t *testing.T) {
	regex := initGaugeRegex(t)
	gaugeCfg := newMetricConfig(t, &configuration.MetricConfig{
		Name:  "temperature",
		Value: "{{.temperature}}",
	})
	gauge := NewGaugeMetric(gaugeCfg, regex, nil)

	gauge.ProcessMatch("Temperature in Berlin: 32")
	gauge.ProcessMatch("Temperature in Moscow: -5")

	switch c := gauge.Collector().(type) {
	case prometheus.Gauge:
		m := io_prometheus_client.Metric{}
		c.Write(&m)
		if *m.Gauge.Value != float64(-5) {
			t.Errorf("Expected -5 as last observed value, but got %v.", *m.Gauge.Value)
		}
	default:
		t.Errorf("Unexpected type of metric: %v", reflect.TypeOf(c))
	}
}

func TestGaugeCumulative(t *testing.T) {
	regex := initGaugeRegex(t)
	gaugeCfg := newMetricConfig(t, &configuration.MetricConfig{
		Name:       "temperature",
		Value:      "{{.temperature}}",
		Cumulative: true,
	})
	gauge := NewGaugeMetric(gaugeCfg, regex, nil)

	gauge.ProcessMatch("Temperature in Berlin: 32")
	gauge.ProcessMatch("Temperature in Moscow: -5")

	switch c := gauge.Collector().(type) {
	case prometheus.Gauge:
		m := io_prometheus_client.Metric{}
		c.Write(&m)
		if *m.Gauge.Value != float64(27) {
			t.Errorf("Expected 27 as cumulative value, but got %v.", *m.Gauge.Value)
		}
	default:
		t.Errorf("Unexpected type of metric: %v", reflect.TypeOf(c))
	}
}

func TestGaugeVec(t *testing.T) {
	regex := initGaugeRegex(t)
	gaugeCfg := newMetricConfig(t, &configuration.MetricConfig{
		Name:  "temperature",
		Value: "{{.temperature}}",
		Labels: map[string]string{
			"city": "{{.city}}",
		},
	})
	gauge := NewGaugeMetric(gaugeCfg, regex, nil)

	gauge.ProcessMatch("Temperature in Berlin: 32")
	gauge.ProcessMatch("Temperature in Moscow: -5")
	gauge.ProcessMatch("Temperature in Berlin: 31")

	switch c := gauge.Collector().(type) {
	case *prometheus.GaugeVec:
		m := io_prometheus_client.Metric{}
		c.WithLabelValues("Berlin").Write(&m)
		if *m.Gauge.Value != float64(31) {
			t.Errorf("Expected 31 as last observed value in Berlin, but got %v.", *m.Gauge.Value)
		}
		c.WithLabelValues("Moscow").Write(&m)
		if *m.Gauge.Value != float64(-5) {
			t.Errorf("Expected -5 as last observed value in Moscow, but got %v.", *m.Gauge.Value)
		}
	default:
		t.Errorf("Unexpected type of metric: %v", reflect.TypeOf(c))
	}
}

func initGaugeRegex(t *testing.T) *oniguruma.Regex {
	patterns := loadPatternDir(t)
	regex, err := Compile("Temperature in %{WORD:city}: %{INT:temperature}", patterns)
	if err != nil {
		t.Error(err)
	}
	return regex
}

func newMetricConfig(t *testing.T, cfg *configuration.MetricConfig) *configuration.MetricConfig {
	err := cfg.InitTemplates()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
