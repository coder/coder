package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/xerrors"
)

// ReleaseExecutor handles dangerous write/mutating operations
// that should be skipped in dry-run mode. Only actions that
// modify the git repo or trigger external side effects belong
// here. Safe operations (file writes, fetches, editor) are
// called directly.
type ReleaseExecutor interface {
	// TriggerWorkflow dispatches the release.yaml GitHub Actions
	// workflow on branch with the given inputs. The workflow creates
	// the release tag at commit once its release environment is
	// approved: only the release GitHub App may create tags.
	TriggerWorkflow(ctx context.Context, branch, version, commit, channel, releaseNotes string) error
}

// liveExecutor performs real operations.
type liveExecutor struct{}

func (*liveExecutor) TriggerWorkflow(_ context.Context, branch, version, commit, channel, releaseNotes string) error {
	payload := map[string]string{
		"dry_run":         "false",
		"release_channel": channel,
		"release_notes":   releaseNotes,
		"version":         version,
		"commit":          commit,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return xerrors.Errorf("marshaling workflow payload: %w", err)
	}
	cmd := exec.Command("gh", "workflow", "run", "release.yaml",
		"--repo", owner+"/"+repo,
		"--ref", branch,
		"--json",
	)
	cmd.Stdin = strings.NewReader(string(payloadJSON))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// dryRunExecutor prints what would happen without doing it.
type dryRunExecutor struct {
	w io.Writer
}

func (e *dryRunExecutor) TriggerWorkflow(_ context.Context, branch, version, commit, channel, _ string) error {
	_, _ = fmt.Fprintf(e.w, "[DRYRUN] would trigger release.yaml workflow (ref=%s, version=%s, commit=%s, channel=%s)\n", branch, version, commit, channel)
	return nil
}
