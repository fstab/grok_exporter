package metrics

import "github.com/prometheus/client_golang/prometheus"

type Metric interface {
	Name() string
	Collector() prometheus.Collector
	Matches(ling string) bool
	Process(line string)
}
