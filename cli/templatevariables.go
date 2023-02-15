package cli

import (
	"os"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/codersdk"
)

func loadVariableValues(variablesFile string) ([]codersdk.VariableValue, error) {
	var values []codersdk.VariableValue
	if variablesFile == "" {
		return values, nil
	}

	variablesMap, err := createVariablesMapFromFile(variablesFile)
	if err != nil {
		return nil, err
	}

	for name, value := range variablesMap {
		values = append(values, codersdk.VariableValue{
			Name:  name,
			Value: value,
		})
	}
	return values, nil
}

// Reads a YAML file and populates a string -> string map.
// Throws an error if the file name is empty.
func createVariablesMapFromFile(variablesFile string) (map[string]string, error) {
	if variablesFile == "" {
		return nil, xerrors.Errorf("variable file name is not specified")
	}

	variablesMap := make(map[string]string)
	variablesFileContents, err := os.ReadFile(variablesFile)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(variablesFileContents, &variablesMap)
	if err != nil {
		return nil, err
	}
	return variablesMap, nil
}
