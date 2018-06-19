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
	textTemplate "text/template"
	"text/template/parse"
)

var funcs functions = make(map[string]functionWithValidator)

func init() {
	funcs.add("timestamp", newTimestampFunc())
	funcs.add("gsub", newGsubFunc())
	funcs.add("add", newAddFunc())
	funcs.add("subtract", newSubtractFunc())
	funcs.add("multiply", newMultiplyFunc())
	funcs.add("divide", newDivideFunc())
}

type functions map[string]functionWithValidator

type functionWithValidator struct {
	function        interface{}
	staticValidator func(cmd *parse.CommandNode) error
}

func (funcs functions) add(name string, f functionWithValidator) {
	funcs[name] = f
}

func (funcs functions) toFuncMap() textTemplate.FuncMap {
	result := make(textTemplate.FuncMap, len(funcs))
	for name, f := range funcs {
		result[name] = f.function
	}
	return result
}

func (funcs functions) validate(name string, cmd *parse.CommandNode) error {
	f, exists := funcs[name]
	if !exists {
		return nil // not one of our custom functions, skip validation
	}
	return f.staticValidator(cmd)
}
