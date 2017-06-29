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

package templates

import (
	"bytes"
	"fmt"
	"strings"
	text_template "text/template"
	"text/template/parse"
	"time"
)

type tmplate struct {
	template             *text_template.Template
	referencedGrokFields map[string]bool // We use this for a set of strings, the value is always true.
}

type Template interface {
	Execute(grokValues map[string]string) (string, error)
	ReferencedGrokFields() []string
	Name() string
}

func New(name, template string) (Template, error) {
	var (
		result *tmplate
		err    error
	)
	result = &tmplate{}
	result.template, err = text_template.New(name).Funcs(funcs).Parse(template)
	if err != nil {
		return nil, err
	}
	result.referencedGrokFields, err = referencedGrokFields(result.template)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (t *tmplate) Name() string {
	return t.template.Name()
}

func (t *tmplate) Execute(grokValues map[string]string) (string, error) {
	var buf bytes.Buffer
	err := t.template.Execute(&buf, grokValues)
	if err != nil {
		return "", fmt.Errorf("unexpected error while evaluating template %v: %v", t.Name(), err.Error())
	}
	return buf.String(), nil
}

func (t *tmplate) ReferencedGrokFields() []string {
	result := make([]string, len(t.referencedGrokFields))
	i := 0
	for field := range t.referencedGrokFields {
		result[i] = field
		i++
	}
	return result
}

var funcs = text_template.FuncMap{
	"timestamp": timestamp,
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

func referencedGrokFields(t *text_template.Template) (map[string]bool, error) {
	var (
		result = make(map[string]bool)
		fields map[string]bool
		err    error
	)
	for _, template := range t.Templates() {
		for _, node := range template.Root.Nodes {
			if fields, err = extractGrokFieldsFromNode(node); err != nil {
				return nil, err
			}
			for field := range fields {
				result[field] = true
			}
		}
	}
	return result, nil
}

func extractGrokFieldsFromNode(node parse.Node) (map[string]bool, error) {
	switch t := node.(type) {
	case *parse.ActionNode:
		return extractGrokFieldsFromPipeNode(t.Pipe)
	case *parse.RangeNode:
		return extractGrokFieldsFromBranchNode(&t.BranchNode)
	case *parse.IfNode:
		return extractGrokFieldsFromBranchNode(&t.BranchNode)
	case *parse.WithNode:
		return extractGrokFieldsFromBranchNode(&t.BranchNode)
	case *parse.TemplateNode:
		return extractGrokFieldsFromPipeNode(t.Pipe)
	default: // TextNode, etc have no grok fields
		return make(map[string]bool), nil
	}
}

func extractGrokFieldsFromPipeNode(node *parse.PipeNode) (map[string]bool, error) {
	var (
		result = make(map[string]bool)
		fields map[string]bool
		err    error
	)
	if node == nil {
		return result, err
	}
	for _, cmd := range node.Cmds {
		if err = validateFunctionCalls(cmd); err != nil {
			return nil, err
		}
		if fields, err = extractGrokFieldsFromCmd(cmd); err != nil {
			return nil, err
		}
		for field := range fields {
			result[field] = true
		}
	}
	return result, nil
}

func extractGrokFieldsFromCmd(cmd *parse.CommandNode) (map[string]bool, error) {
	result := make(map[string]bool)
	for _, arg := range cmd.Args {
		if fieldNode, ok := arg.(*parse.FieldNode); ok {
			for _, ident := range fieldNode.Ident {
				result[ident] = true
			}
		}
	}
	return result, nil
}

func validateFunctionCalls(cmd *parse.CommandNode) error {
	if len(cmd.Args) > 0 {
		if identifierNode, ok := cmd.Args[0].(*parse.IdentifierNode); ok {
			switch {
			case identifierNode.Ident == "timestamp":
				if err := validateTimestampCall(cmd); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func extractGrokFieldsFromBranchNode(node *parse.BranchNode) (map[string]bool, error) {
	var (
		result = make(map[string]bool)
		fields map[string]bool
		err    error
	)
	if fields, err = extractGrokFieldsFromPipeNode(node.Pipe); err != nil {
		return nil, err
	}
	for field := range fields {
		result[field] = true
	}
	if node.List != nil {
		for _, n := range node.List.Nodes {
			if fields, err = extractGrokFieldsFromNode(n); err != nil {
				return nil, err
			}
			for field := range fields {
				result[field] = true
			}
		}
	}
	if node.ElseList != nil {
		for _, n := range node.ElseList.Nodes {
			if fields, err = extractGrokFieldsFromNode(n); err != nil {
				return nil, err
			}
			for field := range fields {
				result[field] = true
			}
		}
	}
	return result, nil
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
