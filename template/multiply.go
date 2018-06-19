package template

import (
	"fmt"
	"reflect"
	"strconv"
	"text/template/parse"
)

func newMultiplyFunc() functionWithValidator {
	return functionWithValidator{
		function:        multiply,
		staticValidator: validateMultiplyCall,
	}
}

func multiply(a, b interface{}) (float64, error) {
	floatA, err := toFloat64(a)
	if err != nil {
		return 0, fmt.Errorf("error evaluating multiply function: cannot convert %v to floating point number: %v", a, err)
	}
	floatB, err := toFloat64(b)
	if err != nil {
		return 0, fmt.Errorf("error evaluating multiply function: cannot convert %v to floating point number: %v", b, err)
	}
	return floatA * floatB, nil
}

func toFloat64(f interface{}) (float64, error) {
	val := reflect.ValueOf(f)
	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(val.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(val.Uint()), nil
	case reflect.Float32, reflect.Float64:
		return val.Float(), nil
	case reflect.String:
		return strconv.ParseFloat(val.String(), 64)
	}
	if val, ok := f.(fmt.Stringer); ok {
		return strconv.ParseFloat(val.String(), 64)
	}
	return 0, fmt.Errorf("%T: unknown type", f)
}

func validateMultiplyCall(cmd *parse.CommandNode) error {
	prefix := "syntax error in multiply call"
	if len(cmd.Args) != 3 {
		return fmt.Errorf("%v: expected two parameters, but found %v parameters", prefix, len(cmd.Args)-1)
	}
	// If a param is a string or number, we check if we can parse it.
	// Otherwise it might be a variable of a function call, we cannot check this statically.
	for _, paramPos := range []int{1, 2} {
		switch param := cmd.Args[paramPos].(type) {
		case *parse.NumberNode:
			if !param.IsFloat {
				return fmt.Errorf("%v: unable to parse %v as a floating point number", prefix, param)
			}
		case *parse.StringNode:
			if _, err := strconv.ParseFloat(param.Text, 64); err != nil {
				return fmt.Errorf("%v: unable to parse %v as a floating point number: %v", prefix, param, err)
			}
		}
	}
	return nil
}
