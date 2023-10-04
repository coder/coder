package cli

import "github.com/coder/coder/v2/cli/clibase"

func (r *RootCmd) externalAuth() *clibase.Cmd {
	return &clibase.Cmd{
		Use:   "external-auth",
		Short: "Manage external authentication",
		Long:  "Authenticate with external services inside of a workspace.",
		Handler: func(i *clibase.Invocation) error {
			return i.Command.HelpHandler(i)
		},
	}
}

func (r *RootCmd) externalAuthList() *clibase.Cmd {
	return &clibase.Cmd{
		Use:   "list",
		Short: "List external authentication providers",
		Long:  "List external authentication.",
		Handler: func(i *clibase.Invocation) error {
			return i.Command.HelpHandler(i)
		},
	}
}

func (r *RootCmd) externalAuthAccessToken() *clibase.Cmd {
	return &clibase.Cmd{
		Use:   "access-token",
		Short: "Print auth for an external provider",
		Long: "Print an access-token for an external auth provider. " +
			"The access-token will be validated and sent to stdout with exit code 0. " +
			"If a valid access-token cannot be obtained, the URL to authenticate will be sent to stderr with exit code 1\n" + formatExamples(
			example{
				Description: "Ensure that the user is authenticated with GitHub before cloning.",
				Command: `#!/usr/bin/env sh

if coder external-auth access-token github ; then
  echo "Authenticated with GitHub"
else
  echo "Please authenticate with GitHub:"
  coder external-auth url github
fi
`,
			},
		),
		Options: clibase.OptionSet{{
			Name:        "Silent",
			Flag:        "s",
			Description: "Do not print the URL or access token.",
		}},

		Handler: func(i *clibase.Invocation) error {
			return nil
		},
	}
}
