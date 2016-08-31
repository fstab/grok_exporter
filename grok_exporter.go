package main

import (
	"flag"
	"fmt"
	"github.com/fstab/grok_exporter/exporter"
	"github.com/fstab/grok_exporter/tailer"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
	"os"
	"time"
)

var (
	printVersion = flag.Bool("version", false, "Print the grok_exporter version.")
	configPath   = flag.String("config", "", "Path to the config file. Try '-config ./example/config.yml' to get started.")
)

const (
	number_of_lines_matched_label = "matched"
	number_of_lines_ignored_label = "ignored"
)

func main() {
	flag.Parse()
	if *printVersion {
		fmt.Printf("%v\n", exporter.VersionString())
		return
	}
	cfg, err := loadConfig()
	exitOnError(err)
	patterns, err := initPatterns(cfg)
	exitOnError(err)
	libonig, err := exporter.InitOnigurumaLib()
	exitOnError(err)
	metrics, err := createMetrics(cfg, patterns, libonig)
	exitOnError(err)
	for _, m := range metrics {
		prometheus.MustRegister(m.Collector())
	}
	nLinesTotal, nMatchesByMetric, procTimeMicrosecondsByMetric, nErrorsByMetric := initSelfMonitoring(metrics)

	tail, err := startTailer(cfg)
	exitOnError(err)
	fmt.Printf("Starting server on %v://localhost:%v/metrics\n", cfg.Server.Protocol, cfg.Server.Port)
	serverErrors := startServer(cfg, "/metrics", prometheus.Handler())

	for {
		select {
		case err := <-serverErrors:
			exitOnError(fmt.Errorf("Server error: %v", err.Error()))
		case err := <-tail.Errors():
			exitOnError(fmt.Errorf("Error reading log lines: %v", err.Error()))
		case line := <-tail.Lines():
			matched := false
			for _, metric := range metrics {
				start := time.Now()
				match, err := metric.Process(line)
				if err != nil {
					fmt.Fprintf(os.Stderr, "WARNING: Skipping log line: %v\n", err.Error())
					fmt.Fprintf(os.Stderr, "%v\n", line)
					nErrorsByMetric.WithLabelValues(metric.Name()).Inc()
				}
				if match {
					nMatchesByMetric.WithLabelValues(metric.Name()).Inc()
					procTimeMicrosecondsByMetric.WithLabelValues(metric.Name()).Add(float64(time.Since(start).Nanoseconds() / int64(1000)))
					matched = true
				}
			}
			if matched {
				nLinesTotal.WithLabelValues(number_of_lines_matched_label).Inc()
			} else {
				nLinesTotal.WithLabelValues(number_of_lines_ignored_label).Inc()
			}
		}
	}
}

func exitOnError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err.Error())
		os.Exit(-1)
	}
}

func loadConfig() (*exporter.Config, error) {
	if *configPath == "" {
		return nil, fmt.Errorf("Usage: grok_exporter -config <path>")
	}
	return exporter.LoadConfigFile(*configPath)
}

func initPatterns(cfg *exporter.Config) (*exporter.Patterns, error) {
	patterns := exporter.InitPatterns()
	if len(cfg.Grok.PatternsDir) > 0 {
		err := patterns.AddDir(cfg.Grok.PatternsDir)
		if err != nil {
			return nil, err
		}
	}
	for _, pattern := range cfg.Grok.AdditionalPatterns {
		err := patterns.AddPattern(pattern)
		if err != nil {
			return nil, err
		}
	}
	return patterns, nil
}

func createMetrics(cfg *exporter.Config, patterns *exporter.Patterns, libonig *exporter.OnigurumaLib) ([]exporter.Metric, error) {
	result := make([]exporter.Metric, 0, len(*cfg.Metrics))
	for _, m := range *cfg.Metrics {
		regex, err := exporter.Compile(m.Match, patterns, libonig)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize metric %v: %v", m.Name, err.Error())
		}
		err = exporter.VerifyFieldNames(m, regex)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize metric %v: %v", m.Name, err.Error())
		}
		switch m.Type {
		case "counter":
			result = append(result, exporter.NewCounterMetric(m, regex))
		case "gauge":
			result = append(result, exporter.NewGaugeMetric(m, regex))
		case "histogram":
			result = append(result, exporter.NewHistogramMetric(m, regex))
		case "summary":
			result = append(result, exporter.NewSummaryMetric(m, regex))
		default:
			return nil, fmt.Errorf("Failed to initialize metrics: Metric type %v is not supported.", m.Type)
		}
	}
	return result, nil
}

func initSelfMonitoring(metrics []exporter.Metric) (*prometheus.CounterVec, *prometheus.CounterVec, *prometheus.CounterVec, *prometheus.CounterVec) {
	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "grok_exporter_build_info",
		Help: "A metric with a constant '1' value labeled by version, builddate, branch, revision, goversion, and platform on which grok_exporter was built.",
	}, []string{"version", "builddate", "branch", "revision", "goversion", "platform"})
	nLinesTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "grok_exporter_lines_total",
		Help: "Total number of log lines processed by grok_exporter.",
	}, []string{"status"})
	nMatchesByMetric := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "grok_exporter_lines_matching_total",
		Help: "Number of lines matched for each metric. Note that one line can be matched by multiple metrics.",
	}, []string{"metric"})
	procTimeMicrosecondsByMetric := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "grok_exporter_lines_processing_time_microseconds_total",
		Help: "Processing time in microseconds for each metric. Divide by grok_exporter_lines_matching_total to get the averge processing time for one log line.",
	}, []string{"metric"})
	nErrorsByMetric := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "grok_exporter_line_processing_errors_total",
		Help: "Number of errors for each metric. If this is > 0 there is an error in the configuration file. Check grok_exporter's console output.",
	}, []string{"metric"})

	prometheus.MustRegister(buildInfo)
	prometheus.MustRegister(nLinesTotal)
	prometheus.MustRegister(nMatchesByMetric)
	prometheus.MustRegister(procTimeMicrosecondsByMetric)
	prometheus.MustRegister(nErrorsByMetric)

	buildInfo.WithLabelValues(exporter.Version, exporter.BuildDate, exporter.Branch, exporter.Revision, exporter.GoVersion, exporter.Platform).Set(1)
	// Initializing a value with zero makes the label appear. Otherwise the label is not shown until the first value is observed.
	nLinesTotal.WithLabelValues(number_of_lines_matched_label).Add(0)
	nLinesTotal.WithLabelValues(number_of_lines_ignored_label).Add(0)
	for _, metric := range metrics {
		nMatchesByMetric.WithLabelValues(metric.Name()).Add(0)
		procTimeMicrosecondsByMetric.WithLabelValues(metric.Name()).Add(0)
		nErrorsByMetric.WithLabelValues(metric.Name()).Add(0)
	}
	return nLinesTotal, nMatchesByMetric, procTimeMicrosecondsByMetric, nErrorsByMetric
}

func startServer(cfg *exporter.Config, path string, handler http.Handler) chan error {
	serverErrors := make(chan error)
	go func() {
		switch {
		case cfg.Server.Protocol == "http":
			serverErrors <- exporter.RunHttpServer(cfg.Server.Port, path, handler)
		case cfg.Server.Protocol == "https":
			if cfg.Server.Cert != "" && cfg.Server.Key != "" {
				serverErrors <- exporter.RunHttpsServer(cfg.Server.Port, cfg.Server.Cert, cfg.Server.Key, path, handler)
			} else {
				serverErrors <- exporter.RunHttpsServerWithDefaultKeys(cfg.Server.Port, path, handler)
			}
		default:
			// This cannot happen, because cfg.validate() makes sure that protocol is either http or https.
			serverErrors <- fmt.Errorf("Configuration error: Invalid 'server.protocol': '%v'. Expecting 'http' or 'https'.", cfg.Server.Protocol)
		}
	}()
	return serverErrors
}

func startTailer(cfg *exporter.Config) (tailer.Tailer, error) {
	var tail tailer.Tailer
	switch {
	case cfg.Input.Type == "file":
		tail = tailer.RunFileTailer(cfg.Input.Path, cfg.Input.Readall, nil)
	case cfg.Input.Type == "stdin":
		tail = tailer.RunStdinTailer()
	default:
		return nil, fmt.Errorf("Config error: Input type '%v' unknown.", cfg.Input.Type)
	}
	return exporter.BufferedTailerWithMetrics(tail), nil
}
