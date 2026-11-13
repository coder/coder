package cli

import (
	"crypto/sha512"
	"errors"
	"os"
	"os/exec"

	"golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentgitsign"
	"github.com/coder/serpent"
)

// gitsignssh is invoked by git as gpg.ssh.program. For `-Y sign` it fetches
// the user's git SSH key from the agent API, as gitssh does, and signs the
// payload in memory, so the private key is never written to disk. Every other
// operation, such as verification, is passed to ssh-keygen unchanged.
func gitsignssh() *serpent.Command {
	agentAuth := &AgentAuth{}
	cmd := &serpent.Command{
		Use:    "gitsign-ssh",
		Hidden: true,
		Short:  `Signs git commits and tags with the coder gitssh key`,
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			ctx, stop := inv.SignalNotifyContext(ctx, StopSignals...)
			defer stop()

			namespace, payloadPath, ok := parseSignArgs(inv.Args)
			if !ok {
				//nolint:gosec // Arguments come from git, which runs this as gpg.ssh.program.
				c := exec.CommandContext(ctx, "ssh-keygen", inv.Args...)
				c.Stdin, c.Stdout, c.Stderr = inv.Stdin, inv.Stdout, inv.Stderr
				err := c.Run()
				// Git reads ssh-keygen's own output and exit code, for example
				// "No principal matched" while verifying, so pass the code
				// through without adding an error message of our own.
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					return ExitError(exitErr.ExitCode(), nil)
				}
				if err != nil {
					return xerrors.Errorf("run ssh-keygen: %w", err)
				}
				return nil
			}

			payload, err := os.ReadFile(payloadPath)
			if err != nil {
				return xerrors.Errorf("read payload: %w", err)
			}

			client, err := agentAuth.CreateClient()
			if err != nil {
				return xerrors.Errorf("create agent client: %w", err)
			}
			key, err := client.GitSSHKey(ctx)
			if err != nil {
				return xerrors.Errorf("get agent git ssh key: %w", err)
			}
			signer, err := ssh.ParsePrivateKey([]byte(key.PrivateKey))
			if err != nil {
				return xerrors.Errorf("parse git ssh key: %w", err)
			}
			digest := sha512.Sum512(payload)
			signature, err := agentgitsign.SignSSHSig(signer, namespace, "sha512", digest[:])
			if err != nil {
				return xerrors.Errorf("sign: %w", err)
			}
			// Git reads the signature from <payload>.sig, as ssh-keygen writes it.
			//nolint:gosec // A signature is not secret.
			if err := os.WriteFile(payloadPath+".sig", []byte(signature), 0o644); err != nil {
				return xerrors.Errorf("write signature: %w", err)
			}
			return nil
		},
	}
	agentAuth.AttachOptions(cmd, false)
	return cmd
}

// parseSignArgs extracts the namespace and payload file from the
// `ssh-keygen -Y sign -n <namespace> -f <key> [-U] <file>` arguments git
// passes. It reports false for any other operation.
func parseSignArgs(args []string) (namespace, payloadPath string, ok bool) {
	var operation string
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-Y", "-n", "-f", "-O":
			if i+1 >= len(args) {
				return "", "", false
			}
			switch args[i] {
			case "-Y":
				operation = args[i+1]
			case "-n":
				namespace = args[i+1]
			}
			i++
		case "-U":
		default:
			positional = append(positional, args[i])
		}
	}
	if operation != "sign" || namespace == "" || len(positional) != 1 {
		return "", "", false
	}
	return namespace, positional[0], true
}
