// Copyright 2019 The grok_exporter Authors
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
	"path/filepath"
	"text/template/parse"
)

func newBaseFunc() functionWithValidator {
	return functionWithValidator{
		function:        base,
		staticValidator: validateBaseCall,
	}
}

func base(path string) string {
	return filepath.Base(path)
}

func validateBaseCall(cmd *parse.CommandNode) error {
	prefix := "syntax error in base call"
	if len(cmd.Args) != 2 {
		return fmt.Errorf("%v: expected one parameter, but found %v parameters", prefix, len(cmd.Args)-1)
	}
	return nil
}
