package dynamicparameters

import (
	_ "embed"
	"encoding/json"
	"strings"
	"text/template"

	"github.com/coder/coder/v2/cryptorand"
)

//go:embed tf/main.tf
var templateContent string

func TemplateContent() (string, error) {
	randomString, err := cryptorand.String(8)
	if err != nil {
		return "", err
	}
	tmpl, err := template.New("workspace-template").Parse(templateContent)
	if err != nil {
		return "", err
	}
	var result strings.Builder
	err = tmpl.Execute(&result, map[string]string{
		"RandomString": randomString,
	})
	if err != nil {
		return "", err
	}
	return result.String(), nil
}

//go:embed tf/modules/two/main.tf
var moduleTwoMainTF string

// GetModuleFiles returns a map of module files to be used with ExtraFiles
func GetModuleFiles() map[string][]byte {
	// Create the modules.json that Terraform needs to see the module
	modulesJSON := struct {
		Modules []struct {
			Key    string `json:"Key"`
			Source string `json:"Source"`
			Dir    string `json:"Dir"`
		} `json:"Modules"`
	}{
		Modules: []struct {
			Key    string `json:"Key"`
			Source string `json:"Source"`
			Dir    string `json:"Dir"`
		}{
			{
				Key:    "",
				Source: "",
				Dir:    ".",
			},
			{
				Key:    "two",
				Source: "./modules/two",
				Dir:    "modules/two",
			},
		},
	}

	modulesJSONBytes, err := json.Marshal(modulesJSON)
	if err != nil {
		panic(err) // This should never happen with static data
	}

	return map[string][]byte{
		"modules/two/main.tf":             []byte(moduleTwoMainTF),
		".terraform/modules/modules.json": modulesJSONBytes,
	}
}
