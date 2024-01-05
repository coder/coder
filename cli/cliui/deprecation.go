package cliui

import (
	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/pretty"
)

func DeprecationWarning(message string) clibase.MiddlewareFunc {
	return func(next clibase.HandlerFunc) clibase.HandlerFunc {
		return func(i *clibase.Invocation) error {
			pretty.Sprint(
				DefaultStyles.Warn,
				"DEPRECATION WARNING: This command will be removed in a future release. \n"+message+"\n")
			return next(i)
		}
	}
}
