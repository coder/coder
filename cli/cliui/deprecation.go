package cliui

import (
	"fmt"

	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func DeprecationWarning(message string) serpent.MiddlewareFunc {
	return func(next serpent.HandlerFunc) serpent.HandlerFunc {
		return func(i *serpent.Invocation) error {
			_, _ = fmt.Fprintln(i.Stdout, "\n"+pretty.Sprint(DefaultStyles.Wrap,
				pretty.Sprint(
					DefaultStyles.Warn,
					"DEPRECATION WARNING: This command will be removed in a future release."+"\n"+message+"\n"),
			))
			return next(i)
		}
	}
}
