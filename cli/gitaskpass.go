package cli

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/gitauth"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/retry"
	"github.com/coder/serpent"
)

// detectGitRef attempts to resolve the current git branch and remote
// origin URL from the given working directory. These are sent to the
// control plane so it can look up PR/diff status via the GitHub API
// without SSHing into the workspace. Failures are silently ignored
// since this is best-effort.
func detectGitRef(workingDirectory string) (branch string, remoteOrigin string) {
	run := func(args ...string) string {
		//nolint:gosec
		cmd := exec.Command(args[0], args[1:]...)
		if workingDirectory != "" {
			cmd.Dir = workingDirectory
		}
		out, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	branch = run("git", "rev-parse", "--abbrev-ref", "HEAD")
	remoteOrigin = run("git", "config", "--get", "remote.origin.url")
	return branch, remoteOrigin
}

// gitAskpass is used by the Coder agent to automatically authenticate
// with Git providers based on a hostname.
func gitAskpass(agentAuth *AgentAuth) *serpent.Command {
	cmd := &serpent.Command{
		Use:    "gitaskpass",
		Hidden: true,
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			ctx, stop := inv.SignalNotifyContext(ctx, StopSignals...)
			defer stop()

			user, host, err := gitauth.ParseAskpass(inv.Args[0])
			if err != nil {
				return xerrors.Errorf("parse host: %w", err)
			}

			client, err := agentAuth.CreateClient()
			if err != nil {
				return xerrors.Errorf("create agent client: %w", err)
			}

			workingDirectory, err := os.Getwd()
			if err != nil {
				workingDirectory = ""
			}

			// Detect the current git branch and remote origin so
			// the control plane can resolve diffs without needing
			// to SSH back into the workspace.
			gitBranch, gitRemoteOrigin := detectGitRef(workingDirectory)

			token, err := client.ExternalAuth(ctx, agentsdk.ExternalAuthRequest{
				Match:           host,
				Workdir:         workingDirectory,
				GitBranch:       gitBranch,
				GitRemoteOrigin: gitRemoteOrigin,
			})
			if err != nil {
				var apiError *codersdk.Error
				if errors.As(err, &apiError) && apiError.StatusCode() == http.StatusNotFound {
					// This prevents the "Run 'coder --help' for usage"
					// message from occurring.
					lines := []string{apiError.Message}
					if apiError.Detail != "" {
						lines = append(lines, apiError.Detail)
					}
					cliui.Warn(inv.Stderr, "Coder was unable to handle this git request. The default git behavior will be used instead.",
						lines...,
					)
					return cliui.ErrCanceled
				}
				return xerrors.Errorf("get git token: %w", err)
			}
			if token.URL != "" {
				if err := openURL(inv, token.URL); err == nil {
					cliui.Infof(inv.Stderr, "Your browser has been opened to authenticate with Git:\n%s", token.URL)
				} else {
					cliui.Infof(inv.Stderr, "Open the following URL to authenticate with Git:\n%s", token.URL)
				}

				for r := retry.New(250*time.Millisecond, 10*time.Second); r.Wait(ctx); {
					token, err = client.ExternalAuth(ctx, agentsdk.ExternalAuthRequest{
						Match:   host,
						Listen:  true,
						Workdir: workingDirectory,
					})
					if err != nil {
						continue
					}
					cliui.Infof(inv.Stderr, "You've been authenticated with Git!")
					break
				}
			}

			if token.Password != "" {
				if user == "" {
					_, _ = fmt.Fprintln(inv.Stdout, token.Username)
				} else {
					_, _ = fmt.Fprintln(inv.Stdout, token.Password)
				}
			} else {
				_, _ = fmt.Fprintln(inv.Stdout, token.Username)
			}

			return nil
		},
	}
	agentAuth.AttachOptions(cmd, false)
	return cmd
}
