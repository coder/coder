package cli
import (
	"errors"
	"encoding/json"
	"fmt"
	"github.com/tidwall/gjson"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)
func (r *RootCmd) externalAuth() *serpent.Command {
	return &serpent.Command{
		Use:   "external-auth",
		Short: "Manage external authentication",
		Long:  "Authenticate with external services inside of a workspace.",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*serpent.Command{
			r.externalAuthAccessToken(),
		},
	}
}
func (r *RootCmd) externalAuthAccessToken() *serpent.Command {
	var extra string
	return &serpent.Command{
		Use:   "access-token <provider>",
		Short: "Print auth for an external provider",
		Long: "Print an access-token for an external auth provider. " +
			"The access-token will be validated and sent to stdout with exit code 0. " +
			"If a valid access-token cannot be obtained, the URL to authenticate will be sent to stdout with exit code 1\n" + FormatExamples(
			Example{
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
			Example{
				Description: "Obtain an extra property of an access token for additional metadata.",
				Command:     "coder external-auth access-token slack --extra \"authed_user.id\"",
			},
		),
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{{
			Name:        "Extra",
			Flag:        "extra",
			Description: "Extract a field from the \"extra\" properties of the OAuth token.",
			Value:       serpent.StringOf(&extra),
		}},
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			ctx, stop := inv.SignalNotifyContext(ctx, StopSignals...)
			defer stop()
			if r.agentToken == "" {
				_, _ = fmt.Fprint(inv.Stderr, pretty.Sprintf(headLineStyle(), "No agent token found, this command must be run from inside a running workspace.\n"))
				return fmt.Errorf("agent token not found")
			}
			client, err := r.createAgentClient()
			if err != nil {
				return fmt.Errorf("create agent client: %w", err)
			}
			extAuth, err := client.ExternalAuth(ctx, agentsdk.ExternalAuthRequest{
				ID: inv.Args[0],
			})
			if err != nil {
				return fmt.Errorf("get external auth token: %w", err)
			}
			if extAuth.URL != "" {
				_, err = inv.Stdout.Write([]byte(extAuth.URL))
				if err != nil {
					return err
				}
				return cliui.Canceled
			}
			if extra != "" {
				if extAuth.TokenExtra == nil {
					return fmt.Errorf("no extra properties found for token")
				}
				data, err := json.Marshal(extAuth.TokenExtra)
				if err != nil {
					return fmt.Errorf("marshal extra properties: %w", err)
				}
				result := gjson.GetBytes(data, extra)
				_, err = inv.Stdout.Write([]byte(result.String()))
				if err != nil {
					return err
				}
				return nil
			}
			_, err = inv.Stdout.Write([]byte(extAuth.AccessToken))
			if err != nil {
				return err
			}
			return nil
		},
	}
}
