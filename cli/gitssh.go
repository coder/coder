package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func (r *RootCmd) gitssh() *serpent.Cmd {
	cmd := &serpent.Cmd{
		Use:    "gitssh",
		Hidden: true,
		Short:  `Wraps the "ssh" command and uses the coder gitssh key for authentication`,
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			env := os.Environ()

			// Catch interrupt signals to ensure the temporary private
			// key file is cleaned up on most cases.
			ctx, stop := inv.SignalNotifyContext(ctx, StopSignals...)
			defer stop()

			// Early check so errors are reported immediately.
			identityFiles, err := parseIdentityFilesForHost(ctx, inv.Args, env)
			if err != nil {
				return err
			}

			client, err := r.createAgentClient()
			if err != nil {
				return xerrors.Errorf("create agent client: %w", err)
			}
			key, err := client.GitSSHKey(ctx)
			if err != nil {
				return xerrors.Errorf("get agent git ssh token: %w", err)
			}

			privateKeyFile, err := os.CreateTemp("", "coder-gitsshkey-*")
			if err != nil {
				return xerrors.Errorf("create temp gitsshkey file: %w", err)
			}
			defer func() {
				_ = privateKeyFile.Close()
				_ = os.Remove(privateKeyFile.Name())
			}()
			_, err = privateKeyFile.WriteString(key.PrivateKey)
			if err != nil {
				return xerrors.Errorf("write to temp gitsshkey file: %w", err)
			}
			err = privateKeyFile.Close()
			if err != nil {
				return xerrors.Errorf("close temp gitsshkey file: %w", err)
			}

			// Append our key, giving precedence to user keys. Note that
			// OpenSSH server are typically configured with MaxAuthTries
			// set to the default value of 6. This means that only the 6
			// first keys can be tried. However, we will assume that if
			// a user has configured 6+ keys for a host, they know what
			// they're doing. This behavior is critical if a server has
			// been configured with MaxAuthTries set to 1.
			identityFiles = append(identityFiles, privateKeyFile.Name())

			var identityArgs []string
			for _, id := range identityFiles {
				identityArgs = append(identityArgs, "-i", id)
			}

			args := inv.Args
			args = append(identityArgs, args...)
			c := exec.CommandContext(ctx, "ssh", args...)
			c.Env = append(c.Env, env...)
			c.Stderr = inv.Stderr
			c.Stdout = inv.Stdout
			c.Stdin = inv.Stdin
			err = c.Run()
			if err != nil {
				exitErr := &exec.ExitError{}
				if xerrors.As(err, &exitErr) && exitErr.ExitCode() == 255 {
					_, _ = fmt.Fprintln(inv.Stderr,
						"\n"+pretty.Sprintf(
							cliui.DefaultStyles.Wrap,
							"Coder authenticates with "+pretty.Sprint(cliui.DefaultStyles.Field, "git")+
								" using the public key below. All clones with SSH are authenticated automatically 🪄.")+"\n",
					)
					_, _ = fmt.Fprintln(inv.Stderr, pretty.Sprint(cliui.DefaultStyles.Code, strings.TrimSpace(key.PublicKey))+"\n")
					_, _ = fmt.Fprintln(inv.Stderr, "Add to GitHub and GitLab:")
					pretty.Fprintf(inv.Stderr, cliui.DefaultStyles.Prompt, "%s", "https://github.com/settings/ssh/new\n\n")
					pretty.Fprintf(inv.Stderr, cliui.DefaultStyles.Prompt, "%s", "https://gitlab.com/-/profile/keys\n\n")
					_, _ = fmt.Fprintln(inv.Stderr)
					return err
				}
				return xerrors.Errorf("run ssh command: %w", err)
			}

			return nil
		},
	}

	return cmd
}

// fallbackIdentityFiles is the list of identity files SSH tries when
// none have been defined for a host.
var fallbackIdentityFiles = strings.Join([]string{
	"identityfile ~/.ssh/id_rsa",
	"identityfile ~/.ssh/id_dsa",
	"identityfile ~/.ssh/id_ecdsa",
	"identityfile ~/.ssh/id_ecdsa_sk",
	"identityfile ~/.ssh/id_ed25519",
	"identityfile ~/.ssh/id_ed25519_sk",
	"identityfile ~/.ssh/id_xmss",
}, "\n")

// parseIdentityFilesForHost uses ssh -G to discern what SSH keys have
// been enabled for the host (via the users SSH config) and returns a
// list of existing identity files.
//
// We do this because when no keys are defined for a host, SSH uses
// fallback keys (see above). However, by passing `-i` to attach our
// private key, we're effectively disabling the fallback keys.
//
// Example invocation:
//
//	ssh -G -o SendEnv=GIT_PROTOCOL git@github.com git-upload-pack 'coder/coder'
//
// The extra arguments work without issue and lets us run the command
// as-is without stripping out the excess (git-upload-pack 'coder/coder').
func parseIdentityFilesForHost(ctx context.Context, args, env []string) (identityFiles []string, error error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, xerrors.Errorf("get user home dir failed: %w", err)
	}

	var outBuf bytes.Buffer
	var r io.Reader = &outBuf

	args = append([]string{"-G"}, args...)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Env = append(cmd.Env, env...)
	cmd.Stdout = &outBuf
	cmd.Stderr = io.Discard
	err = cmd.Run()
	if err != nil {
		// If ssh -G failed, the SSH version is likely too old, fallback
		// to using the default identity files.
		r = strings.NewReader(fallbackIdentityFiles)
	}

	s := bufio.NewScanner(r)
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "identityfile ") {
			id := strings.TrimPrefix(line, "identityfile ")
			if strings.HasPrefix(id, "~/") {
				id = home + id[1:]
			}
			// OpenSSH on Windows is weird, it supports using (and does
			// use) mixed \ and / in paths.
			//
			// Example: C:\Users\ZeroCool/.ssh/known_hosts
			//
			// To check the file existence in Go, though, we want to use
			// proper Windows paths.
			// OpenSSH is amazing, this will work on Windows too:
			// C:\Users\ZeroCool/.ssh/id_rsa
			id = filepath.FromSlash(id)

			// Only include the identity file if it exists.
			if _, err := os.Stat(id); err == nil {
				identityFiles = append(identityFiles, id)
			}
		}
	}
	if err := s.Err(); err != nil {
		// This should never happen, the check is for completeness.
		return nil, xerrors.Errorf("scan ssh output: %w", err)
	}

	return identityFiles, nil
}
