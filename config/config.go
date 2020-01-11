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

package config

import (
	"fmt"
	"github.com/fstab/grok_exporter/config/v2"
	"io/ioutil"
	"regexp"
	"strconv"
	"strings"
)

// Example config: See ./example/config.yml

func LoadConfigFile(filename string) (*v2.Config, string, error) {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, "", fmt.Errorf("Failed to load %v: %v", filename, err.Error())
	}
	cfg, warn, err := LoadConfigString(content)
	if err != nil {
		return nil, warn, fmt.Errorf("Failed to load %v: %v", filename, err.Error())
	}
	return cfg, warn, nil
}

func LoadConfigString(content []byte) (*v2.Config, string, error) {
	version, warn, err := findVersion(string(content))
	if err != nil {
		return nil, warn, err
	}
	cfg, err := unmarshal(content, version)
	return cfg, warn, err
}

// returns (version, warning, error).
// Warning is for deprecating old versions, but as we currently only support version 2 it is currently not used.
func findVersion(content string) (int, string, error) {
	versionExpr := regexp.MustCompile(`"?global"?:\s*"?config_version"?:[\t\f ]*(\S+)`)
	versionInfo := versionExpr.FindStringSubmatch(content)
	if len(versionInfo) == 2 {
		version, err := strconv.Atoi(strings.TrimSpace(versionInfo[1]))
		if err != nil {
			return 0, "", fmt.Errorf("invalid 'global' configuration: '%v' is not a valid 'config_version'.", versionInfo[1])
		}
		return version, "", nil
	} else { // no version found
		return 0, "", fmt.Errorf("invalid configuration: 'global.config_version' not found.")
	}
}

func unmarshal(content []byte, version int) (*v2.Config, error) {
	switch version {
	case 2:
		return v2.Unmarshal(content)
	default:
		return nil, fmt.Errorf("global.config_version %v is not supported.", version)
	}
}
