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

// releaseOptions holds the releaser command-line flags.
type releaseOptions struct {
	// DryRun prints mutating commands instead of executing them.
	DryRun bool
	// NonInteractive answers every prompt with its default.
	NonInteractive bool
	// AllowOpenPRs continues when open PRs target the release branch.
	AllowOpenPRs bool
	// AllowBreakingChanges continues when a patch release contains
	// breaking changes.
	AllowBreakingChanges bool
	// RequestedBy is the GitHub login of the user who asked for the
	// release when the releaser runs on their behalf, such as from the
	// Tag release workflow.
	RequestedBy string
	// RequestedByRunURL links to the run that asked for the release.
	RequestedByRunURL string
}

// Run executes the interactive release wizard.
//
// It verifies dependencies, warns when the gh CLI is not configured, wires
// up a live or dry-run executor, and then walks the operator through
// tagging, pushing, and triggering the release workflow.
func Run(inv *serpent.Invocation, opts releaseOptions) error {
	ctx := inv.Context()
	w := inv.Stderr

	// --- Check dependencies ---
	if _, err := exec.LookPath("git"); err != nil {
		return xerrors.New("git is required but not found in PATH")
	}

	// --- Check gh CLI auth ---
	ghAvailable := checkGHAuth()
	if !ghAvailable {
		// Without gh, the open PR and breaking change checks are
		// skipped, so an unattended release must not proceed.
		if opts.NonInteractive {
			return xerrors.New("gh CLI is not available or not authenticated, which non-interactive mode requires")
		}
		warnf(w, "gh CLI is not available or not authenticated.")
		infof(w, "Continuing without GitHub features (PR checks, label lookups, workflow trigger).")
		_, _ = fmt.Fprintln(w)
	}

	// --- Wire up executor ---
	var executor ReleaseExecutor
	if opts.DryRun {
		outputPrefix = "[DRYRUN] "
		executor = &dryRunExecutor{w: w}
	} else {
		executor = &liveExecutor{requestedBy: opts.RequestedBy, requestedByRunURL: opts.RequestedByRunURL}
	}

	return runRelease(ctx, inv, executor, ghAvailable, opts)
}
