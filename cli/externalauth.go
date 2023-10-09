package cli

import (
	"os/signal"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

func (r *RootCmd) externalAuth() *clibase.Cmd {
	return &clibase.Cmd{
		Use:   "external-auth",
		Short: "Manage external authentication",
		Long:  "Authenticate with external services inside of a workspace.",
		Handler: func(i *clibase.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*clibase.Cmd{
			r.externalAuthAccessToken(),
		},
	}
}

func (r *RootCmd) externalAuthAccessToken() *clibase.Cmd {
	var silent bool
	return &clibase.Cmd{
		Use:   "access-token <provider>",
		Short: "Print auth for an external provider",
		Long: "Print an access-token for an external auth provider. " +
			"The access-token will be validated and sent to stdout with exit code 0. " +
			"If a valid access-token cannot be obtained, the URL to authenticate will be sent to stdout with exit code 1\n" + formatExamples(
			example{
				Description: "Ensure that the user is authenticated with GitHub before cloning.",
				Command: `#!/usr/bin/env sh

OUTPUT=$(coder external-auth access-token github)
if [ $? -eq 0 ]; then
  echo "Authenticated with GitHub"
else
  echo "Please authenticate with GitHub:"
  echo $OUTPUT
fi
`,
			},
		),
		Options: clibase.OptionSet{{
			Name:        "Silent",
			Flag:        "s",
			Description: "Do not print the URL or access token.",
			Value:       clibase.BoolOf(&silent),
		}},

		Handler: func(inv *clibase.Invocation) error {
			ctx := inv.Context()

			ctx, stop := signal.NotifyContext(ctx, InterruptSignals...)
			defer stop()

			client, err := r.createAgentClient()
			if err != nil {
				return xerrors.Errorf("create agent client: %w", err)
			}

			token, err := client.ExternalAuth(ctx, agentsdk.ExternalAuthRequest{
				ID: inv.Args[0],
			})
			if err != nil {
				return xerrors.Errorf("get external auth token: %w", err)
			}

			if !silent {
				if token.URL != "" {
					_, err = inv.Stdout.Write([]byte(token.URL))
				} else {
					_, err = inv.Stdout.Write([]byte(token.AccessToken))
				}
				if err != nil {
					return err
				}
			}

			if token.URL != "" {
				return cliui.Canceled
			}
			return nil
		},
	}
}
