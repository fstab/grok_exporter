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
	"os"
	"strconv"
	"time"
)

func convert(v2cfg *v2.Config) *Config {
	return &Config{
		Global:       convertGlobal(v2cfg),
		Input:        convertInput(v2cfg),
		GrokPatterns: convertGrok(v2cfg),
		OrigMetrics:  convertMetrics(v2cfg),
		Imports:      convertImports(v2cfg),
		Server:       convertServer(v2cfg),
	}
}

func convertGlobal(v2cfg *v2.Config) GlobalConfig {
	return GlobalConfig{
		ConfigVersion:          3,
		RetentionCheckInterval: v2cfg.Global.RetentionCheckInterval,
	}
}

func convertInput(v2cfg *v2.Config) InputConfig {
	return InputConfig{
		Type:                       v2cfg.Input.Type,
		LineDelimiter:              "\n",
		PathsAndGlobs:              convertPathsAndGlobs(v2cfg.Input.PathsAndGlobs),
		FailOnMissingLogfileString: v2cfg.Input.FailOnMissingLogfileString,
		FailOnMissingLogfile:       v2cfg.Input.FailOnMissingLogfile,
		Readall:                    v2cfg.Input.Readall,
		PollInterval:               convertPollInterval(v2cfg),
		MaxLinesInBuffer:           v2cfg.Input.MaxLinesInBuffer,
		WebhookPath:                v2cfg.Input.WebhookPath,
		WebhookFormat:              v2cfg.Input.WebhookFormat,
		WebhookJsonSelector:        v2cfg.Input.WebhookJsonSelector,
		WebhookTextBulkSeparator:   v2cfg.Input.WebhookTextBulkSeparator,
	}
}

func convertPathsAndGlobs(v2globs v2.PathsAndGlobs) PathsAndGlobs {
	return PathsAndGlobs{
		Path:  v2globs.Path,
		Paths: v2globs.Paths,
		Globs: v2globs.Globs,
	}
}

func convertPollInterval(v2cfg *v2.Config) time.Duration {
	if len(v2cfg.Input.PollIntervalSeconds) > 0 {
		nSeconds, err := strconv.Atoi(v2cfg.Input.PollIntervalSeconds)
		if err != nil {
			// This cannot happen, because outside of tests v2cfg is validated before it is converted.
			fmt.Fprintf(os.Stderr, "invalid configuration: '%v' is not a valid number in 'input.poll_interval_seconds'", v2cfg.Input.PollIntervalSeconds)
			os.Exit(1)
		}
		return time.Duration(nSeconds) * time.Second
	}
	return 0
}

func convertGrok(v2cfg *v2.Config) GrokPatternsConfig {
	result := make([]string, 0, len(v2cfg.Grok.AdditionalPatterns))
	for _, pattern := range v2cfg.Grok.AdditionalPatterns {
		result = append(result, pattern)
	}
	return result
}

func convertMetrics(v2cfg *v2.Config) MetricsConfig {
	result := make([]MetricConfig, 0, len(v2cfg.Metrics))
	for _, v2metric := range v2cfg.Metrics {
		result = append(result, MetricConfig{
			Type:                 v2metric.Type,
			Name:                 v2metric.Name,
			Help:                 v2metric.Help,
			PathsAndGlobs:        convertPathsAndGlobs(v2metric.PathsAndGlobs),
			Match:                v2metric.Match,
			Retention:            v2metric.Retention,
			Value:                v2metric.Value,
			Cumulative:           v2metric.Cumulative,
			Buckets:              v2metric.Buckets,
			Quantiles:            v2metric.Quantiles,
			Labels:               v2metric.Labels,
			LabelTemplates:       v2metric.LabelTemplates,
			ValueTemplate:        v2metric.ValueTemplate,
			DeleteMatch:          v2metric.DeleteMatch,
			DeleteLabels:         v2metric.DeleteLabels,
			DeleteLabelTemplates: v2metric.DeleteLabelTemplates,
		})
	}
	return result
}

func convertImports(v2cfg *v2.Config) ImportsConfig {
	if len(v2cfg.Grok.PatternsDir) > 0 {
		return []ImportConfig{{
			Type: "grok_patterns",
			Dir:  v2cfg.Grok.PatternsDir,
		}}
	} else {
		return nil
	}
}

func convertServer(v2cfg *v2.Config) ServerConfig {
	return ServerConfig{
		Protocol: v2cfg.Server.Protocol,
		Host:     v2cfg.Server.Host,
		Port:     v2cfg.Server.Port,
		Path:     v2cfg.Server.Path,
		Cert:     v2cfg.Server.Cert,
		Key:      v2cfg.Server.Key,
	}
}
