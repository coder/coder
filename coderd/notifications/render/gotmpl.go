package render

import (
	"fmt"
	"errors"
	"strings"

	"text/template"
	"github.com/coder/coder/v2/coderd/notifications/types"

)
// NoValue is used when a template variable is not found.
// This string is not exported as a const from the text/template.

const NoValue = "<no value>"
// GoTemplate attempts to substitute the given payload into the given template using Go's templating syntax.
// TODO: memoize templates for memory efficiency?
func GoTemplate(in string, payload types.MessagePayload, extraFuncs template.FuncMap) (string, error) {

	tmpl, err := template.New("text").
		Funcs(extraFuncs).
		// text/template substitutes a missing label with "<no value>".
		// NOTE: html/template does not, for obvious reasons.
		Option("missingkey=invalid").
		Parse(in)
	if err != nil {
		return "", fmt.Errorf("template parse: %w", err)
	}
	var out strings.Builder
	if err = tmpl.Execute(&out, payload); err != nil {
		return "", fmt.Errorf("template execute: %w", err)
	}

	return out.String(), nil
}
