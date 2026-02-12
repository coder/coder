package cli

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/fido2"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) webauthn() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "webauthn",
		Short: "Manage WebAuthn security key credentials",
		Long: FormatExamples(
			Example{
				Description: "Register a new security key",
				Command:     "coder webauthn register",
			},
			Example{
				Description: "List registered security keys",
				Command:     "coder webauthn list",
			},
		),
		Aliases: []string{"fido2"},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.webauthnRegister(),
			r.webauthnList(),
			r.webauthnDelete(),
		},
	}
	return cmd
}

func (r *RootCmd) webauthnRegister() *serpent.Command {
	var credName string
	cmd := &serpent.Command{
		Use:   "register",
		Short: "Register a FIDO2 security key for workspace connections",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			if !fido2.IsHelperInstalled() {
				return xerrors.New("coder-fido2 helper not found on PATH; install it first")
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			if credName == "" {
				credName, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text: "Name for this security key:",
				})
				if err != nil {
					return err
				}
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Starting WebAuthn registration...")

			// Step 1: Begin registration on the server.
			creation, err := client.BeginWebAuthnRegistration(inv.Context(), codersdk.Me)
			if err != nil {
				return xerrors.Errorf("begin registration: %w", err)
			}

			// Marshal the creation options to send to the helper.
			creationJSON, err := json.Marshal(creation)
			if err != nil {
				return xerrors.Errorf("marshal creation options: %w", err)
			}

			// The origin must match the server's RP origin.
			origin := client.URL.String()

			// Step 2: Shell out to the FIDO2 helper for the
			// attestation ceremony (touch the key).
			attestation, err := fido2RunWithRetry(inv, fido2.RunRegister, creationJSON, origin)
			if err != nil {
				return xerrors.Errorf("FIDO2 registration: %w", err)
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Touch detected! Completing registration...")

			// Step 3: Finish registration on the server.
			cred, err := client.FinishWebAuthnRegistration(
				inv.Context(), codersdk.Me, credName, attestation,
			)
			if err != nil {
				return xerrors.Errorf("finish registration: %w", err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Security key %q registered (ID: %s)\n",
				cred.Name, cred.ID)
			return nil
		},
	}
	cmd.Options = serpent.OptionSet{
		{
			Flag:        "name",
			Description: "Name for the security key credential.",
			Value:       serpent.StringOf(&credName),
		},
	}
	return cmd
}

func (r *RootCmd) webauthnList() *serpent.Command {
	cmd := &serpent.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List registered security keys",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			creds, err := client.ListWebAuthnCredentials(inv.Context(), codersdk.Me)
			if err != nil {
				return xerrors.Errorf("list credentials: %w", err)
			}

			if len(creds) == 0 {
				_, _ = fmt.Fprintln(inv.Stdout, "No security keys registered.")
				return nil
			}

			for _, c := range creds {
				lastUsed := "never"
				if c.LastUsedAt != nil {
					lastUsed = c.LastUsedAt.Format("2006-01-02 15:04:05")
				}
				_, _ = fmt.Fprintf(inv.Stdout, "%-36s  %-20s  created: %s  last used: %s\n",
					c.ID, c.Name, c.CreatedAt.Format("2006-01-02 15:04:05"), lastUsed)
			}
			return nil
		},
	}
	return cmd
}

func (r *RootCmd) webauthnDelete() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "delete <credential-id>",
		Short: "Delete a registered security key",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			credID, err := uuid.Parse(inv.Args[0])
			if err != nil {
				return xerrors.Errorf("invalid credential ID: %w", err)
			}

			err = client.DeleteWebAuthnCredential(inv.Context(), codersdk.Me, credID)
			if err != nil {
				return xerrors.Errorf("delete credential: %w", err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Credential %s deleted.\n", credID)
			return nil
		},
	}
	return cmd
}

// fido2RunWithRetry runs a FIDO2 helper function with retry on
// touch timeout (up to 3 attempts) and PIN prompting. The fn
// signature matches fido2.RunRegister and fido2.RunAssert.
func fido2RunWithRetry(inv *serpent.Invocation, fn func(json.RawMessage, string, string) (json.RawMessage, error), optionsJSON json.RawMessage, origin string) (json.RawMessage, error) {
	var pin string
	for attempt := 0; attempt < 3; attempt++ {
		result, err := fn(optionsJSON, pin, origin)
		if err == nil {
			return result, nil
		}
		if xerrors.Is(err, fido2.ErrTouchTimeout) {
			_, _ = fmt.Fprintln(inv.Stderr, "Touch timed out, try again...")
			continue
		}
		if xerrors.Is(err, fido2.ErrPinRequired) {
			var promptErr error
			pin, promptErr = cliui.Prompt(inv, cliui.PromptOptions{
				Text:   "Security key PIN:",
				Secret: true,
			})
			if promptErr != nil {
				return nil, promptErr
			}
			continue
		}
		return nil, err
	}
	return nil, xerrors.New("FIDO2 operation failed after 3 attempts")
}
