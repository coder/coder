package cli

import (
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

// Reads a YAML file and populates a string -> string map.
// Throws an error if the file name is empty.
func createParameterMapFromFile(parameterFile string) (map[string]string, error) {
	if parameterFile != "" {
		parameterMap := make(map[string]string)

		parameterFileContents, err := os.ReadFile(parameterFile)
		if err != nil {
			return nil, err
		}

		err = yaml.Unmarshal(parameterFileContents, &parameterMap)

		if err != nil {
			return nil, err
		}

		return parameterMap, nil
	}

	return nil, xerrors.Errorf("Parameter file name is not specified")
}

// Returns a parameter value from a given map, if the map does not exist or does not contain the item, it takes input from the user.
// Throws an error if there are any errors with the users input.
func getParameterValueFromMapOrInput(cmd *cobra.Command, parameterMap map[string]string, parameterSchema codersdk.ParameterSchema) (string, error) {
	var parameterValue string
	var err error
	if parameterMap != nil {
		var ok bool
		parameterValue, ok = parameterMap[parameterSchema.Name]
		if !ok {
			parameterValue, err = cliui.ParameterSchema(cmd, parameterSchema)
			if err != nil {
				return "", err
			}
		}
	} else {
		parameterValue, err = cliui.ParameterSchema(cmd, parameterSchema)
		if err != nil {
			return "", err
		}
	}
	return parameterValue, nil
}

func getWorkspaceBuildParameterValueFromMapOrInput(cmd *cobra.Command, parameterMap map[string]string, templateVersionParameter codersdk.TemplateVersionParameter) (*codersdk.WorkspaceBuildParameter, error) {
	var parameterValue string
	var err error
	if parameterMap != nil {
		var ok bool
		parameterValue, ok = parameterMap[templateVersionParameter.Name]
		if !ok {
			parameterValue, err = cliui.RichParameter(cmd, templateVersionParameter)
			if err != nil {
				return nil, err
			}
		}
	} else {
		parameterValue, err = cliui.RichParameter(cmd, templateVersionParameter)
		if err != nil {
			return nil, err
		}
	}
	return &codersdk.WorkspaceBuildParameter{
		Name:  templateVersionParameter.Name,
		Value: parameterValue,
	}, nil
}
