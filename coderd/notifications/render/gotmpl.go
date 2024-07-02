package render

import (
	"strings"
	"text/template"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/notifications/types"
)

// GoTemplate attempts to substitute the given payload into the given template using Go's templating syntax.
// TODO: memoize templates for memory efficiency?
func GoTemplate(in string, payload types.MessagePayload, extraFuncs template.FuncMap) (string, error) {
	tmpl, err := template.New("text").Funcs(extraFuncs).Parse(in)
	if err != nil {
		return "", xerrors.Errorf("template parse: %w", err)
	}

	var out strings.Builder
	if err = tmpl.Execute(&out, payload); err != nil {
		return "", xerrors.Errorf("template execute: %w", err)
	}

	return out.String(), nil
}
