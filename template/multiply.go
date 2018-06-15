package template

import (
	"fmt"
	"strconv"
	"text/template/parse"
)

func newMultiplyFunc() functionWithValidator {
	return functionWithValidator{
		function:        multiply,
		staticValidator: validateMultiplyCall,
	}
}

func multiply(value string, multiplier float64) (float64, error) {
	i, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}

	return i * multiplier, nil
}

func validateMultiplyCall(cmd *parse.CommandNode) error {
	prefix := "syntax error in shift call"
	if len(cmd.Args) != 3 {
		return fmt.Errorf("%v: expected two parameters, but found %v parameters", prefix, len(cmd.Args)-1)
	}
	exponentNode, ok := cmd.Args[2].(*parse.NumberNode)
	if !ok || !exponentNode.IsFloat {
		return fmt.Errorf("%v: unable to parse %v as a float number", prefix, exponentNode.Text)
	}

	return nil
}
