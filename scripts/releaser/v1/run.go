package v1

import (
	"fmt"
	"os/exec"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/serpent"
)

const (
	owner = "coder"
	repo  = "coder"
)

// Run executes the legacy interactive release wizard.
//
// It mirrors the behavior of the original standalone releaser tool: it
// verifies dependencies, warns when GPG signing or the gh CLI are not
// configured, wires up a live or dry-run executor, and then walks the
// operator through tagging, pushing, and triggering the release
// workflow.
//
//nolint:revive // dryRun selects the dry-run executor for the wizard.
func Run(inv *serpent.Invocation, dryRun bool) error {
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
		warnf(w, "GPG signing is not configured. Tags will be unsigned, so there will be no way to verify who pushed the tag.")
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
}
