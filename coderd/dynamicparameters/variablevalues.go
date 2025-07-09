package dynamicparameters

import (
	"strconv"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/json"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

func VariableValues(vals []database.TemplateVersionVariable) (map[string]cty.Value, error) {
	ctyVals := make(map[string]cty.Value, len(vals))
	for _, v := range vals {
		value := v.Value
		if value == "" && v.DefaultValue != "" {
			value = v.DefaultValue
		}

		if value == "" {
			// Empty strings are unsupported I guess?
			continue // omit non-set vals
		}

		var err error
		switch v.Type {
		case "string":
			ctyVals[v.Name] = cty.StringVal(value)
		case "number":
			ctyVals[v.Name], err = cty.ParseNumberVal(value)
			if err != nil {
				return nil, xerrors.Errorf("parse variable %q: %w", v.Name, err)
			}
		case "bool":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return nil, xerrors.Errorf("parse variable %q: %w", v.Name, err)
			}
			ctyVals[v.Name] = cty.BoolVal(parsed)
		default:
			// If it is a complex type, let the cty json code give it a try.
			// TODO: Ideally we parse `list` & `map` and build the type ourselves.
			ty, err := json.ImpliedType([]byte(value))
			if err != nil {
				return nil, xerrors.Errorf("implied type for variable %q: %w", v.Name, err)
			}

			jv, err := json.Unmarshal([]byte(value), ty)
			if err != nil {
				return nil, xerrors.Errorf("unmarshal variable %q: %w", v.Name, err)
			}
			ctyVals[v.Name] = jv
		}
	}

	return ctyVals, nil
}
