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

package main

import (
	"flag"
	"fmt"
	"github.com/fstab/grok_exporter/config"
	"github.com/fstab/grok_exporter/config/v2"
	"github.com/fstab/grok_exporter/exporter"
	"github.com/fstab/grok_exporter/oniguruma"
	"github.com/fstab/grok_exporter/tailer"
	"github.com/fstab/grok_exporter/tailer/fswatcher"
	"github.com/fstab/grok_exporter/tailer/glob"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"os"
	"strings"
	"time"
)

var (
	printVersion = flag.Bool("version", false, "Print the grok_exporter version.")
	configPath   = flag.String("config", "", "Path to the config file. Try '-config ./example/config.yml' to get started.")
	showConfig   = flag.Bool("showconfig", false, "Print the current configuration to the console. Example: 'grok_exporter -showconfig -config ./example/config.yml'")
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
	validateCommandLineOrExit()
	cfg, warn, err := config.LoadConfigFile(*configPath)
	if len(warn) > 0 && !*showConfig {
		// warning is suppressed when '-showconfig' is used
		fmt.Fprintf(os.Stderr, "%v\n", warn)
	}
	exitOnError(err)
	if *showConfig {
		fmt.Printf("%v\n", cfg)
		return
	}
	patterns, err := initPatterns(cfg)
	exitOnError(err)
	metrics, err := createMetrics(cfg, patterns)
	exitOnError(err)
	for _, m := range metrics {
		prometheus.MustRegister(m.Collector())
	}
	nLinesTotal, nMatchesByMetric, procTimeMicrosecondsByMetric, nErrorsByMetric := initSelfMonitoring(metrics)

	tail, err := startTailer(cfg)
	exitOnError(err)

	// gather up the handlers with which to start the webserver
	httpHandlers := []exporter.HttpServerPathHandler{}
	httpHandlers = append(httpHandlers, exporter.HttpServerPathHandler{
		Path:    cfg.Server.Path,
		Handler: prometheus.Handler()})
	if cfg.Input.Type == "webhook" {
		httpHandlers = append(httpHandlers, exporter.HttpServerPathHandler{
			Path:    cfg.Input.WebhookPath,
			Handler: tailer.WebhookHandler()})
	}

	fmt.Print(startMsg(cfg, httpHandlers))
	serverErrors := startServer(cfg.Server, httpHandlers)

	retentionTicker := time.NewTicker(cfg.Global.RetentionCheckInterval)

	for {
		select {
		case err := <-serverErrors:
			exitOnError(fmt.Errorf("server error: %v", err.Error()))
		case err := <-tail.Errors():
			if os.IsNotExist(err.Cause()) {
				exitOnError(fmt.Errorf("error reading log lines: %v: use 'fail_on_missing_logfile: false' in the input configuration if you want grok_exporter to start even though the logfile is missing", err))
			} else {
				exitOnError(fmt.Errorf("error reading log lines: %v", err.Error()))
			}
		case line := <-tail.Lines():
			matched := false
			for _, metric := range metrics {
				start := time.Now()
				match, err := metric.ProcessMatch(line.Line)
				if err != nil {
					fmt.Fprintf(os.Stderr, "WARNING: skipping log line: %v\n", err.Error())
					fmt.Fprintf(os.Stderr, "%v\n", line.Line)
					nErrorsByMetric.WithLabelValues(metric.Name()).Inc()
				}
				if match != nil {
					nMatchesByMetric.WithLabelValues(metric.Name()).Inc()
					procTimeMicrosecondsByMetric.WithLabelValues(metric.Name()).Add(float64(time.Since(start).Nanoseconds() / int64(1000)))
					matched = true
				}
				_, err = metric.ProcessDeleteMatch(line.Line)
				if err != nil {
					fmt.Fprintf(os.Stderr, "WARNING: skipping log line: %v\n", err.Error())
					fmt.Fprintf(os.Stderr, "%v\n", line.Line)
					nErrorsByMetric.WithLabelValues(metric.Name()).Inc()
				}
				// TODO: create metric to monitor number of matching delete_patterns
			}
			if matched {
				nLinesTotal.WithLabelValues(number_of_lines_matched_label).Inc()
			} else {
				nLinesTotal.WithLabelValues(number_of_lines_ignored_label).Inc()
			}
		case <-retentionTicker.C:
			for _, metric := range metrics {
				err = metric.ProcessRetention()
				if err != nil {
					fmt.Fprintf(os.Stderr, "WARNING: error while processing retention on metric %v: %v", metric.Name(), err)
					nErrorsByMetric.WithLabelValues(metric.Name()).Inc()
				}
			}
			// TODO: create metric to monitor number of metrics cleaned up via retention
		}
	}
}

func startMsg(cfg *v2.Config, httpHandlers []exporter.HttpServerPathHandler) string {
	host := "localhost"
	if len(cfg.Server.Host) > 0 {
		host = cfg.Server.Host
	} else {
		hostname, err := os.Hostname()
		if err == nil {
			host = hostname
		}
	}

	var sb strings.Builder
	baseUrl := fmt.Sprintf("%v://%v:%v", cfg.Server.Protocol, host, cfg.Server.Port)
	sb.WriteString("Starting server on")
	for _, httpHandler := range httpHandlers {
		sb.WriteString(fmt.Sprintf(" %v%v", baseUrl, httpHandler.Path))
	}
	sb.WriteString("\n")
	return sb.String()
}

func exitOnError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err.Error())
		os.Exit(-1)
	}
}

func validateCommandLineOrExit() {
	if len(*configPath) == 0 {
		if *showConfig {
			fmt.Fprint(os.Stderr, "Usage: grok_exporter -showconfig -config <path>\n")
		} else {
			fmt.Fprint(os.Stderr, "Usage: grok_exporter -config <path>\n")
		}
		os.Exit(-1)
	}
}

func initPatterns(cfg *v2.Config) (*exporter.Patterns, error) {
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

func createMetrics(cfg *v2.Config, patterns *exporter.Patterns) ([]exporter.Metric, error) {
	result := make([]exporter.Metric, 0, len(cfg.Metrics))
	for _, m := range cfg.Metrics {
		var (
			regex, deleteRegex *oniguruma.Regex
			err                error
		)
		regex, err = exporter.Compile(m.Match, patterns)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize metric %v: %v", m.Name, err.Error())
		}
		if len(m.DeleteMatch) > 0 {
			deleteRegex, err = exporter.Compile(m.DeleteMatch, patterns)
			if err != nil {
				return nil, fmt.Errorf("failed to initialize metric %v: %v", m.Name, err.Error())
			}
		}
		err = exporter.VerifyFieldNames(&m, regex, deleteRegex)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize metric %v: %v", m.Name, err.Error())
		}
		switch m.Type {
		case "counter":
			result = append(result, exporter.NewCounterMetric(&m, regex, deleteRegex))
		case "gauge":
			result = append(result, exporter.NewGaugeMetric(&m, regex, deleteRegex))
		case "histogram":
			result = append(result, exporter.NewHistogramMetric(&m, regex, deleteRegex))
		case "summary":
			result = append(result, exporter.NewSummaryMetric(&m, regex, deleteRegex))
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

func startServer(cfg v2.ServerConfig, httpHandlers []exporter.HttpServerPathHandler) chan error {
	serverErrors := make(chan error)
	go func() {
		switch {
		case cfg.Protocol == "http":
			serverErrors <- exporter.RunHttpServer(cfg.Host, cfg.Port, httpHandlers)
		case cfg.Protocol == "https":
			if cfg.Cert != "" && cfg.Key != "" {
				serverErrors <- exporter.RunHttpsServer(cfg.Host, cfg.Port, cfg.Cert, cfg.Key, httpHandlers)
			} else {
				serverErrors <- exporter.RunHttpsServerWithDefaultKeys(cfg.Host, cfg.Port, httpHandlers)
			}
		default:
			// This cannot happen, because cfg.validate() makes sure that protocol is either http or https.
			serverErrors <- fmt.Errorf("Configuration error: Invalid 'server.protocol': '%v'. Expecting 'http' or 'https'.", cfg.Protocol)
		}
	}()
	return serverErrors
}

func startTailer(cfg *v2.Config) (fswatcher.FileTailer, error) {
	logger := logrus.New()
	logger.Level = logrus.WarnLevel
	var tail fswatcher.FileTailer
	g, err := glob.FromPath(cfg.Input.Path)
	if err != nil {
		return nil, err
	}
	switch {
	case cfg.Input.Type == "file":
		if cfg.Input.PollInterval == 0 {
			tail, err = fswatcher.RunFileTailer([]glob.Glob{g}, cfg.Input.Readall, cfg.Input.FailOnMissingLogfile, logger)
		} else {
			tail, err = fswatcher.RunPollingFileTailer([]glob.Glob{g}, cfg.Input.Readall, cfg.Input.FailOnMissingLogfile, cfg.Input.PollInterval, logger)
		}
	case cfg.Input.Type == "stdin":
		tail = tailer.RunStdinTailer()
	case cfg.Input.Type == "webhook":
		tail = tailer.InitWebhookTailer(&cfg.Input)
	default:
		return nil, fmt.Errorf("Config error: Input type '%v' unknown.", cfg.Input.Type)
	}
	bufferLoadMetric := exporter.NewBufferLoadMetric(logger, cfg.Input.MaxLinesInBuffer > 0)
	return tailer.BufferedTailerWithMetrics(tail, bufferLoadMetric, logger, cfg.Input.MaxLinesInBuffer), nil
}
