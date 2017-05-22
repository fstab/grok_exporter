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

package v2

import (
	"fmt"
	"github.com/fstab/grok_exporter/templates"
	"gopkg.in/yaml.v2"
	"regexp"
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

func (cfg *Config) String() string {
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Sprintf("ERROR: Failed to marshal config: %v", err.Error())
	}
	return string(out)
}

type GlobalConfig struct {
	ConfigVersion int `yaml:"config_version,omitempty"`
	PushgatewayAddr string `yaml:"pushgateway_addr,omitempty"` // add pushgateway address <ip>:<port>
}

type InputConfig struct {
	Type    string `yaml:",omitempty"`
	Path    string `yaml:",omitempty"`
	Readall bool   `yaml:",omitempty"`
}

type GrokConfig struct {
	PatternsDir        string   `yaml:"patterns_dir,omitempty"`
	AdditionalPatterns []string `yaml:"additional_patterns,omitempty"`
}

type MetricConfig struct {
	Type           string               `yaml:",omitempty"`
	Name           string               `yaml:",omitempty"`
	Help           string               `yaml:",omitempty"`
	Match          string               `yaml:",omitempty"`

	/**************pushgateway related configs*******************/
	Pushgateway    bool					`yaml:",omitempty"` //flag for if push metric to pushgateway or not 
	JobName		   string 				`yaml:"job_name,omitempty"` //job name used to push metric to pushgateway
	DeleteMatch    string               `yaml:"delete_match,omitempty"` //if match, metric will be deleted from pushgateway
	GroupingKey    map[string]string 	`yaml:"grouping_key,omitempty"` //grouping key used to push and delete metric from pushgateway
    GroupTemplates []templates.Template `yaml:"-"`
	/**************end of pushgateway related configs************/

	Value          string               `yaml:",omitempty"`
	Cumulative     bool                 `yaml:",omitempty"`
	Buckets        []float64            `yaml:",flow,omitempty"`
	Quantiles      map[float64]float64  `yaml:",flow,omitempty"`
	Labels         map[string]string    `yaml:",omitempty"`
	LabelTemplates []templates.Template `yaml:"-"` // parsed version of Labels, will not be serialized to yaml.
	ValueTemplate  templates.Template   `yaml:"-"` // parsed version of Value, will not be serialized to yaml.
}

type MetricsConfig []*MetricConfig

type ServerConfig struct {
	Protocol string `yaml:",omitempty"`
	Host     string `yaml:",omitempty"`
	Port     int    `yaml:",omitempty"`
	Cert     string `yaml:",omitempty"`
	Key      string `yaml:",omitempty"`
}

type Config struct {
	Global  *GlobalConfig  `yaml:",omitempty"`
	Input   *InputConfig   `yaml:",omitempty"`
	Grok    *GrokConfig    `yaml:",omitempty"`
	Metrics *MetricsConfig `yaml:",omitempty"`
	Server  *ServerConfig  `yaml:",omitempty"`
}

func (cfg *Config) addDefaults() {
	if cfg.Global == nil {
		cfg.Global = &GlobalConfig{}
	}
	cfg.Global.addDefaults()
	if cfg.Input == nil {
		cfg.Input = &InputConfig{}
	}
	cfg.Input.addDefaults()
	if cfg.Grok == nil {
		cfg.Grok = &GrokConfig{}
	}
	cfg.Grok.addDefaults()
	if cfg.Metrics == nil {
		metrics := MetricsConfig(make([]*MetricConfig, 0))
		cfg.Metrics = &metrics
	}
	cfg.Metrics.addDefaults()
	if cfg.Server == nil {
		cfg.Server = &ServerConfig{}
	}
	cfg.Server.addDefaults()
}

func (c *GlobalConfig) addDefaults() {
	if c.ConfigVersion == 0 {
		c.ConfigVersion = 2
	}
}

func (c *InputConfig) addDefaults() {
	if c.Type == "" {
		c.Type = "stdin"
	}
}

func (c *GrokConfig) addDefaults() {}

//add default job name
func (c *MetricConfig) addDefaults() {
	if c.JobName == "" {
		c.JobName = "grok_exporter"
	}
}

func (c *ServerConfig) addDefaults() {
	if c.Protocol == "" {
		c.Protocol = "http"
	}
	if c.Port == 0 {
		c.Port = 9144
	}
}

func (cfg *Config) validate() error {
	err := cfg.Global.validate()
	if err != nil {
		return err
	}
	err = cfg.Input.validate()
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

func (c *GlobalConfig) validate() error {
	//ignore version validation
	if len(c.PushgatewayAddr) > 0 {
			reg := regexp.MustCompile("\\d{1,3}\\.\\d{1,3}\\.\\d{1,3}\\.\\d{1,3}:[0-9]{1,5}")
			result := reg.FindString(c.PushgatewayAddr)
			if result == "" {
				return fmt.Errorf("Not valid pushgateway address, usage: <ip>>:<port>.")
			}
	}
	return nil
}
func (c *InputConfig) validate() error {
	switch {
	case c.Type == "stdin":
		if c.Path != "" {
			return fmt.Errorf("Invalid input configuration: cannot use 'input.path' when 'input.type' is stdin.")
		}
	case c.Type == "file":
		if c.Path == "" {
			return fmt.Errorf("Invalid input configuration: 'input.path' is required for input type \"file\".")
		}
	default:
		return fmt.Errorf("Unsupported 'input.type': %v", c.Type)
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
	// Labels and value are validated in InitTemplates()
	//validate pushgateway related configs
	if len(c.DeleteMatch) > 0 && !c.Pushgateway {
		return fmt.Errorf("Invalid metric configuration: 'delete_match' can only be defined when 'pushgateway = true'.")
	}
	//grouping key is validated in InitTemplates()
	return nil
}

func (c *ServerConfig) validate() error {
	switch {
	case c.Protocol != "https" && c.Protocol != "http":
		return fmt.Errorf("Invalid 'server.protocol': '%v'. Expecting 'http' or 'https'.", c.Protocol)
	case c.Port <= 0:
		return fmt.Errorf("Invalid 'server.port': '%v'.", c.Port)
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
	for _, metric := range []*MetricConfig(*cfg.Metrics) {
		err = metric.InitTemplates()
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
		tmplt templates.Template
		msg   = "invalid configuration: failed to read metric %v: error parsing %v template: %v: " +
			"don't forget to put a . (dot) in front of grok fields, otherwise it will be interpreted as a function."
	)
	metric.LabelTemplates = make([]templates.Template, 0, len(metric.Labels))
	for name, templateString := range metric.Labels {
		tmplt, err = templates.New(name, templateString)
		if err != nil {
			return fmt.Errorf(msg, fmt.Sprintf("label %v", metric.Name), name, err.Error())
		}
		metric.LabelTemplates = append(metric.LabelTemplates, tmplt)
	}
	// validate grouping key
	metric.GroupTemplates = make([]templates.Template, 0, len(metric.GroupingKey))
    for name, templateString := range metric.GroupingKey {
        tmplt, err = templates.New(name, templateString)
        if err != nil {
            return fmt.Errorf(msg, fmt.Sprintf("groupingKey %v", metric.Name), name, err.Error())
        }
        metric.GroupTemplates = append(metric.GroupTemplates, tmplt)
    }

	if len(metric.Value) > 0 {
		metric.ValueTemplate, err = templates.New("__value__", metric.Value)
		if err != nil {
			return fmt.Errorf(msg, "value", metric.Name, err.Error())
		}
	}
	return nil
}
