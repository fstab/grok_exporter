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

package v2

import (
	"fmt"
	"github.com/fstab/grok_exporter/template"
	"gopkg.in/yaml.v2"
	"strconv"
	"strings"
	"time"
)

const (
	defaultRetentionCheckInterval = 53 * time.Second
	inputTypeStdin                = "stdin"
	inputTypeFile                 = "file"
	inputTypeWebhook              = "webhook"
)

func Unmarshal(config []byte) (*Config, error) {
	cfg := &Config{}
	err := yaml.Unmarshal(config, cfg)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration: %v. make sure to use 'single quotes' around strings with special characters (like match patterns or label templates), and make sure to use '-' only for lists (metrics) but not for maps (labels).", err.Error())
	}
	err = AddDefaultsAndValidate(cfg)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

type Config struct {
	Global  GlobalConfig  `yaml:",omitempty"`
	Input   InputConfig   `yaml:",omitempty"`
	Grok    GrokConfig    `yaml:",omitempty"`
	Metrics MetricsConfig `yaml:",omitempty"`
	Server  ServerConfig  `yaml:",omitempty"`
}

type GlobalConfig struct {
	ConfigVersion          int           `yaml:"config_version,omitempty"`
	RetentionCheckInterval time.Duration `yaml:"retention_check_interval,omitempty"` // implicitly parsed with time.ParseDuration()
}

type InputConfig struct {
	Type                       string        `yaml:",omitempty"`
	Path                       string        `yaml:",omitempty"`
	FailOnMissingLogfileString string        `yaml:"fail_on_missing_logfile,omitempty"` // cannot use bool directly, because yaml.v2 doesn't support true as default value.
	FailOnMissingLogfile       bool          `yaml:"-"`
	Readall                    bool          `yaml:",omitempty"`
	PollIntervalSeconds        string        `yaml:"poll_interval_seconds,omitempty"` // TODO: Use time.Duration directly
	PollInterval               time.Duration `yaml:"-"`                               // parsed version of PollIntervalSeconds
	MaxLinesInBuffer           int           `yaml:"max_lines_in_buffer,omitempty"`
	WebhookPath                string        `yaml:"webhook_path,omitempty"`
	WebhookFormat              string        `yaml:"webhook_format,omitempty"`
	WebhookJsonSelector        string        `yaml:"webhook_json_selector,omitempty"`
	WebhookTextBulkSeparator   string        `yaml:"webhook_text_bulk_separator,omitempty"`
}

type GrokConfig struct {
	PatternsDir        string   `yaml:"patterns_dir,omitempty"`
	AdditionalPatterns []string `yaml:"additional_patterns,omitempty"`
}

type MetricConfig struct {
	Type                 string              `yaml:",omitempty"`
	Name                 string              `yaml:",omitempty"`
	Help                 string              `yaml:",omitempty"`
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
	cfg.Grok.addDefaults()
	if cfg.Metrics == nil {
		cfg.Metrics = MetricsConfig(make([]MetricConfig, 0))
	}
	cfg.Metrics.addDefaults()
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

func (c *GrokConfig) addDefaults() {}

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
	err = cfg.Grok.validate()
	if err != nil {
		return err
	}
	err = cfg.Metrics.validate()
	if err != nil {
		return err
	}
	err = cfg.Server.validate()
	if err != nil {
		return err
	}
	return nil
}

func (c *InputConfig) validate() error {
	var err error
	switch {
	case c.Type == inputTypeStdin:
		if c.Path != "" {
			return fmt.Errorf("invalid input configuration: cannot use 'input.path' when 'input.type' is stdin")
		}
		if c.Readall {
			return fmt.Errorf("invalid input configuration: cannot use 'input.readall' when 'input.type' is stdin")
		}
		if c.PollIntervalSeconds != "" {
			return fmt.Errorf("invalid input configuration: cannot use 'input.poll_interval_seconds' when 'input.type' is stdin")
		}
	case c.Type == inputTypeFile:
		if c.Path == "" {
			return fmt.Errorf("invalid input configuration: 'input.path' is required for input type \"file\"")
		}
		if len(c.PollIntervalSeconds) > 0 { // TODO: Use duration directly, as with other durations in the config file
			nSeconds, err := strconv.Atoi(c.PollIntervalSeconds)
			if err != nil {
				return fmt.Errorf("invalid input configuration: '%v' is not a valid number in 'input.poll_interval_seconds'", c.PollIntervalSeconds)
			}
			c.PollInterval = time.Duration(nSeconds) * time.Second
		}
		if len(c.FailOnMissingLogfileString) > 0 {
			c.FailOnMissingLogfile, err = strconv.ParseBool(c.FailOnMissingLogfileString)
			if err != nil {
				return fmt.Errorf("invalid input configuration: '%v' is not a valid boolean value in 'input.fail_on_missing_logfile'", c.FailOnMissingLogfileString)
			}
		}
	case c.Type == inputTypeWebhook:
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

func (c *GrokConfig) validate() error {
	if c.PatternsDir == "" && len(c.AdditionalPatterns) == 0 {
		return fmt.Errorf("Invalid grok configuration: no patterns defined: one of 'grok.patterns_dir' and 'grok.additional_patterns' must be configured.")
	}
	return nil
}

func (c *MetricsConfig) validate() error {
	if len(*c) == 0 {
		return fmt.Errorf("Invalid metrics configuration: 'metrics' must not be empty.")
	}
	metricNames := make(map[string]bool)
	for _, metric := range *c {
		err := metric.validate()
		if err != nil {
			return err
		}
		_, exists := metricNames[metric.Name]
		if exists {
			return fmt.Errorf("Invalid metric configuration: metric '%v' defined twice.", metric.Name)
		}
		metricNames[metric.Name] = true
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
	for i := range []MetricConfig(cfg.Metrics) {
		err = cfg.Metrics[i].InitTemplates()
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
	return stripped.marshalToString()
}

func (cfg *Config) copy() *Config {
	result, _ := Unmarshal([]byte(cfg.marshalToString()))
	return result
}

func (cfg *Config) marshalToString() string {
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Sprintf("ERROR: Failed to marshal config: %v", err.Error())
	}
	result := string(out)
	// Pretend fail_on_missing_logfile is a boolean, remove quotes
	result = strings.Replace(result, "fail_on_missing_logfile: \"false\"", "fail_on_missing_logfile: false", -1)
	result = strings.Replace(result, "fail_on_missing_logfile: \"true\"", "fail_on_missing_logfile: true", -1)
	return result
}
