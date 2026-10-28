//go:build linux || darwin

package terraform

import (
	"context"
	"fmt"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyStateIsAuthoritative(t *testing.T) {
	t.Parallel()
	const state = `{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"lineage","resources":[]}`
	for _, tc := range []struct {
		name, command, state       string
		cancel, kill, signalCancel bool
		want                       bool
	}{
		{name: "Success", state: state, want: true},
		{name: "PartialFailure", command: "exit 1", state: state, want: true},
		{name: "Crash", command: "exit 2", state: state},
		{name: "TerraformPanic", command: "exit 11", state: state},
		{name: "Killed", command: "kill -KILL $$", state: state},
		{name: "CanceledPartialFailure", command: "exit 1", state: state, cancel: true},
		{name: "CanceledSuccess", state: state, cancel: true},
		{name: "KillContext", state: state, kill: true},
		{name: "CancellationBeforeWatcher", command: "exit 1", state: state, signalCancel: true},
		{name: "InvalidState", command: "exit 1", state: "{}"},
		{name: "MalformedState", command: "exit 1", state: "checkpoint"},
		{name: "MissingState", command: "exit 1"},
		{name: "InvalidSuccessState", state: "{}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			killCtx, kill := context.WithCancel(t.Context())
			defer kill()
			canceled := make(chan struct{})
			var applyErr error
			if tc.command != "" {
				applyErr = exec.CommandContext(t.Context(), "/bin/sh", "-c", tc.command).Run()
				require.Error(t, applyErr)
				applyErr = fmt.Errorf("terraform apply: %w", applyErr)
			}
			if tc.cancel {
				cancel()
			}
			if tc.kill {
				kill()
			}
			if tc.signalCancel {
				close(canceled)
			}
			require.Equal(t, tc.want, applyStateIsAuthoritative(ctx, killCtx, canceled, []byte(tc.state), applyErr))
		})
	}
	t.Run("ExecutionFailure", func(t *testing.T) {
		t.Parallel()
		err := exec.CommandContext(t.Context(), "/does/not/exist").Run()
		require.Error(t, err)
		require.False(t, applyStateIsAuthoritative(t.Context(), t.Context(), nil, []byte(state), err))
	})
}
