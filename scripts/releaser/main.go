package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

const (
	owner = "coder"
	repo  = "coder"
)

func main() {
	var dryRun bool
	cmd := &serpent.Command{
		Use:   "releaser",
		Short: "Interactive release tagging for coder/coder.",
		Long:  "Tag RCs from main, releases/patches from release/X.Y. The tool detects the branch, infers the next version, and walks you through tagging, pushing, and triggering the release workflow.",
		Options: serpent.OptionSet{
			{
				Name:        "dry-run",
				Flag:        "dry-run",
				Description: "Print write commands instead of executing them.",
				Value:       serpent.BoolOf(&dryRun),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			w := inv.Stderr

			// --- Check dependencies ---
			if _, err := exec.LookPath("git"); err != nil {
				return xerrors.New("git is required but not found in PATH")
			}

			// --- Check GPG signing ---
			signingKey, _ := gitOutput("config", "--get", "user.signingkey")
			gpgFormat, _ := gitOutput("config", "--get", "gpg.format")
			gpgConfigured := signingKey != "" || gpgFormat != ""
			if !gpgConfigured {
				warnf(w, "GPG signing is not configured. Tags will be unsigned — there will be no way to verify who pushed the tag.")
				_, _ = fmt.Fprintf(w, "  To fix: set git config user.signingkey or gpg.format\n")
				if err := confirmWithDefault(inv, "Continue without signing?", cliui.ConfirmNo); err != nil {
					return err
				}
				_, _ = fmt.Fprintln(w)
			}

			// --- Check gh CLI auth ---
			ghAvailable := checkGHAuth()
			if !ghAvailable {
				warnf(w, "gh CLI is not available or not authenticated.")
				infof(w, "Continuing without GitHub features (PR checks, label lookups, workflow trigger).")
				_, _ = fmt.Fprintln(w)
			}

			// --- Wire up executor ---
			var executor ReleaseExecutor
			if dryRun {
				outputPrefix = "[DRYRUN] "
				executor = &dryRunExecutor{w: w}
			} else {
				executor = &liveExecutor{}
			}

			return runRelease(ctx, inv, executor, ghAvailable, gpgConfigured, dryRun)
		},
	}

	err := cmd.Invoke().WithOS().Run()
	if err != nil {
		if errors.Is(err, cliui.ErrCanceled) {
			os.Exit(1)
		}
		// Unwrap serpent's "running command ..." wrapper to
		// keep output clean.
		var runErr *serpent.RunCommandError
		if errors.As(err, &runErr) {
			err = runErr.Err
		}
		pretty.Fprintf(os.Stderr, cliui.DefaultStyles.Error, "Error: %s\n", err)
		os.Exit(1)
	}
}
