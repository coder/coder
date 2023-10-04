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
			"If the user has a refresh-token, the access-token automatically refresh the access-token. " +
			"If the user has not authenticated before, ",
	}
}
