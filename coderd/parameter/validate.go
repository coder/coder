package parameter

import (
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"golang.org/x/xerrors"
)

// Contains parses possible values for a conditional.
func Contains(condition string) ([]string, bool, error) {
	if condition == "" {
		return nil, false, nil
	}
	expression, diags := hclsyntax.ParseExpression([]byte(condition), "", hcl.InitialPos)
	if len(diags) > 0 {
		return nil, false, xerrors.Errorf("parse condition: %s", diags.Error())
	}
	functionCallExpression, valid := expression.(*hclsyntax.FunctionCallExpr)
	if !valid {
		return nil, false, nil
	}
	if functionCallExpression.Name != "contains" {
		return nil, false, nil
	}
	if len(functionCallExpression.Args) < 2 {
		return nil, false, nil
	}
	value, diags := functionCallExpression.Args[0].Value(&hcl.EvalContext{})
	if len(diags) > 0 {
		return nil, false, xerrors.Errorf("parse value: %s", diags.Error())
	}
	possible := make([]string, 0)
	for _, subValue := range value.AsValueSlice() {
		if subValue.Type().FriendlyName() != "string" {
			continue
		}
		possible = append(possible, subValue.AsString())
	}
	sort.Strings(possible)
	return possible, true, nil
}
