package main

import (
	"fmt"
	"os/exec"

	"golang.org/x/xerrors"

	"github.com/coder/serpent"
)

const (
	owner = "coder"
	repo  = "coder"
)

// Run executes the interactive release wizard.
//
// It verifies dependencies, warns when the gh CLI is not configured, wires
// up a live or dry-run executor, and then walks the operator through
// tagging, pushing, and triggering the release workflow.
//
//nolint:revive // dryRun selects the dry-run executor for the wizard.
func Run(inv *serpent.Invocation, dryRun bool) error {
	ctx := inv.Context()
	w := inv.Stderr

	// --- Check dependencies ---
	if _, err := exec.LookPath("git"); err != nil {
		return xerrors.New("git is required but not found in PATH")
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

	return runRelease(ctx, inv, executor, ghAvailable, dryRun)
}
