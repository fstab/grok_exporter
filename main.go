package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/fstab/grok_exporter/config"
	"github.com/fstab/grok_exporter/metrics"
	"github.com/fstab/grok_exporter/server"
	"github.com/hpcloud/tail"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
	"os"
)

var (
	configPath = flag.String("config", "", "Path to the config file. Try '-config ./example/config.yml' to get started.")
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(-1)
	}
	patterns, err := initPatterns(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(-1)
	}
	metrics, err := createMetrics(cfg, patterns)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(-1)
	}
	for _, m := range metrics {
		prometheus.MustRegister(m.Collector())
	}
	serverErrorChannel := startServer(cfg, "/metrics", prometheus.Handler())
	fmt.Printf("Starting server on %v://localhost:%v/metrics\n", cfg.Server.Protocol, cfg.Server.Port)
	err = processLogLines(cfg, metrics, serverErrorChannel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err.Error())
		os.Exit(-1)
	}
}

func loadConfig() (*config.Config, error) {
	flag.Parse()
	if *configPath == "" {
		return nil, fmt.Errorf("Usage: grok_exporter -config <path>")
	}
	return config.LoadConfigFile(*configPath)
}

func initPatterns(cfg *config.Config) (*Patterns, error) {
	patterns := InitPatterns()
	if len(cfg.Grok.Patterns) > 0 {
		err := patterns.AddDir(cfg.Grok.PatternsDir)
		if err != nil {
			return nil, err
		}
	}
	if len(cfg.Grok.Patterns) > 0 {
		for _, pattern := range cfg.Grok.Patterns {
			err := patterns.AddPattern(pattern)
			if err != nil {
				return nil, err
			}
		}
	}
	return patterns, nil
}

func createMetrics(cfg *config.Config, patterns *Patterns) ([]metrics.Metric, error) {
	result := make([]metrics.Metric, 0, len(*cfg.Metrics))
	for _, m := range *cfg.Metrics {
		regex, err := Compile(m.Match, patterns)
		if err != nil {
			return nil, err
		}
		switch {
		case m.Type == "counter":
			result = append(result, metrics.CreateGenericCounterVecMetric(m, regex))
		default:
			return nil, fmt.Errorf("Failed to initialize metrics: Metric type %v is not supported.\n", m.Type)
		}
	}
	return result, nil
}

func startServer(cfg *config.Config, path string, handler http.Handler) chan error {
	result := make(chan error)
	go func() {
		switch {
		case cfg.Server.Protocol == "http":
			result <- server.RunHttp(cfg.Server.Port, path, handler)
		case cfg.Server.Protocol == "https":
			if cfg.Server.Cert != "" && cfg.Server.Key != "" {
				result <- server.RunHttps(cfg.Server.Port, cfg.Server.Cert, cfg.Server.Key, path, handler)
			} else {
				result <- server.RunHttpsWithDefaultKeys(cfg.Server.Port, path, handler)
			}
		default:
			// This is a bug, because cfg.validate() should make sure that protocol is either http or https.
			result <- fmt.Errorf("Configuration error: Invalid 'server.protocol': '%v'. Expecting 'http' or 'https'.", cfg.Server.Protocol)
		}
	}()
	return result
}

func processLogLines(cfg *config.Config, metrics []metrics.Metric, serverErrorChannel chan error) error {
	switch {
	case cfg.Input.Type == "file":
		return processLogLinesFile(cfg, metrics, serverErrorChannel)
	case cfg.Input.Type == "stdin":
		return processLogLinesStdin(cfg, metrics, serverErrorChannel)
	default:
		return fmt.Errorf("Config error: Input type '%v' unknown.", cfg.Input.Type)
	}
}

func processLogLinesFile(cfg *config.Config, metrics []metrics.Metric, serverErrorChannel chan error) error {
	tailFile, err := tailFile(cfg.Input.Path, cfg.Input.Readall)
	if err != nil {
		return fmt.Errorf("Failed to read %v: %v", cfg.Input.Path, err.Error())
	}
	for {
		select {
		case err := <-serverErrorChannel:
			tailFile.Stop()
			return fmt.Errorf("Server error: %v", err.Error())
		case line := <-tailFile.Lines:
			if line.Err == nil {
				process(line.Text, metrics)
			}
		}
	}
}

func tailFile(logfile string, readall bool) (*tail.Tail, error) {
	whence := os.SEEK_END
	if readall {
		whence = os.SEEK_SET
	}
	return tail.TailFile(logfile, tail.Config{
		MustExist: true,                      // Fail early if the file does not exist
		ReOpen:    true,                      // Reopen recreated files (tail -F)
		Follow:    true,                      // Continue looking for new lines (tail -f)
		Logger:    tail.DiscardingLogger,     // Disable logging
		Location:  &tail.SeekInfo{0, whence}, // Start at the beginning or end of the file?
	})
}

func processLogLinesStdin(cfg *config.Config, metrics []metrics.Metric, serverErrorChannel chan error) error {
	c := stdinChan()
	for {
		select {
		case err := <-serverErrorChannel:
			// TODO: We should stop the STDIN reading goroutine here.
			return fmt.Errorf("Server error: %v", err.Error())
		case r := <-c:
			if r.err != nil {
				// TODO: We should stop the server here.
				return fmt.Errorf("Stopped reading on stdin: %v", r.err.Error())
			}
			process(r.line, metrics)
		}
	}
}

type stdinRead struct {
	line string
	err  error
}

func stdinChan() chan (*stdinRead) {
	out := make(chan (*stdinRead))
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			line, err := reader.ReadString('\n')
			out <- &stdinRead{
				line: line,
				err:  err,
			}
			if err != nil {
				close(out)
			}
		}
	}()
	return out
}

func process(line string, metrics []metrics.Metric) {
	for _, metric := range metrics {
		if metric.Matches(line) {
			metric.Process(line)
		}
	}
}
