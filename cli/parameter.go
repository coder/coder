package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

// Reads a YAML file and populates a string -> string map.
// Throws an error if the file name is empty.
func createParameterMapFromFile(parameterFile string) (map[string]string, error) {
	if parameterFile != "" {
		parameterFileContents, err := os.ReadFile(parameterFile)
		if err != nil {
			return nil, err
		}

		mapStringInterface := make(map[string]interface{})
		err = yaml.Unmarshal(parameterFileContents, &mapStringInterface)
		if err != nil {
			return nil, err
		}

		parameterMap := map[string]string{}
		for k, v := range mapStringInterface {
			switch val := v.(type) {
			case string, bool, int:
				parameterMap[k] = fmt.Sprintf("%v", val)
			case []interface{}:
				b, err := json.Marshal(&val)
				if err != nil {
					return nil, err
				}
				parameterMap[k] = string(b)
			default:
				return nil, xerrors.Errorf("invalid parameter type: %T", v)
			}
		}
		return parameterMap, nil
	}

	return nil, xerrors.Errorf("Parameter file name is not specified")
}

func getWorkspaceBuildParameterValueFromMapOrInput(inv *clibase.Invocation, parameterMap map[string]string, templateVersionParameter codersdk.TemplateVersionParameter) (*codersdk.WorkspaceBuildParameter, error) {
	var parameterValue string
	var err error
	if parameterMap != nil {
		var ok bool
		parameterValue, ok = parameterMap[templateVersionParameter.Name]
		if !ok {
			parameterValue, err = cliui.RichParameter(inv, templateVersionParameter)
			if err != nil {
				return nil, err
			}
		}
	} else {
		parameterValue, err = cliui.RichParameter(inv, templateVersionParameter)
		if err != nil {
			return nil, err
		}
	}
	return &codersdk.WorkspaceBuildParameter{
		Name:  templateVersionParameter.Name,
		Value: parameterValue,
	}, nil
}
