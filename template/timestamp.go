// Copyright 2018 The grok_exporter Authors
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

package template

import (
	"fmt"
	"strings"
	"text/template/parse"
	"time"
)

func newTimestampFunc() functionWithValidator {
	return functionWithValidator{
		function:        timestamp,
		staticValidator: validateTimestampCall,
	}
}

func timestamp(layout, value string) (float64, error) {
	layout, value, err := fixCommas(layout, value)
	if err != nil {
		return 0, err
	}
	result, err := time.Parse(layout, value)
	if err != nil {
		return 0, err
	}
	return float64(result.UnixNano()) * time.Nanosecond.Seconds(), nil
}

// Cannot parse ISO 8601 timestamps (commonly used in log4j) with time.Parse()
// because these timestamps use a comma separator between seconds and microseconds
// while time.Parse() requires a dot separator between seconds and microseconds.
// As a workaround, replace comma with dot. See https://github.com/golang/go/issues/6189
func fixCommas(layout, value string) (string, string, error) {
	errmsg := "comma not allowed in reference timestamp, except for milliseconds ',000' or ',999'"
	switch strings.Count(layout, ",") {
	case 0:
		return layout, value, nil // no comma -> nothing to fix
	case 1:
		if strings.Contains(layout, ",000") || strings.Contains(layout, ",999") {
			layout = strings.Replace(layout, ",", ".", -1)
			value = strings.Replace(value, ",", ".", -1)
			return layout, value, nil
		} else {
			return "", "", fmt.Errorf("%v.", errmsg)
		}
	default:
		return "", "", fmt.Errorf("%v.", errmsg)
	}
}

func validateTimestampCall(cmd *parse.CommandNode) error {
	prefix := "syntax error in timestamp call"
	if len(cmd.Args) != 3 {
		return fmt.Errorf("%v: expected two parameters, but found %v parameters.", prefix, len(cmd.Args)-1)
	}
	if stringNode, ok := cmd.Args[1].(*parse.StringNode); ok {
		_, err := timestamp(stringNode.Text, stringNode.Text)
		if err != nil {
			return fmt.Errorf("%v: %v is not a valid reference timestamp: %v", prefix, stringNode.Text, err)
		}
	} else {
		return fmt.Errorf("%v: first parameter is not a valid reference timestamp.", prefix)
	}
	return nil
}
