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
	// CreateTag creates an annotated (optionally signed) git tag.
	CreateTag(ctx context.Context, tag, ref, message string, sign bool) error
	// PushTag pushes a tag to the origin remote.
	PushTag(ctx context.Context, tag string) error
	// TriggerWorkflow dispatches the release.yaml GitHub Actions
	// workflow with the given inputs.
	TriggerWorkflow(ctx context.Context, ref, channel, releaseNotes string) error
}

// liveExecutor performs real operations.
type liveExecutor struct{}

//nolint:revive // sign flag is part of the ReleaseExecutor interface contract.
func (e *liveExecutor) CreateTag(_ context.Context, tag, ref, message string, sign bool) error {
	args := []string{"tag", "-a"}
	if sign {
		args = append(args, "-s")
	}
	args = append(args, tag, "-m", message, ref)
	return gitRun(args...)
}

func (*liveExecutor) PushTag(_ context.Context, tag string) error {
	return gitRun("push", "origin", tag)
}

func (*liveExecutor) TriggerWorkflow(_ context.Context, ref, channel, releaseNotes string) error {
	payload := map[string]string{
		"dry_run":         "false",
		"release_channel": channel,
		"release_notes":   releaseNotes,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return xerrors.Errorf("marshaling workflow payload: %w", err)
	}
	cmd := exec.Command("gh", "workflow", "run", "release.yaml",
		"--repo", owner+"/"+repo,
		"--ref", ref,
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

//nolint:revive // sign flag is part of the ReleaseExecutor interface contract.
func (e *dryRunExecutor) CreateTag(_ context.Context, tag, ref, message string, sign bool) error {
	signFlag := ""
	if sign {
		signFlag = "-s "
	}
	_, _ = fmt.Fprintf(e.w, "[DRYRUN] would run: git tag %s-a %s -m %q %s\n", signFlag, tag, message, ref)
	return nil
}

func (e *dryRunExecutor) PushTag(_ context.Context, tag string) error {
	_, _ = fmt.Fprintf(e.w, "[DRYRUN] would run: git push origin %s\n", tag)
	return nil
}

func (e *dryRunExecutor) TriggerWorkflow(_ context.Context, ref, channel, _ string) error {
	_, _ = fmt.Fprintf(e.w, "[DRYRUN] would trigger release.yaml workflow (ref=%s, channel=%s)\n", ref, channel)
	return nil
}
