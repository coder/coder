//go:build !slim

package cli

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/testutil"
)

// blockingReloader blocks in Reload until the context is canceled, then
// returns its error. It models the standalone gateway's initial reload
// waiting on a daemon connection to an unreachable coderd.
type blockingReloader struct {
	started chan struct{}
}

func (r *blockingReloader) Reload(ctx context.Context) error {
	select {
	case r.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

// failThenSucceedReloader fails the first failUntil reloads, then succeeds,
// modeling a coderd connection or provider fetch that recovers after a few
// transient failures.
type failThenSucceedReloader struct {
	calls     atomic.Int32
	failUntil int32
}

func (r *failThenSucceedReloader) Reload(_ context.Context) error {
	if r.calls.Add(1) <= r.failUntil {
		return xerrors.New("transient failure")
	}
	return nil
}

// alwaysFailReloader returns the same error every time Reload is called.
type alwaysFailReloader struct {
	calls  atomic.Int32
	err    error
	after  func()
	called chan struct{}
}

func (r *alwaysFailReloader) Reload(context.Context) error {
	r.calls.Add(1)
	if r.after != nil {
		r.after()
	}
	select {
	case r.called <- struct{}{}:
	default:
	}
	return r.err
}

// TestLoadProviders_Interruptible verifies that a stop signal,
// modeled by canceling the context, unblocks the initial provider load even
// when the reloader is stuck waiting for coderd. This guards the standalone
// "ai-gateway start" command against the regression where startup could not
// be interrupted.
func TestLoadProviders_Interruptible(t *testing.T) {
	t.Parallel()

	// testCtx bounds the test and drives the channel receives; runCtx is the
	// context handed to loadProviders and is canceled to model a
	// stop signal. They are distinct so the receives still work after the
	// signal context is canceled.
	testCtx := testutil.Context(t, testutil.WaitShort)
	runCtx, cancel := context.WithCancel(testCtx)
	defer cancel()

	reloader := &blockingReloader{started: make(chan struct{}, 1)}
	logger := slog.Make()

	done := make(chan error, 1)
	go func() {
		done <- loadProviders(runCtx, reloader, logger, nil)
	}()

	// Wait for the reload to be in-flight, then cancel as a signal would.
	testutil.RequireReceive(testCtx, t, reloader.started)
	cancel()

	err := testutil.RequireReceive(testCtx, t, done)
	require.ErrorIs(t, err, context.Canceled)
}

// TestLoadProviders_RetrySucceeds verifies loadProviders keeps retrying past
// transient failures and returns nil once a reload succeeds. This guards the
// retry contract: replacing the loop's continue with a return would fail here.
func TestLoadProviders_RetrySucceeds(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	reloader := &failThenSucceedReloader{failUntil: 2}

	require.NoError(t, loadProviders(ctx, reloader, slog.Make(), nil))
	require.GreaterOrEqual(t, reloader.calls.Load(), int32(3))
}

func TestLoadProviders_AIBridgedDoneStopsRetry(t *testing.T) {
	t.Parallel()

	errMsg := "aibridged fatal"
	ctx := testutil.Context(t, testutil.WaitShort)
	aibridgedDone := make(chan struct{})
	reloader := &alwaysFailReloader{
		err:    xerrors.New(errMsg),
		called: make(chan struct{}, 1),
		after: func() {
			close(aibridgedDone)
		},
	}

	err := loadProviders(ctx, reloader, slog.Make(), aibridgedDone)
	require.ErrorContains(t, err, errMsg)
	require.Equal(t, int32(1), reloader.calls.Load())
}

func TestResolveAIGatewayKey(t *testing.T) {
	t.Parallel()

	keyFile := filepath.Join(t.TempDir(), "gateway.key")
	require.NoError(t, os.WriteFile(keyFile, []byte("file-key\n"), 0o600))

	tests := []struct {
		name    string
		key     string
		keyFile string
		want    string
		wantErr string
	}{
		{
			name:    "Nothing set",
			wantErr: keyFlagsMissingErr,
		},
		{
			name: "Key",
			key:  "flag-key",
			want: "flag-key",
		},
		{
			name:    "KeyFile",
			keyFile: keyFile,
			want:    "file-key",
		},
		{
			name:    "MutuallyExclusive",
			key:     "flag-key",
			keyFile: keyFile,
			wantErr: keyFlagsExclusiveErr,
		},
		{
			name:    "MissingKeyFile",
			keyFile: filepath.Join(t.TempDir(), "missing.key"),
			wantErr: "read AI Gateway key file",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveAIGatewayKey(tc.key, tc.keyFile)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestAIGatewayStart_DeploymentOptions(t *testing.T) {
	t.Parallel()

	cmd := (&RootCmd{}).aiGatewayStart()

	// Standalone Gateway only consumes deployment options used in LLM traffic.
	// Coderd-only settings such as provider seeds, retention,
	// structured logging, and Coder MCP injection must stay server-only.
	var got []string
	for _, opt := range cmd.Options {
		if opt.Group != nil && opt.Group.Name == "AI Gateway" {
			got = append(got, opt.Env)
		}
	}

	want := []string{
		"CODER_AI_GATEWAY_ALLOW_BYOK",
		"CODER_AI_GATEWAY_CIRCUIT_BREAKER_ENABLED",
		"CODER_AI_GATEWAY_CIRCUIT_BREAKER_FAILURE_THRESHOLD",
		"CODER_AI_GATEWAY_CIRCUIT_BREAKER_INTERVAL",
		"CODER_AI_GATEWAY_CIRCUIT_BREAKER_MAX_REQUESTS",
		"CODER_AI_GATEWAY_CIRCUIT_BREAKER_TIMEOUT",
		"CODER_AI_GATEWAY_DUMP_DIR",
		"CODER_AI_GATEWAY_MAX_CONCURRENCY",
		"CODER_AI_GATEWAY_RATE_LIMIT",
		"CODER_AI_GATEWAY_SEND_ACTOR_HEADERS",
	}
	require.ElementsMatch(t, want, got)
}
