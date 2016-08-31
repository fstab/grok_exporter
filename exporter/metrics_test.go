package exporter

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_model/go"
	"reflect"
	"testing"
)

func TestCounterVec(t *testing.T) {
	regex := initRegex(t)
	counterCfg := &MetricConfig{
		Name: "exim_rejected_rcpt_total",
		Labels: []Label{
			{
				GrokFieldName:   "message",
				PrometheusLabel: "error_message",
			},
		},
	}
	counter := NewCounterMetric(counterCfg, regex)
	counter.Process("some unrelated line")
	counter.Process("2016-04-26 10:19:57 H=(85.214.241.101) [36.224.138.227] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted")
	counter.Process("2016-04-26 12:31:39 H=(186-90-8-31.genericrev.cantv.net) [186.90.8.31] F=<Hans.Krause9@cantv.net> rejected RCPT <ug2seeng-admin@example.com>: Unrouteable address")
	counter.Process("2016-04-26 10:19:57 H=(85.214.241.101) [36.224.138.227] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted")

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
	regex := initRegex(t)
	counterCfg := &MetricConfig{
		Name: "exim_rejected_rcpt_total",
	}
	counter := NewCounterMetric(counterCfg, regex)

	counter.Process("some unrelated line")
	counter.Process("2016-04-26 10:19:57 H=(85.214.241.101) [36.224.138.227] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted")
	counter.Process("2016-04-26 12:31:39 H=(186-90-8-31.genericrev.cantv.net) [186.90.8.31] F=<Hans.Krause9@cantv.net> rejected RCPT <ug2seeng-admin@example.com>: Unrouteable address")
	counter.Process("2016-04-26 10:19:57 H=(85.214.241.101) [36.224.138.227] F=<z2007tw@yahoo.com.tw> rejected RCPT <alan.a168@msa.hinet.net>: relay not permitted")

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

func initRegex(t *testing.T) *OnigurumaRegexp {
	patterns := loadPatternDir(t)
	err := patterns.AddPattern("EXIM_MESSAGE [a-zA-Z ]*")
	if err != nil {
		t.Error(err)
	}
	libonig, err := InitOnigurumaLib()
	if err != nil {
		t.Error(err)
	}
	regex, err := Compile("%{EXIM_DATE} %{EXIM_REMOTE_HOST} F=<%{EMAILADDRESS}> rejected RCPT <%{EMAILADDRESS}>: %{EXIM_MESSAGE:message}", patterns, libonig)
	if err != nil {
		t.Error(err)
	}
	return regex
}
