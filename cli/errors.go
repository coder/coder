package cli

import (
	"io"
	"os"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/codersdk"
	"golang.org/x/xerrors"
)

func (r *RootCmd) errorExample() *clibase.Cmd {
	errorCmd := func(use string, err error) *clibase.Cmd {
		return &clibase.Cmd{
			Use: use,
			Handler: func(inv *clibase.Invocation) error {
				return err
			},
		}
	}

	cmd := &clibase.Cmd{
		Use:   "example-error",
		Short: "Shows what different error messages look like",
		Long: "This command is pretty pointless, but without it testing errors is" +
			"difficult to visually inspect. Error message formatting is inherently" +
			"visual, so we need a way to quickly see what they look like.",
		Handler: func(inv *clibase.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*clibase.Cmd{
			// Typical codersdk error
			errorCmd("sdk", &codersdk.Error{
				Response: codersdk.Response{
					Message: "Top level sdk error message.",
					Detail:  "magic dust unavailable, please try again later",
					Validations: []codersdk.ValidationError{
						{
							Field:  "region",
							Detail: "magic dust is not available in your region",
						},
					},
				},
				Helper: "Have you tried turning it off and on again?",
			}),

			// Typical cli error
			errorCmd("cmd", xerrors.Errorf("some error: %w", errorWithStackTrace())),

			// A multi-error
			{
				Use: "multi-error",
				Handler: func(inv *clibase.Invocation) error {
					// Closing the stdin file descriptor will cause the next close
					// to fail. This is joined to the returned Command error.
					if f, ok := inv.Stdin.(*os.File); ok {
						_ = f.Close()
					}

					return xerrors.Errorf("some error: %w", errorWithStackTrace())
				},
			},
		},
	}

	return cmd
}

type errorClose struct {
	io.ReadCloser
}

func (e errorClose) Close() error {
	err := e.ReadCloser.Close()
	if err == nil {
		return xerrors.Errorf("always close error")
	}
	return err
}

func errorWithStackTrace() error {
	return xerrors.Errorf("function decided not to work, and it never will")
}
