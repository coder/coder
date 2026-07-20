package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) oauth2Provider() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "oauth2-provider",
		Short: "Manage Coder OAuth2 provider settings",
		Long: "Administrators can use these commands to change OAuth2 provider settings.\n" + FormatExamples(
			Example{
				Description: "Enable dynamic client registration (RFC 7591), allowing OAuth2/MCP clients to self-register without an admin creating an app first",
				Command:     "coder oauth2-provider dcr enable",
			},
			Example{
				Description: "Disable dynamic client registration. Clients that already registered are unaffected; only new self-registration attempts are rejected",
				Command:     "coder oauth2-provider dcr disable",
			},
		),
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.oauth2ProviderDCR(),
		},
	}
	return cmd
}

func (r *RootCmd) oauth2ProviderDCR() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "dcr",
		Short: "Manage OAuth2 dynamic client registration (RFC 7591)",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.enableOAuth2ProviderDCR(),
			r.disableOAuth2ProviderDCR(),
		},
	}
	return cmd
}

func (r *RootCmd) enableOAuth2ProviderDCR() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "enable",
		Short: "Enable OAuth2 dynamic client registration",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			_, err = client.PutOAuth2ProviderSettings(inv.Context(), codersdk.OAuth2ProviderSettings{
				DynamicClientRegistrationEnabled: true,
			})
			if err != nil {
				return xerrors.Errorf("unable to enable dynamic client registration: %w", err)
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Dynamic client registration is now enabled.")
			return nil
		},
	}
	return cmd
}

func (r *RootCmd) disableOAuth2ProviderDCR() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "disable",
		Short: "Disable OAuth2 dynamic client registration",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			_, err = client.PutOAuth2ProviderSettings(inv.Context(), codersdk.OAuth2ProviderSettings{
				DynamicClientRegistrationEnabled: false,
			})
			if err != nil {
				return xerrors.Errorf("unable to disable dynamic client registration: %w", err)
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Dynamic client registration is now disabled.")
			return nil
		},
	}
	return cmd
}
