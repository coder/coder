package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/codersdk"
)

func (RootCmd) errorExample() *clibase.Cmd {
	errorCmd := func(use string, err error) *clibase.Cmd {
		return &clibase.Cmd{
			Use: use,
			Handler: func(inv *clibase.Invocation) error {
				return err
			},
		}
	}

	// Make an api error
	recorder := httptest.NewRecorder()
	recorder.WriteHeader(http.StatusBadRequest)
	resp := recorder.Result()
	_ = resp.Body.Close()
	resp.Request, _ = http.NewRequest(http.MethodPost, "http://example.com", nil)
	apiError := codersdk.ReadBodyAsError(resp)
	//nolint:errorlint,forcetypeassert
	apiError.(*codersdk.Error).Response = codersdk.Response{
		Message: "Top level sdk error message.",
		Detail:  "magic dust unavailable, please try again later",
		Validations: []codersdk.ValidationError{
			{
				Field:  "region",
				Detail: "magic dust is not available in your region",
			},
		},
	}
	//nolint:errorlint,forcetypeassert
	apiError.(*codersdk.Error).Helper = "Have you tried turning it off and on again?"

	// Some flags
	var magicWord clibase.String

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
			// Typical codersdk api error
			errorCmd("api", apiError),

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

			{
				Use: "validation",
				Options: clibase.OptionSet{
					clibase.Option{
						Name:        "magic-word",
						Description: "Take a good guess.",
						Required:    true,
						Flag:        "magic-word",
						Default:     "",
						Value: clibase.Validate(&magicWord, func(value *clibase.String) error {
							return xerrors.Errorf("magic word is incorrect")
						}),
					},
				},
				Handler: func(i *clibase.Invocation) error {
					_, _ = fmt.Fprint(i.Stdout, "Try setting the --magic-word flag\n")
					return nil
				},
			},
		},
	}

	return cmd
}

func errorWithStackTrace() error {
	return xerrors.Errorf("function decided not to work, and it never will")
}
