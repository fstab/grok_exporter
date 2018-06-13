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
	"github.com/fstab/grok_exporter/oniguruma"
	"os"
	"text/template/parse"
)

func newGsubFunc() functionWithValidator {
	return functionWithValidator{
		function:        gsub,
		staticValidator: validateGsubCall,
	}
}

func gsub(src, expr, repl string) (string, error) {
	regex, err := oniguruma.Compile(expr)
	if err != nil {
		// this cannot happen, because validateGsubCall() was successful
		fmt.Fprintf(os.Stderr, "unexpected error compiling regex '%v': %v\n", expr, err)
		return src, nil
	}
	result, err := regex.Gsub(src, repl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unexpected error replacing '%v' with '%v': %v\n", regex, repl, err)
		return src, nil
	}
	return result, nil
}

func validateGsubCall(cmd *parse.CommandNode) error {
	prefix := "syntax error in gsub call"
	if len(cmd.Args) != 4 {
		return fmt.Errorf("%v: expected three parameters, but found %v parameters", prefix, len(cmd.Args)-1)
	}
	if stringNode, ok := cmd.Args[2].(*parse.StringNode); ok {
		if _, err := oniguruma.Compile(stringNode.Text); err != nil {
			return fmt.Errorf("%v: '%v' is not a valid regular expression: %v", prefix, stringNode.Text, err)
		}
	} else {
		// The regular expression should be a string, everything else is probably an error.
		return fmt.Errorf("%v: second parameter is not a valid regular expression", prefix)
	}
	if stringNode, ok := cmd.Args[3].(*parse.StringNode); ok {
		if err := oniguruma.ValidateReplacementString(stringNode.Text); err != nil {
			return fmt.Errorf("%v: '%v' is not a valid replacement: %v", prefix, stringNode.Text, err)
		}
	} else {
		// If the replacement is not a string, it could be an {{if ...}} {{else}} {{end}}, which is ok.
		// Ignore this case.
	}
	return nil
}
