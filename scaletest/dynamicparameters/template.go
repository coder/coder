package dynamicparameters

import (
	_ "embed"
	"strings"
	"text/template"

	"github.com/coder/coder/v2/cryptorand"
)

//go:embed workspace-template.tf
var templateContent string

func TemplateContent() (string, error) {
	randomString, err := cryptorand.String(8)
	if err != nil {
		return "", err
	}

	// Parse the template
	tmpl, err := template.New("workspace-template").Parse(templateContent)
	if err != nil {
		return "", err
	}

	// Execute the template with the random string
	var result strings.Builder
	err = tmpl.Execute(&result, map[string]string{
		"RandomString": randomString,
	})
	if err != nil {
		// Return the original template if execution fails
		return "", err
	}

	return result.String(), nil
}
