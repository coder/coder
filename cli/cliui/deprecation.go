package cliui

import (
	"fmt"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/pretty"
)

func DeprecationWarning(message string) clibase.MiddlewareFunc {
	return func(next clibase.HandlerFunc) clibase.HandlerFunc {
		return func(i *clibase.Invocation) error {
			_, _ = fmt.Fprintln(i.Stdout, "\n"+pretty.Sprint(DefaultStyles.Wrap,
				pretty.Sprint(
					DefaultStyles.Warn,
					"DEPRECATION WARNING: This command will be removed in a future release."+"\n"+message+"\n"),
			))
			return next(i)
		}
	}
}
