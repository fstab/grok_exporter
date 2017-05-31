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

package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/fstab/grok_exporter/config"
	"github.com/fstab/grok_exporter/config/v2"
	"github.com/fstab/grok_exporter/exporter"
	"github.com/fstab/grok_exporter/tailer"
	"github.com/prometheus/client_golang/prometheus"
	//"github.com/prometheus/client_golang/prometheus/push"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var (
	printVersion = flag.Bool("version", false, "Print the grok_exporter version.")
	configPath   = flag.String("config", "", "Path to the config file. Try '-config ./example/config.yml' to get started.")
	showConfig   = flag.Bool("showconfig", false, "Print the current configuration to the console. Example: 'grok_exporter -showconfig -config ./exemple/config.yml'")
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
	fmt.Println(startMsg(cfg))
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
				match, delete_match, groupingKey, labelValues, err := metric.Process(line)
				//fmt.Println(fmt.Sprintf("[DEBUG] Process result: match: %s, delete_match: %s, groupingKey: %s, err: %s", match, delete_match, groupingKey, err))

				pushFlag := true
				if err != nil {
					fmt.Fprintf(os.Stderr, "WARNING: Skipping log line: %v\n", err.Error())
					fmt.Fprintf(os.Stderr, "%v\n", line)
					nErrorsByMetric.WithLabelValues(metric.Name()).Inc()
					pushFlag = false
				}
				if match {
					if metric.NeedPush() && pushFlag {
						err := pushMetric(metric, cfg.Global.PushgatewayAddr, groupingKey, labelValues)
						if err != nil {
							//fmt.Println(fmt.Sprintf("[DEBUG] Push error: %s", err))
							fmt.Errorf("Error pushing metric %v to pushgateway.", metric.Name())
						}
					}

					nMatchesByMetric.WithLabelValues(metric.Name()).Inc()
					procTimeMicrosecondsByMetric.WithLabelValues(metric.Name()).Add(float64(time.Since(start).Nanoseconds() / int64(1000)))
					matched = true
				}
				if delete_match {
					err := deleteMetric(metric, cfg.Global.PushgatewayAddr, groupingKey)
					if err != nil {
						fmt.Errorf("Error deleting metric %v from pushgateway.", metric.Name())
					}
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

func pushMetric(m exporter.Metric, pushUrl string, groupingKey map[string]string, labelValues []string) error {
	//fmt.Println(fmt.Sprintf("[DEBUG] Pushing metric %s with labels %s to pushgateway %s of job %s", m.Name(), groupingKey, pushUrl, m.JobName()))
	r := prometheus.NewRegistry()
	if err := r.Register(m.Collector()); err != nil {
		return err
	}
	err := doRequest(m.JobName(), groupingKey, pushUrl, r, "POST")
	if err != nil {
		return err
	}
	//remove metric from collector
	if m.MetricVec() != nil {
		m.MetricVec().DeleteLabelValues(labelValues...)
	}
	return nil
}

func deleteMetric(m exporter.Metric, deleteUrl string, groupingKey map[string]string) error {
	//fmt.Println(fmt.Sprintf("[DEBUG] Deleting metric %s with labels %s from pushgateway %s of job %s", m.Name(), groupingKey, deleteUrl, m.JobName()))
	return doRequest(m.JobName(), groupingKey, deleteUrl, nil, "DELETE")

}

func doRequest(job string, groupingKey map[string]string, targetUrl string, g prometheus.Gatherer, method string) error {
	if !strings.Contains(targetUrl, "://") {
		targetUrl = "http://" + targetUrl
	}
	if strings.HasSuffix(targetUrl, "/") {
		targetUrl = targetUrl[:len(targetUrl)-1]
	}

	if strings.Contains(job, "/") {
		return fmt.Errorf("job contains '/' : %s", job)
	}
	urlComponents := []string{url.QueryEscape(job)}
	for ln, lv := range groupingKey {
		if !model.LabelName(ln).IsValid() {
			return fmt.Errorf("groupingKey label has invalid name: %s", ln)
		}
		if strings.Contains(lv, "/") {
			return fmt.Errorf("value of groupingKey label %s contains '/': %s", ln, lv)
		}
		urlComponents = append(urlComponents, ln, lv)
	}

	targetUrl = fmt.Sprintf("%s/metrics/job/%s", targetUrl, strings.Join(urlComponents, "/"))

	buf := &bytes.Buffer{}
	enc := expfmt.NewEncoder(buf, expfmt.FmtProtoDelim)
	if g != nil {
		mfs, err := g.Gather()
		if err != nil {
			return err
		}
		for _, mf := range mfs {
			//ignore checking for pre-existing labels
			enc.Encode(mf)
		}
	}

	var request *http.Request
	var err error
	if method == "DELETE" {
		request, err = http.NewRequest(method, targetUrl, nil)
	} else {
		request, err = http.NewRequest(method, targetUrl, buf)
	}

	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", string(expfmt.FmtProtoDelim))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 202 {
		return fmt.Errorf("unexpected status code %d, method %s", response.StatusCode, method)
	}
	return nil
}

func startMsg(cfg *v2.Config) string {
	host := "localhost"
	if len(cfg.Server.Host) > 0 {
		host = cfg.Server.Host
	} else {
		hostname, err := os.Hostname()
		if err == nil {
			host = hostname
		}
	}
	return fmt.Sprintf("Starting server on %v://%v:%v/metrics\n", cfg.Server.Protocol, host, cfg.Server.Port)
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

func createMetrics(cfg *v2.Config, patterns *exporter.Patterns, libonig *exporter.OnigurumaLib) ([]exporter.Metric, error) {
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

		var delete_regex *exporter.OnigurumaRegexp = nil

		if len(m.DeleteMatch) != 0 {
			delete_regex, err = exporter.Compile(m.DeleteMatch, patterns, libonig)
			if err != nil {
				return nil, fmt.Errorf("failed to initialize metric %v: %v", m.Name, err.Error())
			}
			err = exporter.VerifyGroupingKeyField(m, delete_regex)
			if err != nil {
				return nil, fmt.Errorf("failed to initialize metric %v: %v", m.Name, err.Error())
			}
		}

		switch m.Type {
		case "counter":
			result = append(result, exporter.NewCounterMetric(m, regex, delete_regex))
		case "gauge":
			result = append(result, exporter.NewGaugeMetric(m, regex, delete_regex))
		case "histogram":
			result = append(result, exporter.NewHistogramMetric(m, regex, delete_regex))
		case "summary":
			result = append(result, exporter.NewSummaryMetric(m, regex, delete_regex))
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

func startServer(cfg *v2.Config, path string, handler http.Handler) chan error {
	serverErrors := make(chan error)
	go func() {
		switch {
		case cfg.Server.Protocol == "http":
			serverErrors <- exporter.RunHttpServer(cfg.Server.Host, cfg.Server.Port, path, handler)
		case cfg.Server.Protocol == "https":
			if cfg.Server.Cert != "" && cfg.Server.Key != "" {
				serverErrors <- exporter.RunHttpsServer(cfg.Server.Host, cfg.Server.Port, cfg.Server.Cert, cfg.Server.Key, path, handler)
			} else {
				serverErrors <- exporter.RunHttpsServerWithDefaultKeys(cfg.Server.Host, cfg.Server.Port, path, handler)
			}
		default:
			// This cannot happen, because cfg.validate() makes sure that protocol is either http or https.
			serverErrors <- fmt.Errorf("Configuration error: Invalid 'server.protocol': '%v'. Expecting 'http' or 'https'.", cfg.Server.Protocol)
		}
	}()
	return serverErrors
}

func startTailer(cfg *v2.Config) (tailer.Tailer, error) {
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
