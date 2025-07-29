package dynamicparameters

import (
	"strconv"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/json"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// VariableValues is a helper function that converts a slice of TemplateVersionVariable
// into a map of cty.Value for use in coder/preview.
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
		// Defaulting the empty type to "string"
		// TODO: This does not match the terraform behavior, however it is too late
		// at this point in the code to determine this, as the database type stores all values
		// as strings. The code needs to be fixed in the `Parse` step of the provisioner.
		// That step should determine the type of the variable correctly and store it in the database.
		case "string", "":
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
