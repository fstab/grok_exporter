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
	"github.com/fstab/grok_exporter/tailer/glob"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"path"
)

func importMetrics(importsConfig ImportsConfig) (MetricsConfig, error) {
	var (
		importConfig ImportConfig
		result       MetricsConfig
		err          error
		files        []*configFile
	)
	for _, importConfig = range importsConfig {
		if importConfig.Type != importMetricsType {
			continue
		}
		err = importConfig.validate()
		if err != nil {
			return nil, err
		}
		if len(importConfig.Dir) > 0 {
			files, err = loadDir(importConfig.Dir)
		} else {
			files, err = loadGlob(importConfig.File)
		}
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			var metricsConfig MetricsConfig
			err := yaml.Unmarshal([]byte(file.contents), &metricsConfig)
			if err != nil {
				return nil, fmt.Errorf("%v: %v", file.path, err)
			}
			for i := range metricsConfig {
				applyImportDefaults(&metricsConfig[i], importConfig.Defaults)
				result = append(result, metricsConfig[i])
			}
		}
	}
	return result, nil
}

func applyImportDefaults(metricConfig *MetricConfig, defaults DefaultConfig) {
	for key, value := range defaults.Labels {
		if _, exists := metricConfig.Labels[key]; !exists {
			if metricConfig.Labels == nil {
				metricConfig.Labels = make(map[string]string)
			}
			metricConfig.Labels[key] = value
		}
	}
	if len(metricConfig.Quantiles) == 0 {
		metricConfig.Quantiles = defaults.Quantiles
	}
	if len(metricConfig.Buckets) == 0 {
		metricConfig.Buckets = defaults.Buckets
	}
	if metricConfig.Retention == 0 {
		metricConfig.Retention = defaults.Retention
	}
	if len(metricConfig.Path) == 0 && len(metricConfig.Paths) == 0 {
		metricConfig.Path = defaults.Path
		metricConfig.Paths = defaults.Paths
	}
}

type configFile struct {
	path     string
	contents string
}

func loadDir(dir string) ([]*configFile, error) {
	return loadGlob(path.Join(dir, "*"))
}

func loadGlob(globString string) ([]*configFile, error) {
	result := make([]*configFile, 0, 0)
	g, err := glob.Parse(globString)
	if err != nil {
		return nil, err
	}
	fileInfos, err := ioutil.ReadDir(g.Dir())
	if err != nil {
		return nil, err
	}
	for _, fileInfo := range fileInfos {
		filePath := path.Join(g.Dir(), fileInfo.Name())
		if g.Match(filePath) {
			contents, err := ioutil.ReadFile(filePath)
			if err != nil {
				return nil, err
			}
			result = append(result, &configFile{
				path:     filePath,
				contents: string(contents),
			})
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("%v: file(s) not found", globString)
	}
	return result, nil
}
