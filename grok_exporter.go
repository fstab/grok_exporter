package main

import (
	"flag"
	"fmt"
	"github.com/fstab/grok_exporter/exporter"
	"github.com/fstab/grok_exporter/tailer"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
	"os"
)

var (
	printVersion = flag.Bool("version", false, "Print the grok_exporter version.")
	configPath   = flag.String("config", "", "Path to the config file. Try '-config ./example/config.yml' to get started.")
)

func main() {
	flag.Parse()
	if *printVersion {
		fmt.Printf("grok_exporter version %v build date %v.\n", exporter.VERSION, exporter.BUILD_DATE)
		return
	}
	cfg, err := loadConfig()
	exitOnError(err)
	patterns, err := initPatterns(cfg)
	exitOnError(err)
	metrics, err := createMetrics(cfg, patterns)
	exitOnError(err)
	for _, m := range metrics {
		prometheus.MustRegister(m.Collector())
	}
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
			for _, metric := range metrics {
				err := metric.Process(line)
				if err != nil {
					fmt.Fprintf(os.Stderr, "WARNING: Skipping log line: %v\n", err.Error())
					fmt.Fprintf(os.Stderr, "%v\n", line)
				}
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

func createMetrics(cfg *exporter.Config, patterns *exporter.Patterns) ([]exporter.Metric, error) {
	result := make([]exporter.Metric, 0, len(*cfg.Metrics))
	for _, m := range *cfg.Metrics {
		regex, err := exporter.Compile(m.Match, patterns)
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
