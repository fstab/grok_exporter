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
	referencedGrokFields []string
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
		return "", fmt.Errorf("unexpected error while evaluating %v template: %v", t.Name(), err.Error())
	}
	return buf.String(), nil
}

func (t *tmplate) ReferencedGrokFields() []string {
	return t.referencedGrokFields
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

func referencedGrokFields(t *text_template.Template) ([]string, error) {
	result := make([]string, 0)
	for _, node := range t.Root.Nodes {
		if actionNode, ok := node.(*parse.ActionNode); ok {
			for _, cmd := range actionNode.Pipe.Cmds {
				if err := validateFunctionCalls(cmd); err != nil {
					return nil, err
				}
				result = append(result, extractGrokFields(cmd)...)
			}
		}
	}
	return result, nil
}

func extractGrokFields(cmd *parse.CommandNode) []string {
	result := make([]string, 0)
	for _, arg := range cmd.Args {
		if fieldNode, ok := arg.(*parse.FieldNode); ok {
			result = append(result, fieldNode.Ident...)
		}
	}
	return result
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
