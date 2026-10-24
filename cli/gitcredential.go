package cli

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/gitauth"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/serpent"
)

// gitCredential is a Git credential helper that returns external auth tokens
// for HTTPS remotes. The agent registers it alongside GIT_ASKPASS because
// some programs disable askpass for the Git processes they spawn (for example
// Claude Code when it clones plugin marketplaces), while leaving credential
// helpers enabled.
//
// Unlike gitaskpass, it never waits for the user to authenticate. If the user
// hasn't linked the provider yet, it prints the authentication URL to stderr
// and returns no credentials, so Git falls through to the next helper or to
// GIT_ASKPASS, which still drives the interactive login flow.
//
// See https://git-scm.com/docs/gitcredentials#_custom_helpers.
func gitCredential() *serpent.Command {
	agentAuth := &AgentAuth{}
	cmd := &serpent.Command{
		Use:    "gitcredential <get|store|erase>",
		Hidden: true,
		Short:  "Git credential helper that provides external auth tokens from Coder",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			// Coder stores and refreshes the tokens, so only "get" does
			// anything. Drain stdin for the other operations so Git never
			// blocks writing the request.
			if inv.Args[0] != "get" {
				_, _ = io.Copy(io.Discard, inv.Stdin)
				return nil
			}

			ctx := inv.Context()
			ctx, stop := inv.SignalNotifyContext(ctx, StopSignals...)
			defer stop()

			req, err := gitauth.ParseCredentialRequest(inv.Stdin)
			if err != nil {
				return err
			}
			match, ok := req.MatchURL()
			if !ok {
				return nil
			}

			client, err := agentAuth.CreateClient()
			if err != nil {
				return xerrors.Errorf("create agent client: %w", err)
			}

			workingDirectory, err := os.Getwd()
			if err != nil {
				workingDirectory = ""
			}
			gitBranch, gitRemoteOrigin := detectGitRef(workingDirectory)

			token, err := client.ExternalAuth(ctx, agentsdk.ExternalAuthRequest{
				Match:           match,
				GitBranch:       gitBranch,
				GitRemoteOrigin: gitRemoteOrigin,
				ChatID:          inv.Environ.Get("CODER_CHAT_ID"),
			})
			if err != nil {
				var apiError *codersdk.Error
				if errors.As(err, &apiError) && apiError.StatusCode() == http.StatusNotFound {
					// No provider serves this host. Stay quiet so Git
					// moves on to its other credential sources.
					return nil
				}
				return xerrors.Errorf("get git token: %w", err)
			}
			if token.URL != "" {
				cliui.Warn(inv.Stderr, "Authenticate with your Git provider in Coder, then retry:", token.URL)
				return nil
			}

			// Mirror gitaskpass: providers that only set Username (such as
			// GitHub) answer both the username and password prompts with it.
			username, password := token.Username, token.Password
			if password == "" {
				password = username
			}
			if username == "" {
				return nil
			}

			_, _ = fmt.Fprintf(inv.Stdout, "username=%s\n", username)
			_, _ = fmt.Fprintf(inv.Stdout, "password=%s\n", password)
			if !token.ExpiresAt.IsZero() {
				// Lets Git and caching helpers drop the token when it
				// rotates instead of reusing a stale one (Git 2.34+).
				_, _ = fmt.Fprintf(inv.Stdout, "password_expiry_utc=%d\n", token.ExpiresAt.Unix())
			}
			return nil
		},
	}
	agentAuth.AttachOptions(cmd, false)
	return cmd
}
