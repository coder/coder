package cli

import (
	"os"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/spf13/cobra"

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

// Returns a parameter value from a given map, if the map exists, else takes input from the user.
// Throws an error if the map exists but does not include a value for the parameter.
func getParameterValueFromMapOrInput(cmd *cobra.Command, parameterMap map[string]string, parameterSchema codersdk.ParameterSchema) (string, error) {
	var parameterValue string
	if parameterMap != nil {
		var ok bool
		parameterValue, ok = parameterMap[parameterSchema.Name]
		if !ok {
			return "", xerrors.Errorf("Parameter value absent in parameter file for %q!", parameterSchema.Name)
		}
	} else {
		var err error
		parameterValue, err = cliui.ParameterSchema(cmd, parameterSchema)
		if err != nil {
			return "", err
		}
	}
	return parameterValue, nil
}
