package cli

import (
	"encoding/json"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/tidwall/gjson"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk/agentsdk"
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
			r.externalAuthLink(),
		},
	}
}

func (r *RootCmd) externalAuthLink() *serpent.Command {
	var (
		matchURL  string
		only      string
		formatter = cliui.NewOutputFormatter(
			cliui.ChangeFormatterData(cliui.TextFormat(), func(data any) (any, error) {
				auth, ok := data.(agentsdk.ExternalAuthResponse)
				if !ok {
					return nil, xerrors.Errorf("expected data to be of type codersdk.ExternalAuth, got %T", data)
				}

				// If the url is set, only return that. It will be accompanied by an error message.
				// This is helpful for scripts to take this output and give it to the user.
				if auth.URL != "" {
					return auth.URL, nil
				}

				switch only {
				case "access_token":
					return auth.AccessToken, nil
				case "refresh_token":
					return auth.RefreshToken, nil
				default:
					refresh := ""
					if auth.RefreshToken != "" {
						refresh = fmt.Sprintf("\nrefresh_token: %s", auth.RefreshToken)
					}
					return fmt.Sprintf("type: %s\naccess_token: %s%s", auth.Type, auth.AccessToken, refresh), nil
				}
			},
			),
			// Table expects a slice of data.
			cliui.ChangeFormatterData(cliui.TableFormat([]agentsdk.ExternalAuthResponse{}, []string{"type", "access_token"}), func(data any) (any, error) {
				auth, ok := data.(agentsdk.ExternalAuthResponse)
				if !ok {
					return nil, xerrors.Errorf("expected data to be of type codersdk.ExternalAuth, got %T", data)
				}
				return []agentsdk.ExternalAuthResponse{auth}, nil
			}),
			cliui.JSONFormat(),
		)
	)

	cmd := &serpent.Command{
		Use:   "link <provider>",
		Short: "Print auth link for an external provider by ID or match regex.",
		Long: "Print auth link for an external provider by ID or match regex. " +
			"Use the flags to tailor the output to your needs. " +
			"If a valid access-token cannot be obtained, the URL to authenticate will be sent to stdout with exit code 1\n" + formatExamples(
			example{
				Description: "Only print the access token",
				Command:     "external-auth link <provider_id> --only access_token",
			},
			example{
				Description: "Dump auth link as json",
				Command:     "external-auth link <provider_id> --o json",
			},
		),
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(0, 1),
		),
		Options: serpent.OptionSet{
			{
				Name:        "Match",
				Flag:        "match",
				Description: "Match a provider with a url. If a provider has a regex that matches the url, the provider will be returned.",
				Value:       serpent.StringOf(&matchURL),
			},
			{
				Name:        "Only",
				Flag:        "only",
				Description: "Only return the specified field from the external auth response. Only works with text response.",
				Value:       serpent.EnumOf(&only, "access_token", "refresh_token"),
			},
		},

		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			req := agentsdk.ExternalAuthRequest{
				Match: matchURL,
			}

			if len(inv.Args) > 0 && matchURL != "" {
				return xerrors.Errorf("cannot specify both provider id and --match")
			}

			if matchURL == "" {
				if len(inv.Args) == 0 {
					return xerrors.Errorf("missing provider argument")
				}
				req.ID = inv.Args[0]
			}

			ctx, stop := inv.SignalNotifyContext(ctx, StopSignals...)
			defer stop()

			client, err := r.createAgentClient()
			if err != nil {
				return xerrors.Errorf("create agent client: %w", err)
			}

			extAuth, err := client.ExternalAuth(ctx, req)
			if err != nil {
				return xerrors.Errorf("get external auth token: %w", err)
			}

			// We always write to the output because if the URL field is
			// populated, we still want that information sent to the user.
			out, err := formatter.Format(inv.Context(), extAuth)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(inv.Stdout, out)

			// If the URL field is set, we need to accompany the output with the
			// authentication error.
			if extAuth.URL != "" {
				// The text & json outputs will have the url. The table one will not,
				// but the error message will. So this is ok.
				return xerrors.Errorf("external auth requires login, visit %s", extAuth.URL)
			}

			return nil
		},
	}
	formatter.AttachOptions(&cmd.Options)

	return cmd
}

func (r *RootCmd) externalAuthAccessToken() *serpent.Command {

	var extra string
	return &serpent.Command{
		Use:   "access-token <provider>",
		Short: "Use 'link <provider> --only access_token' instead. Will print auth for an external provider",
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
			example{
				Description: "Obtain an extra property of an access token for additional metadata.",
				Command:     "coder external-auth access-token slack --extra \"authed_user.id\"",
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
		},

		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			ctx, stop := inv.SignalNotifyContext(ctx, StopSignals...)
			defer stop()

			client, err := r.createAgentClient()
			if err != nil {
				return xerrors.Errorf("create agent client: %w", err)
			}

			extAuth, err := client.ExternalAuth(ctx, agentsdk.ExternalAuthRequest{
				ID: inv.Args[0],
			})
			if err != nil {
				return xerrors.Errorf("get external auth token: %w", err)
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
					return xerrors.Errorf("no extra properties found for token")
				}
				data, err := json.Marshal(extAuth.TokenExtra)
				if err != nil {
					return xerrors.Errorf("marshal extra properties: %w", err)
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
