package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"

	"github.com/coder/coder/v2/codersdk"
)

/**
 * DiscoverVarsFiles function loads vars files in a predefined order:
 * 1. terraform.tfvars
 * 2. terraform.tfvars.json
 * 3. *.auto.tfvars
 * 4. *.auto.tfvars.json
 */
func DiscoverVarsFiles(workDir string) ([]string, error) {
	var found []string

	fi, err := os.Stat(filepath.Join(workDir, "terraform.tfvars"))
	if err == nil {
		found = append(found, filepath.Join(workDir, fi.Name()))
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	fi, err = os.Stat(filepath.Join(workDir, "terraform.tfvars.json"))
	if err == nil {
		found = append(found, filepath.Join(workDir, fi.Name()))
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	dirEntries, err := os.ReadDir(workDir)
	if err != nil {
		return nil, err
	}

	for _, dirEntry := range dirEntries {
		if strings.HasSuffix(dirEntry.Name(), ".auto.tfvars") || strings.HasSuffix(dirEntry.Name(), ".auto.tfvars.json") {
			found = append(found, filepath.Join(workDir, dirEntry.Name()))
		}
	}
	return found, nil
}

func ParseUserVariableValues(varsFiles []string, variablesFile string, commandLineVariables []string) ([]codersdk.VariableValue, error) {
	fromVars, err := parseVariableValuesFromVarsFiles(varsFiles)
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

func parseVariableValuesFromVarsFiles(varsFiles []string) ([]codersdk.VariableValue, error) {
	var parsed []codersdk.VariableValue
	for _, varsFile := range varsFiles {
		content, err := os.ReadFile(varsFile)
		if err != nil {
			return nil, err
		}

		var t []codersdk.VariableValue
		ext := filepath.Ext(varsFile)
		switch ext {
		case ".tfvars":
			t, err = parseVariableValuesFromHCL(content)
			if err != nil {
				return nil, xerrors.Errorf("unable to parse HCL content: %w", err)
			}
		case ".json":
			t, err = parseVariableValuesFromJSON(content)
			if err != nil {
				return nil, xerrors.Errorf("unable to parse JSON content: %w", err)
			}
		default:
			return nil, xerrors.Errorf("unexpected tfvars format: %s", ext)
		}

		parsed = append(parsed, t...)
	}
	return parsed, nil
}

func parseVariableValuesFromHCL(content []byte) ([]codersdk.VariableValue, error) {
	parser := hclparse.NewParser()
	hclFile, diags := parser.ParseHCL(content, "file.hcl")
	if diags.HasErrors() {
		return nil, diags
	}

	attrs, diags := hclFile.Body.JustAttributes()
	if diags.HasErrors() {
		return nil, diags
	}

	stringData := map[string]string{}
	for _, attribute := range attrs {
		ctyValue, diags := attribute.Expr.Value(nil)
		if diags.HasErrors() {
			return nil, diags
		}

		ctyType := ctyValue.Type()
		if ctyType.Equals(cty.String) {
			stringData[attribute.Name] = ctyValue.AsString()
		} else if ctyType.Equals(cty.Number) {
			stringData[attribute.Name] = ctyValue.AsBigFloat().String()
		} else if ctyType.IsTupleType() {
			// In case of tuples, Coder only supports the list(string) type.
			var items []string
			var err error
			_ = ctyValue.ForEachElement(func(key, val cty.Value) (stop bool) {
				if !val.Type().Equals(cty.String) {
					err = xerrors.Errorf("unsupported tuple item type: %s ", val.GoString())
					return true
				}
				items = append(items, val.AsString())
				return false
			})
			if err != nil {
				return nil, err
			}

			m, err := json.Marshal(items)
			if err != nil {
				return nil, err
			}
			stringData[attribute.Name] = string(m)
		} else {
			return nil, xerrors.Errorf("unsupported value type (name: %s): %s", attribute.Name, ctyType.GoString())
		}
	}

	return convertMapIntoVariableValues(stringData), nil
}

// parseVariableValuesFromJSON converts the .tfvars.json content into template variables.
// The function visits only root-level properties as template variables do not support nested
// structures.
func parseVariableValuesFromJSON(content []byte) ([]codersdk.VariableValue, error) {
	var data map[string]interface{}
	err := json.Unmarshal(content, &data)
	if err != nil {
		return nil, err
	}

	stringData := map[string]string{}
	for key, value := range data {
		switch value.(type) {
		case string, int, bool:
			stringData[key] = fmt.Sprintf("%v", value)
		default:
			m, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			stringData[key] = string(m)
		}
	}

	return convertMapIntoVariableValues(stringData), nil
}

func convertMapIntoVariableValues(m map[string]string) []codersdk.VariableValue {
	var parsed []codersdk.VariableValue
	for key, value := range m {
		parsed = append(parsed, codersdk.VariableValue{
			Name:  key,
			Value: value,
		})
	}
	sort.Slice(parsed, func(i, j int) bool {
		return parsed[i].Name < parsed[j].Name
	})
	return parsed
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

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}
