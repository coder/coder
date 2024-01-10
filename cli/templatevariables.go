package cli

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/v2/codersdk"
)

func ParseUserVariableValues(workDir string, variablesFile string, commandLineVariables []string) ([]codersdk.VariableValue, error) {
	varsFiles, err := discoverVarsFiles(workDir)
	if err != nil {
		return nil, err
	}

	fromVars, err := parseTerraformVarsFromFiles(varsFiles)
	if err != nil {
		return nil, err
	}

	fromFile, err := parseVariableValuesFromFile(variablesFile)
	if err != nil {
		return nil, err
	}

	fromCommandLine, err := parseVariableValuesFromCommandLine(commandLineVariables)
	if err != nil {
		return nil, err
	}

	return combineVariableValues(fromVars, fromFile, fromCommandLine), nil
}

/**
 * discoverVarsFiles function loads vars files in a predefined order:
 * 1. terraform.tfvars
 * 2. terraform.tfvars.json
 * 3. *.auto.tfvars
 * 4. *.auto.tfvars.json
 */
func discoverVarsFiles(workDir string) ([]string, error) {
	var found []string

	fi, err := os.Stat(filepath.Join(workDir, "terraform.tfvars"))
	if err == nil {
		found = append(found, fi.Name())
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	fi, err = os.Stat(filepath.Join(workDir, "terraform.tfvars.json"))
	if err == nil {
		found = append(found, fi.Name())
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	dirEntries, err := os.ReadDir(workDir)
	if err != nil {
		return nil, err
	}

	for _, dirEntry := range dirEntries {
		if strings.HasSuffix(dirEntry.Name(), ".auto.tfvars") || strings.HasSuffix(dirEntry.Name(), ".auto.tfvars.json") {
			found = append(found, dirEntry.Name())
		}
	}
	return found, nil
}

func parseTerraformVarsFromFiles(varsFiles []string) ([]codersdk.VariableValue, error) {
	panic("not implemented yet")
}

func parseTerraformVarsFromHCL(hcl string) ([]codersdk.VariableValue, error) {
	panic("not implemented yet")
}

func parseTerraformVarsFromJSON(json string) ([]codersdk.VariableValue, error) {
	panic("not implemented yet")
}

func parseVariableValuesFromFile(variablesFile string) ([]codersdk.VariableValue, error) {
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

func parseVariableValuesFromCommandLine(variables []string) ([]codersdk.VariableValue, error) {
	var values []codersdk.VariableValue
	for _, keyValue := range variables {
		split := strings.SplitN(keyValue, "=", 2)
		if len(split) < 2 {
			return nil, xerrors.Errorf("format key=value expected, but got %s", keyValue)
		}

		values = append(values, codersdk.VariableValue{
			Name:  split[0],
			Value: split[1],
		})
	}
	return values, nil
}

func combineVariableValues(valuesSets ...[]codersdk.VariableValue) []codersdk.VariableValue {
	combinedValues := make(map[string]string)

	for _, values := range valuesSets {
		for _, v := range values {
			combinedValues[v.Name] = v.Value
		}
	}

	var result []codersdk.VariableValue
	for name, value := range combinedValues {
		result = append(result, codersdk.VariableValue{Name: name, Value: value})
	}

	return result
}
