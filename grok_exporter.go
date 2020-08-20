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

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fstab/grok_exporter/config"
	"github.com/fstab/grok_exporter/config/v3"
	"github.com/fstab/grok_exporter/exporter"
	"github.com/fstab/grok_exporter/oniguruma"
	"github.com/fstab/grok_exporter/selfmonitoring"
	"github.com/fstab/grok_exporter/tailer"
	"github.com/fstab/grok_exporter/tailer/fswatcher"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

var (
	printVersion           = flag.Bool("version", false, "Print the grok_exporter version.")
	configPath             = flag.String("config", "", "Path to the config file. Try '-config ./example/config.yml' to get started.")
	showConfig             = flag.Bool("showconfig", false, "Print the current configuration to the console. Example: 'grok_exporter -showconfig -config ./example/config.yml'")
	disableExporterMetrics = flag.Bool("disable-exporter-metrics", false, "If this flag is set, the metrics about the exporter itself (go_*, process_*, promhttp_*) will be excluded from /metrics")
)

var (
	logfile = "logfile"
	extra   = "extra"
)

/*
const (
	number_of_lines_matched_label = "matched"
	number_of_lines_ignored_label = "ignored"
)
*/

var additionalFieldDefinitions = map[string]string{
	logfile: "full path of the log file",
	extra:   "full json log object",
}

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
	registry := prometheus.NewRegistry()
	if !*disableExporterMetrics {
		// init like the default registry, see client_golang/prometheus/registry.go init()
		registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
		registry.MustRegister(prometheus.NewGoCollector())
	}
	patterns, err := initPatterns(cfg)
	exitOnError(err)
	metrics, err := createMetrics(cfg, patterns)
	exitOnError(err)
	for _, m := range metrics {
		registry.MustRegister(m.Collector())
	}
	logger := logrus.New()
	logger.Level = logrus.WarnLevel
	selfMonitoring := selfmonitoring.Start(registry, metrics, cfg.Input.MaxLinesInBuffer > 0, logger)
	selfMonitoring.SetBuildInfo(exporter.Version, exporter.BuildDate, exporter.Branch, exporter.Revision, exporter.GoVersion, exporter.Platform)
	processorState := selfMonitoring.ForProcessor()
	//nLinesTotal, nMatchesByMetric, procTimeMicrosecondsByMetric, nErrorsByMetric := initSelfMonitoring(metrics, registry)

	tail, err := startTailer(cfg, selfMonitoring, logger)
	exitOnError(err)

	// gather up the handlers with which to start the webserver
	var httpHandlers []exporter.HttpServerPathHandler
	metricsHandler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	if !*disableExporterMetrics {
		metricsHandler = promhttp.InstrumentMetricHandler(registry, metricsHandler)
	}
	httpHandlers = append(httpHandlers, exporter.HttpServerPathHandler{
		Path:    cfg.Server.Path,
		Handler: metricsHandler,
	})
	if cfg.Input.Type == "webhook" {
		httpHandlers = append(httpHandlers, exporter.HttpServerPathHandler{
			Path:    cfg.Input.WebhookPath,
			Handler: tailer.WebhookHandler(),
		})
	}

	fmt.Print(startMsg(cfg, httpHandlers))
	serverErrors := startServer(cfg.Server, httpHandlers)

	retentionTicker := time.NewTicker(cfg.Global.RetentionCheckInterval)

	for {
		processorState.WaitingForLogLine()
		select {
		case err := <-serverErrors:
			exitOnError(fmt.Errorf("server error: %v", err.Error()))
		case err := <-tail.Errors():
			if err.Type() == fswatcher.FileNotFound || os.IsNotExist(err.Cause()) {
				exitOnError(fmt.Errorf("error reading log lines: %v: use 'fail_on_missing_logfile: false' in the input configuration if you want grok_exporter to start even though the logfile is missing", err))
			} else {
				exitOnError(fmt.Errorf("error reading log lines: %v", err.Error()))
			}
		case line := <-tail.Lines():
			matched := false
			for _, metric := range metrics {
				if !metric.PathMatches(line.File) {
					continue
				}
				processorState.ProcessingMatch(metric.Name())
				match, err := metric.ProcessMatch(line.Line, makeAdditionalFields(line))
				if err != nil {
					fmt.Fprintf(os.Stderr, "WARNING: skipping log line: %v\n", err.Error())
					fmt.Fprintf(os.Stderr, "%v\n", line.Line)
					selfMonitoring.ObserverLineProcessingError(metric.Name())
				} else if match != nil {
					selfMonitoring.ObserveLineMatched(metric.Name())
					matched = true
				}
				processorState.ProcessingDeleteMatch(metric.Name())
				_, err = metric.ProcessDeleteMatch(line.Line, makeAdditionalFields(line))
				if err != nil {
					fmt.Fprintf(os.Stderr, "WARNING: skipping log line: %v\n", err.Error())
					fmt.Fprintf(os.Stderr, "%v\n", line.Line)
					selfMonitoring.ObserverLineProcessingError(metric.Name())
				}
				// TODO: create metric to monitor number of matching delete_patterns
			}
			selfMonitoring.ObserveLineProcessed(matched)
		case <-retentionTicker.C:
			for _, metric := range metrics {
				err = metric.ProcessRetention()
				if err != nil {
					fmt.Fprintf(os.Stderr, "WARNING: error while processing retention on metric %v: %v", metric.Name(), err)
					selfMonitoring.ObserverLineProcessingError(metric.Name())
				}
			}
			// TODO: create metric to monitor number of metrics cleaned up via retention
		}
	}
}

func makeAdditionalFields(line *fswatcher.Line) map[string]interface{} {
	return map[string]interface{}{
		logfile: line.File,
		extra:   line.Extra,
	}
}

func startMsg(cfg *v3.Config, httpHandlers []exporter.HttpServerPathHandler) string {
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

func initPatterns(cfg *v3.Config) (*exporter.Patterns, error) {
	patterns := exporter.InitPatterns()
	for _, importedPatterns := range cfg.Imports {
		if importedPatterns.Type == "grok_patterns" {
			if len(importedPatterns.Dir) > 0 {
				err := patterns.AddDir(importedPatterns.Dir)
				if err != nil {
					return nil, err
				}
			} else if len(importedPatterns.File) > 0 {
				err := patterns.AddGlob(importedPatterns.File)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	for _, pattern := range cfg.GrokPatterns {
		err := patterns.AddPattern(pattern)
		if err != nil {
			return nil, err
		}
	}
	return patterns, nil
}

func createMetrics(cfg *v3.Config, patterns *exporter.Patterns) ([]exporter.Metric, error) {
	result := make([]exporter.Metric, 0, len(cfg.AllMetrics))
	for _, m := range cfg.AllMetrics {
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
		err = exporter.VerifyFieldNames(&m, regex, deleteRegex, additionalFieldDefinitions)
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

/*

func initSelfMonitoring(metrics []exporter.Metric, registry prometheus.Registerer) (*prometheus.CounterVec, *prometheus.CounterVec, *prometheus.CounterVec, *prometheus.CounterVec) {

	registry.MustRegister(buildInfo)
	registry.MustRegister(nLinesTotal)
	registry.MustRegister(nMatchesByMetric)
	registry.MustRegister(procTimeMicrosecondsByMetric)
	registry.MustRegister(nErrorsByMetric)

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

*/

func startServer(cfg v3.ServerConfig, httpHandlers []exporter.HttpServerPathHandler) chan error {
	serverErrors := make(chan error)
	go func() {
		switch {
		case cfg.Protocol == "http":
			serverErrors <- exporter.RunHttpServer(cfg.Host, cfg.Port, httpHandlers)
		case cfg.Protocol == "https":
			serverErrors <- exporter.RunHttpsServer(cfg, httpHandlers)
		default:
			// This cannot happen, because cfg.validate() makes sure that protocol is either http or https.
			serverErrors <- fmt.Errorf("Configuration error: Invalid 'server.protocol': '%v'. Expecting 'http' or 'https'.", cfg.Protocol)
		}
	}()
	return serverErrors
}

// -----------------------------------------
// TODO: Replace registry with perfmonitor and pass it to fswatcher as well as buffered tailer.
// -----------------------------------------

func startTailer(cfg *v3.Config, selfMonitoring selfmonitoring.SelfMonitoring, logger logrus.FieldLogger) (fswatcher.FileTailer, error) {
	var (
		tail fswatcher.FileTailer
		err  error
	)
	switch {
	case cfg.Input.Type == "file":
		if cfg.Input.PollInterval == 0 {
			tail, err = fswatcher.RunFileTailer(cfg.Input.Globs, cfg.Input.Readall, cfg.Input.FailOnMissingLogfile, selfMonitoring.ForFileSystemWatcher(), logger)
			if err != nil {
				return nil, err
			}
		} else {
			tail, err = fswatcher.RunPollingFileTailer(cfg.Input.Globs, cfg.Input.Readall, cfg.Input.FailOnMissingLogfile, cfg.Input.PollInterval, selfMonitoring.ForFileSystemWatcher(), logger)
			if err != nil {
				return nil, err
			}
		}
	case cfg.Input.Type == "stdin":
		tail = tailer.RunStdinTailer()
	case cfg.Input.Type == "webhook":
		tail = tailer.InitWebhookTailer(&cfg.Input)
	case cfg.Input.Type == "kafka":
		tail = tailer.RunKafkaTailer(&cfg.Input)
	default:
		return nil, fmt.Errorf("Config error: Input type '%v' unknown.", cfg.Input.Type)
	}
	return tailer.BufferedTailerWithMetrics(tail, cfg.Input.MaxLinesInBuffer, selfMonitoring.BufferLoadMetric(), selfMonitoring.ForLogLineBufferProducer(), selfMonitoring.ForLogLineBufferConsumer(), logger), nil
}
