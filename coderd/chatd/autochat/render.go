package autochat

import (
	"bytes"
	"strings"
	"text/template"
	"time"

	"golang.org/x/xerrors"
)

// promptFuncMap returns the restricted set of functions available in
// prompt templates. A fresh map is returned each call so callers
// cannot mutate shared state.
func promptFuncMap() template.FuncMap {
	return template.FuncMap{
		"trimSpace": strings.TrimSpace,
		"upper":     strings.ToUpper,
		"lower":     strings.ToLower,
		"now":       func() string { return time.Now().UTC().Format(time.RFC3339) },
	}
}

// RenderPrompt executes a Go text/template against the provided data.
// Webhook triggers pass {"Body": <parsed JSON>, "Headers": <map>}.
// Cron triggers pass {"ScheduledAt": <RFC3339 string>}.
func RenderPrompt(tmpl string, data map[string]any) (string, error) {
	t, err := template.New("prompt").
		Option("missingkey=error").
		Funcs(promptFuncMap()).
		Parse(tmpl)
	if err != nil {
		return "", xerrors.Errorf("parse prompt template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", xerrors.Errorf("execute prompt template: %w", err)
	}
	rendered := strings.TrimSpace(buf.String())
	if rendered == "" {
		return "", xerrors.New("prompt template rendered to empty string")
	}
	return rendered, nil
}
