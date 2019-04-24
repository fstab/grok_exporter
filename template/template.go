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

package template

import (
	"bytes"
	"fmt"
	textTemplate "text/template"
	"text/template/parse"
)

// Works like golang's text/template, but additionally provides the list of referenced fields.
// Example: "{{if eq .field1 .field2}}{{.field3}}{{end}}"
// Executing this template is similar to text/template.Template.Execute(), and
// ReferencedGrokFields() yields {"field1", "field2", "field3"}
type Template interface {
	Execute(grokValues map[string]string) (string, error)
	ReferencedGrokFields() []string
	Name() string
}

type templateImpl struct {
	template             *textTemplate.Template
	referencedGrokFields map[string]bool // This map is used as a set. Value true indicates the string is present in the set.
}

func New(name, template string) (Template, error) {
	var (
		result *templateImpl
		err    error
	)
	result = &templateImpl{}
	result.template, err = textTemplate.New(name).Funcs(funcs.toFuncMap()).Parse(template)
	if err != nil {
		return nil, err
	}
	result.referencedGrokFields, err = referencedGrokFields(result.template)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (t *templateImpl) Name() string {
	return t.template.Name()
}

func (t *templateImpl) Execute(grokValues map[string]string) (string, error) {
	var buf bytes.Buffer
	err := t.template.Execute(&buf, grokValues)
	if err != nil {
		return "", fmt.Errorf("unexpected error while evaluating template for label %v: %v", t.Name(), err.Error())
	}
	return buf.String(), nil
}

func (t *templateImpl) ReferencedGrokFields() []string {
	result := make([]string, len(t.referencedGrokFields))
	i := 0
	for field := range t.referencedGrokFields {
		result[i] = field
		i++
	}
	return result
}

func referencedGrokFields(t *textTemplate.Template) (map[string]bool, error) {
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
	case *parse.PipeNode:
		return extractGrokFieldsFromPipeNode(t)
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
		for _, node := range cmd.Args {
			fields, err := extractGrokFieldsFromNode(node)
			if err != nil {
				return nil, err
			}
			for field := range fields {
				result[field] = true
			}
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

func validateFunctionCalls(cmd *parse.CommandNode) error {
	if len(cmd.Args) > 0 {
		if identifierNode, ok := cmd.Args[0].(*parse.IdentifierNode); ok {
			if err := funcs.validate(identifierNode.Ident, cmd); err != nil {
				return err
			}
		}
	}
	return nil
}
