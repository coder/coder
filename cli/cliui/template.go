package cliui

import (
	"fmt"
	"io"
	"time"

	"github.com/coder/pretty"
)

// WriteTemplateCreateHint writes a standardized banner and example command for creating
// a workspace from a newly created template.
//
// We accept the current time as a parameter instead of calling time.Now() internally so
// that callers (and tests) can provide a fixed time value, making output deterministic
// and easy to assert in golden tests.
func WriteTemplateCreateHint(w io.Writer, templateName, organizationName string, t time.Time) {
	_, _ = fmt.Fprintln(w, "\n"+Wrap(
		"The "+Keyword(templateName)+" template has been created at "+Timestamp(t)+"! "+
			"Developers can provision a workspace with this template using:")+"\n")

	_, _ = fmt.Fprintln(w, "  "+pretty.Sprint(
		DefaultStyles.Code,
		fmt.Sprintf("coder create --template=%q --org=%q [workspace name]", templateName, organizationName),
	))
	_, _ = fmt.Fprintln(w)
}
