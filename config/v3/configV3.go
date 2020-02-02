// Copyright 2020 The grok_exporter Authors
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

package v3

import (
	"fmt"
	v2 "github.com/fstab/grok_exporter/config/v2"
	"github.com/fstab/grok_exporter/tailer/glob"
	"github.com/fstab/grok_exporter/template"
	"gopkg.in/yaml.v2"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultRetentionCheckInterval = 53 * time.Second
	inputTypeStdin                = "stdin"
	inputTypeFile                 = "file"
	inputTypeWebhook              = "webhook"
	importMetricsType             = "metrics"
	importPatternsType            = "grok_patterns"
)

func Unmarshal(config []byte) (*Config, error) {
	cfg := &Config{}
	err := yaml.Unmarshal(config, cfg)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration: %v. make sure to use 'single quotes' around strings with special characters (like match patterns or label templates), and make sure to use '-' only for lists (metrics) but not for maps (labels).", err.Error())
	}
	importedMetrics, err := importMetrics(cfg.Imports)
	if err != nil {
		return nil, err
	}
	for _, metric := range cfg.OrigMetrics {
		cfg.AllMetrics = append(cfg.AllMetrics, metric)
	}
	for _, metric := range importedMetrics {
		cfg.AllMetrics = append(cfg.AllMetrics, metric)
	}
	err = AddDefaultsAndValidate(cfg)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func Convert(v2cfg *v2.Config) (*Config, error) {
	v3cfg := convert(v2cfg)
	for _, metric := range v3cfg.OrigMetrics {
		v3cfg.AllMetrics = append(v3cfg.AllMetrics, metric)
	}
	err := AddDefaultsAndValidate(v3cfg)
	if err != nil {
		return nil, err
	}
	return v3cfg, nil
}

type Config struct {
	Global       GlobalConfig       `yaml:",omitempty"`
	Input        InputConfig        `yaml:",omitempty"`
	Imports      ImportsConfig      `yaml:",omitempty"`
	GrokPatterns GrokPatternsConfig `yaml:"grok_patterns,omitempty"`
	OrigMetrics  MetricsConfig      `yaml:"metrics,omitempty"` // not including imported config files
	AllMetrics   MetricsConfig      `yaml:"-"`                 // including metrics from imported config files
	Server       ServerConfig       `yaml:",omitempty"`
}

type GlobalConfig struct {
	ConfigVersion          int           `yaml:"config_version,omitempty"`
	RetentionCheckInterval time.Duration `yaml:"retention_check_interval,omitempty"` // implicitly parsed with time.ParseDuration()
}

type InputConfig struct {
	Type                       string `yaml:",omitempty"`
	PathsAndGlobs              `yaml:",inline"`
	FailOnMissingLogfileString string        `yaml:"fail_on_missing_logfile,omitempty"` // cannot use bool directly, because yaml.v2 doesn't support true as default value.
	FailOnMissingLogfile       bool          `yaml:"-"`
	Readall                    bool          `yaml:",omitempty"`
	PollInterval               time.Duration `yaml:"poll_interval,omitempty"` // implicitly parsed with time.ParseDuration()
	MaxLinesInBuffer           int           `yaml:"max_lines_in_buffer,omitempty"`
	WebhookPath                string        `yaml:"webhook_path,omitempty"`
	WebhookFormat              string        `yaml:"webhook_format,omitempty"`
	WebhookJsonSelector        string        `yaml:"webhook_json_selector,omitempty"`
	WebhookTextBulkSeparator   string        `yaml:"webhook_text_bulk_separator,omitempty"`
}

type GrokPatternsConfig []string

type PathsAndGlobs struct {
	Path  string      `yaml:",omitempty"`
	Paths []string    `yaml:",omitempty"`
	Globs []glob.Glob `yaml:"-"`
}

type MetricConfig struct {
	Type                 string `yaml:",omitempty"`
	Name                 string `yaml:",omitempty"`
	Help                 string `yaml:",omitempty"`
	PathsAndGlobs        `yaml:",inline"`
	Match                string              `yaml:",omitempty"`
	Retention            time.Duration       `yaml:",omitempty"` // implicitly parsed with time.ParseDuration()
	Value                string              `yaml:",omitempty"`
	Cumulative           bool                `yaml:",omitempty"`
	Buckets              []float64           `yaml:",flow,omitempty"`
	Quantiles            map[float64]float64 `yaml:",flow,omitempty"`
	Labels               map[string]string   `yaml:",omitempty"`
	LabelTemplates       []template.Template `yaml:"-"` // parsed version of Labels, will not be serialized to yaml.
	ValueTemplate        template.Template   `yaml:"-"` // parsed version of Value, will not be serialized to yaml.
	DeleteMatch          string              `yaml:"delete_match,omitempty"`
	DeleteLabels         map[string]string   `yaml:"delete_labels,omitempty"` // TODO: Make sure that DeleteMatch is not nil if DeleteLabels are used.
	DeleteLabelTemplates []template.Template `yaml:"-"`                       // parsed version of DeleteLabels, will not be serialized to yaml.
}

type MetricsConfig []MetricConfig

type ImportsConfig []ImportConfig

type ImportConfig struct {
	Type     string        `yaml:",omitempty"`
	Dir      string        `yaml:",omitempty"`
	File     string        `yaml:",omitempty"`
	Defaults DefaultConfig `yaml:",omitempty"`
}

type DefaultConfig struct {
	PathsAndGlobs `yaml:",inline"`
	Retention     time.Duration       `yaml:",omitempty"` // implicitly parsed with time.ParseDuration()
	Buckets       []float64           `yaml:",flow,omitempty"`
	Quantiles     map[float64]float64 `yaml:",flow,omitempty"`
	Labels        map[string]string   `yaml:",omitempty"`
}

type ServerConfig struct {
	Protocol string `yaml:",omitempty"`
	Host     string `yaml:",omitempty"`
	Port     int    `yaml:",omitempty"`
	Path     string `yaml:",omitempty"`
	Cert     string `yaml:",omitempty"`
	Key      string `yaml:",omitempty"`
}

func (cfg *Config) addDefaults() {
	cfg.Global.addDefaults()
	cfg.Input.addDefaults()
	cfg.GrokPatterns.addDefaults()
	if cfg.AllMetrics != nil {
		cfg.AllMetrics.addDefaults()
	}
	cfg.Server.addDefaults()
}

func (c *GlobalConfig) addDefaults() {
	if c.ConfigVersion == 0 {
		c.ConfigVersion = 2
	}
	if c.RetentionCheckInterval == 0 {
		c.RetentionCheckInterval = defaultRetentionCheckInterval
	}
}

func (c *InputConfig) addDefaults() {
	if c.Type == "" {
		c.Type = inputTypeStdin
	}
	if c.Type == inputTypeFile && len(c.FailOnMissingLogfileString) == 0 {
		c.FailOnMissingLogfileString = "true"
	}
	if c.Type == inputTypeWebhook {
		if len(c.WebhookPath) == 0 {
			c.WebhookPath = "/webhook"
		}
		if len(c.WebhookFormat) == 0 {
			c.WebhookFormat = "text_single"
		}
		if len(c.WebhookJsonSelector) == 0 {
			c.WebhookJsonSelector = ".message"
		}
		if len(c.WebhookTextBulkSeparator) == 0 {
			c.WebhookTextBulkSeparator = "\n\n"
		}
	}
}

func (c *GrokPatternsConfig) addDefaults() {}

func (c *MetricsConfig) addDefaults() {}

func (c *ServerConfig) addDefaults() {
	if c.Protocol == "" {
		c.Protocol = "http"
	}
	if c.Port == 0 {
		c.Port = 9144
	}
	if c.Path == "" {
		c.Path = "/metrics"
	}
}

func (cfg *Config) validate() error {
	err := cfg.Input.validate()
	if err != nil {
		return err
	}
	err = cfg.GrokPatterns.validate()
	if err != nil {
		return err
	}
	err = cfg.AllMetrics.validate()
	if err != nil {
		return err
	}
	err = cfg.Server.validate()
	if err != nil {
		return err
	}
	return nil
}

func validateGlobs(p *PathsAndGlobs, optional bool, prefix string) error {
	if !optional && len(p.Path) == 0 && len(p.Paths) == 0 {
		return fmt.Errorf("%v: one of 'path' or 'paths' is required", prefix)
	}
	if len(p.Path) > 0 && len(p.Paths) > 0 {
		return fmt.Errorf("%v: use either 'path' or 'paths' but not both", prefix)
	}
	if len(p.Path) > 0 {
		parsedGlob, err := glob.Parse(p.Path)
		if err != nil {
			return fmt.Errorf("%v: %v", prefix, err)
		}
		p.Globs = []glob.Glob{parsedGlob}
	}
	if len(p.Paths) > 0 {
		p.Globs = make([]glob.Glob, 0, len(p.Paths))
		for _, path := range p.Paths {
			parsedGlob, err := glob.Parse(path)
			if err != nil {
				return fmt.Errorf("%v: %v", prefix, err)
			}
			p.Globs = append(p.Globs, parsedGlob)
		}
	}
	return nil
}

func (c *InputConfig) validate() error {
	var err error
	switch {
	case c.Type == inputTypeStdin:
		if len(c.Path) > 0 {
			return fmt.Errorf("invalid input configuration: cannot use 'input.path' when 'input.type' is stdin")
		}
		if len(c.Paths) > 0 {
			return fmt.Errorf("invalid input configuration: cannot use 'input.paths' when 'input.type' is stdin")
		}
		if c.Readall {
			return fmt.Errorf("invalid input configuration: cannot use 'input.readall' when 'input.type' is stdin")
		}
		if c.PollInterval > 0 {
			return fmt.Errorf("invalid input configuration: cannot use 'input.poll_interval' when 'input.type' is stdin")
		}
	case c.Type == inputTypeFile:
		err = validateGlobs(&c.PathsAndGlobs, false, "invalid input configuration")
		if err != nil {
			return err
		}
		if len(c.FailOnMissingLogfileString) > 0 {
			c.FailOnMissingLogfile, err = strconv.ParseBool(c.FailOnMissingLogfileString)
			if err != nil {
				return fmt.Errorf("invalid input configuration: '%v' is not a valid boolean value in 'input.fail_on_missing_logfile'", c.FailOnMissingLogfileString)
			}
		}
	case c.Type == inputTypeWebhook:
		if c.Path != "" {
			return fmt.Errorf("invalid input configuration: cannot use 'input.path' when 'input.type' is %v", inputTypeWebhook)
		}
		if len(c.Paths) > 0 {
			return fmt.Errorf("invalid input configuration: cannot use 'input.paths' when 'input.type' is %v", inputTypeWebhook)
		}
		if c.Readall {
			return fmt.Errorf("invalid input configuration: cannot use 'input.readall' when 'input.type' is %v", inputTypeWebhook)
		}
		if c.PollInterval > 0 {
			return fmt.Errorf("invalid input configuration: cannot use 'input.poll_interval' when 'input.type' is %v", inputTypeWebhook)
		}
		if c.WebhookPath == "" {
			return fmt.Errorf("invalid input configuration: 'input.webhook_path' is required for input type \"webhook\"")
		} else if c.WebhookPath[0] != '/' {
			return fmt.Errorf("invalid input configuration: 'input.webhook_path' must start with \"/\"")
		}
		if c.WebhookFormat != "text_single" && c.WebhookFormat != "text_bulk" && c.WebhookFormat != "json_single" && c.WebhookFormat != "json_bulk" {
			return fmt.Errorf("invalid input configuration: 'input.webhook_format' must be \"text_single|text_bulk|json_single|json_bulk\"")
		}
		if c.WebhookJsonSelector == "" {
			return fmt.Errorf("invalid input configuration: 'input.webhook_json_selector' is required for input type \"webhook\"")
		} else if c.WebhookJsonSelector[0] != '.' {
			return fmt.Errorf("invalid input configuration: 'input.webhook_json_selector' must start with \".\"")
		}
		if c.WebhookFormat == "text_bulk" && c.WebhookTextBulkSeparator == "" {
			return fmt.Errorf("invalid input configuration: 'input.webhook_text_bulk_separator' is required for input type \"webhook\" and webhook_format \"text_bulk\"")
		}
	default:
		return fmt.Errorf("unsupported 'input.type': %v", c.Type)
	}
	return nil
}

func (c ImportConfig) validate() error {
	switch c.Type {
	case importPatternsType:
		for _, field := range []struct {
			name    string
			present bool
		}{
			{"path", len(c.Defaults.Path) > 0},
			{"paths", len(c.Defaults.Paths) > 0},
			{"retention", c.Defaults.Retention != 0},
			{"buckets", len(c.Defaults.Buckets) > 0},
			{"quantiles", len(c.Defaults.Quantiles) > 0},
			{"labels", len(c.Defaults.Labels) > 0},
		} {
			if field.present {
				return fmt.Errorf("invalid imports configuration: cannot use imports.%v for imports.type=%v", field.name, c.Type)
			}
		}
	case importMetricsType:
		if len(c.Defaults.Path) > 0 && len(c.Defaults.Paths) > 0 {
			return fmt.Errorf("invalid imports configuration: use either imports.defaults.path or imports.defaults.paths, but not both")
		}
		// TODO: Validate the other fields
	default:
		return fmt.Errorf("invalid imports configuration: unsupported imports.type: %v", c.Type)
	}
	if len(c.Dir) > 0 && len(c.File) > 0 {
		return fmt.Errorf("invalid imports configuration: either use imports.dir or imports.file, but not both")
	}
	if len(c.Dir) == 0 && len(c.File) == 0 {
		return fmt.Errorf("invalid imports configuration: one of imports.dir or imports.file must be present")
	}
	return nil
}

func (c *GrokPatternsConfig) validate() error {
	return nil
}

func (c *MetricsConfig) validate() error {
	if len(*c) == 0 {
		return fmt.Errorf("Invalid metrics configuration: 'metrics' must not be empty.")
	}
	metricNames := make(map[string]bool)
	for i := range *c {
		metric := &(*c)[i] // validate modifies the metric, therefore we must use it by reference here.
		err := metric.validate()
		if err != nil {
			return err
		}
		_, exists := metricNames[metric.Name]
		if exists {
			return fmt.Errorf("Invalid metric configuration: metric '%v' defined twice.", metric.Name)
		}
		metricNames[metric.Name] = true

		if len(metric.Path) > 0 && len(metric.Paths) > 0 {
			return fmt.Errorf("invalid metric configuration: metric %v defines both path and paths, you should use either one or the other", metric.Name)
		}
		if len(metric.Path) > 0 {
			metric.Paths = []string{metric.Path}
			metric.Path = ""
		}
	}
	return nil
}

func (c *MetricConfig) validate() error {
	switch {
	case c.Type == "":
		return fmt.Errorf("Invalid metric configuration: 'metrics.type' must not be empty.")
	case c.Name == "":
		return fmt.Errorf("Invalid metric configuration: 'metrics.name' must not be empty.")
	case c.Help == "":
		return fmt.Errorf("Invalid metric configuration: 'metrics.help' must not be empty.")
	case c.Match == "":
		return fmt.Errorf("Invalid metric configuration: 'metrics.match' must not be empty.")
	}
	err := validateGlobs(&c.PathsAndGlobs, true, fmt.Sprintf("invalid metric configuration: %v", c.Name))
	if err != nil {
		return err
	}
	var hasValue, cumulativeAllowed, bucketsAllowed, quantilesAllowed bool
	switch c.Type {
	case "counter":
		hasValue, cumulativeAllowed, bucketsAllowed, quantilesAllowed = false, false, false, false
	case "gauge":
		hasValue, cumulativeAllowed, bucketsAllowed, quantilesAllowed = true, true, false, false
	case "histogram":
		hasValue, cumulativeAllowed, bucketsAllowed, quantilesAllowed = true, false, true, false
	case "summary":
		hasValue, cumulativeAllowed, bucketsAllowed, quantilesAllowed = true, false, false, true
	default:
		return fmt.Errorf("Invalid 'metrics.type': '%v'. We currently only support 'counter' and 'gauge'.", c.Type)
	}
	switch {
	case hasValue && len(c.Value) == 0:
		return fmt.Errorf("Invalid metric configuration: 'metrics.value' must not be empty for %v metrics.", c.Type)
	case !hasValue && len(c.Value) > 0:
		return fmt.Errorf("Invalid metric configuration: 'metrics.value' cannot be used for %v metrics.", c.Type)
	case !cumulativeAllowed && c.Cumulative:
		return fmt.Errorf("Invalid metric configuration: 'metrics.cumulative' cannot be used for %v metrics.", c.Type)
	case !bucketsAllowed && len(c.Buckets) > 0:
		return fmt.Errorf("Invalid metric configuration: 'metrics.buckets' cannot be used for %v metrics.", c.Type)
	case !quantilesAllowed && len(c.Quantiles) > 0:
		return fmt.Errorf("Invalid metric configuration: 'metrics.buckets' cannot be used for %v metrics.", c.Type)
	}
	if len(c.DeleteMatch) > 0 && len(c.Labels) == 0 {
		return fmt.Errorf("Invalid metric configuration: 'metrics.delete_match' is only supported for metrics with labels.")
	}
	if len(c.DeleteMatch) == 0 && len(c.DeleteLabelTemplates) > 0 {
		return fmt.Errorf("Invalid metric configuration: 'metrics.delete_labels' can only be used when 'metrics.delete_match' is present.")
	}
	if c.Retention > 0 && len(c.Labels) == 0 {
		return fmt.Errorf("Invalid metric configuration: 'metrics.retention' is only supported for metrics with labels.")
	}
	for _, deleteLabelTemplate := range c.DeleteLabelTemplates {
		found := false
		for _, labelTemplate := range c.LabelTemplates {
			if deleteLabelTemplate.Name() == labelTemplate.Name() {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("Invalid metric configuration: '%v' cannot be used as a delete_label, because the metric does not have a label named '%v'.", deleteLabelTemplate.Name(), deleteLabelTemplate.Name())
		}
	}
	// InitTemplates() validates that labels/delete_labels/value are present as grok_fields in the grok pattern.
	return nil
}

func (c *ServerConfig) validate() error {
	switch {
	case c.Protocol != "https" && c.Protocol != "http":
		return fmt.Errorf("Invalid 'server.protocol': '%v'. Expecting 'http' or 'https'.", c.Protocol)
	case c.Port <= 0:
		return fmt.Errorf("Invalid 'server.port': '%v'.", c.Port)
	case !strings.HasPrefix(c.Path, "/"):
		return fmt.Errorf("Invalid server configuration: 'server.path' must start with '/'.")
	case c.Protocol == "https":
		if c.Cert != "" && c.Key == "" {
			return fmt.Errorf("Invalid server configuration: 'server.cert' must not be specified without 'server.key'")
		}
		if c.Cert == "" && c.Key != "" {
			return fmt.Errorf("Invalid server configuration: 'server.key' must not be specified without 'server.cert'")
		}
	case c.Protocol == "http":
		if c.Cert != "" || c.Key != "" {
			return fmt.Errorf("Invalid server configuration: 'server.cert' and 'server.key' can only be configured for protocol 'https'.")
		}
	}
	return nil
}

// Made this public so it can be called when converting config v1 to config v2.
func AddDefaultsAndValidate(cfg *Config) error {
	var err error
	cfg.addDefaults()
	for i := range []MetricConfig(cfg.AllMetrics) {
		err = cfg.AllMetrics[i].InitTemplates()
		if err != nil {
			return err
		}
	}
	return cfg.validate()
}

// Made this public so MetricConfig can be initialized in tests.
func (metric *MetricConfig) InitTemplates() error {
	var (
		err   error
		tmplt template.Template
		msg   = "invalid configuration: failed to read metric %v: error parsing %v template: %v: " +
			"don't forget to put a . (dot) in front of grok fields, otherwise it will be interpreted as a function."
	)
	for _, t := range []struct {
		src  map[string]string    // label / template string as read from the config file
		dest *[]template.Template // parsed template used internally in grok_exporter
	}{
		{
			src:  metric.Labels,
			dest: &(metric.LabelTemplates),
		},
		{
			src:  metric.DeleteLabels,
			dest: &(metric.DeleteLabelTemplates),
		},
	} {
		*t.dest = make([]template.Template, 0, len(t.src))
		for name, templateString := range t.src {
			tmplt, err = template.New(name, templateString)
			if err != nil {
				return fmt.Errorf(msg, fmt.Sprintf("label %v", metric.Name), name, err.Error())
			}
			*t.dest = append(*t.dest, tmplt)
		}
	}
	if len(metric.Value) > 0 {
		metric.ValueTemplate, err = template.New("__value__", metric.Value)
		if err != nil {
			return fmt.Errorf(msg, "value", metric.Name, err.Error())
		}
	}
	return nil
}

// YAML representation, does not include default values.
func (cfg *Config) String() string {
	stripped := cfg.copy()
	if stripped.Global.RetentionCheckInterval == defaultRetentionCheckInterval {
		stripped.Global.RetentionCheckInterval = 0
	}
	if stripped.Input.FailOnMissingLogfileString == "true" {
		stripped.Input.FailOnMissingLogfileString = ""
	}
	if stripped.Server.Path == "/metrics" {
		stripped.Server.Path = ""
	}
	if len(stripped.Input.Paths) == 1 {
		stripped.Input.Path = stripped.Input.Paths[0]
		stripped.Input.Paths = nil
	}
	for i := range stripped.OrigMetrics {
		if len(stripped.OrigMetrics[i].Paths) == 1 {
			stripped.OrigMetrics[i].Path = stripped.OrigMetrics[i].Paths[i]
			stripped.OrigMetrics[i].Paths = nil
		}
	}
	return stripped.marshalToString()
}

func (cfg *Config) copy() *Config {
	var result Config
	err := yaml.Unmarshal([]byte(cfg.marshalToString()), &result)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "unexpected fatal error: failed to unmarshal config: %v", err)
	}
	return &result
}

func (cfg *Config) marshalToString() string {
	var newlineEscape = "___GROK_EXPORTER_NEWLINE_ESCAPE___"
	cfg.Input.WebhookTextBulkSeparator = strings.Replace(cfg.Input.WebhookTextBulkSeparator, "\n", newlineEscape, -1)
	out, err := yaml.Marshal(cfg)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "unexpected fatal error: failed to marshal config: %v", err)
		os.Exit(1)
	}
	result := string(out)
	// Pretend fail_on_missing_logfile is a boolean, remove quotes
	result = strings.Replace(result, "fail_on_missing_logfile: \"false\"", "fail_on_missing_logfile: false", -1)
	result = strings.Replace(result, "fail_on_missing_logfile: \"true\"", "fail_on_missing_logfile: true", -1)
	// write newlines like \n instead of actual newlines
	result = strings.Replace(result, newlineEscape, "\\n", -1)
	return result
}
