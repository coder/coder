package cli

import (
	"encoding/json"

	"github.com/tidwall/gjson"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/serpent"
)

func externalAuth() *serpent.Command {
	return &serpent.Command{
		Use:   "external-auth",
		Short: "Manage external authentication",
		Long:  "Authenticate with external services inside of a workspace.",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*serpent.Command{
			externalAuthAccessToken(),
		},
	}
}

func externalAuthAccessToken() *serpent.Command {
	var (
		extra        string
		outputFormat string
	)
	agentAuth := &AgentAuth{}
	cmd := &serpent.Command{
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
			Example{
				Description: "Print the full token response as JSON.",
				Command:     "coder external-auth access-token github --output json",
			},
		),
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{
			{
				Name:        "Extra",
				Flag:        "extra",
				Description: "Extract a field from the \"extra\" properties of the OAuth token.",
				Value:       serpent.StringOf(&extra),
			},
			{
				Name:        "Output",
				Flag:        "output",
				Description: "Output format. Available formats: text, json.",
				Value:       serpent.EnumOf(&outputFormat, "text", "json"),
				Default:     "text",
			},
		},

		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			ctx, stop := inv.SignalNotifyContext(ctx, StopSignals...)
			defer stop()

			client, err := agentAuth.CreateClient()
			if err != nil {
				return xerrors.Errorf("create agent client: %w", err)
			}

			extAuth, err := client.ExternalAuth(ctx, agentsdk.ExternalAuthRequest{
				ID: inv.Args[0],
			})
			if err != nil {
				return xerrors.Errorf("get external auth token: %w", err)
			}

			switch {
			case outputFormat == "json":
				data, err := json.MarshalIndent(extAuth, "", "  ")
				if err != nil {
					return xerrors.Errorf("marshal external auth response: %w", err)
				}
				if _, err := inv.Stdout.Write(data); err != nil {
					return err
				}
			case extAuth.URL != "":
				if _, err := inv.Stdout.Write([]byte(extAuth.URL)); err != nil {
					return err
				}
			case extra != "":
				if extAuth.TokenExtra == nil {
					return xerrors.Errorf("no extra properties found for token")
				}
				data, err := json.Marshal(extAuth.TokenExtra)
				if err != nil {
					return xerrors.Errorf("marshal extra properties: %w", err)
				}
				result := gjson.GetBytes(data, extra)
				if _, err := inv.Stdout.Write([]byte(result.String())); err != nil {
					return err
				}
			default:
				if _, err := inv.Stdout.Write([]byte(extAuth.AccessToken)); err != nil {
					return err
				}
			}

			if extAuth.URL != "" {
				return cliui.ErrCanceled
			}
			return nil
		},
	}
	agentAuth.AttachOptions(cmd, false)
	return cmd
}
