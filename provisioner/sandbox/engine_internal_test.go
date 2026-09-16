package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
	protobuf "google.golang.org/protobuf/proto"

	"github.com/coder/coder/v2/provisionersdk/proto"
)

type fakeRuntime struct {
	mu          sync.Mutex
	allocations map[string]RuntimeSpec
	starts      int
	onStart     func(context.Context) error
	deleteErr   error
}

func newFakeRuntime() *fakeRuntime { return &fakeRuntime{allocations: make(map[string]RuntimeSpec)} }
func (f *fakeRuntime) Start(ctx context.Context, spec RuntimeSpec) error {
	f.mu.Lock()
	if _, exists := f.allocations[spec.ID]; !exists {
		f.starts++
	}
	f.allocations[spec.ID] = spec
	f.mu.Unlock()
	if f.onStart != nil {
		return f.onStart(ctx)
	}
	return nil
}

func (f *fakeRuntime) Inspect(_ context.Context, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.allocations[id]
	return ok, nil
}

func (f *fakeRuntime) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.allocations, id)
	return nil
}

func (f *fakeRuntime) List(_ context.Context, workspace string) ([]RuntimeAllocation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []RuntimeAllocation
	for _, s := range f.allocations {
		if s.WorkspaceID == workspace {
			out = append(out, RuntimeAllocation{ID: s.ID, WorkspaceID: s.WorkspaceID, BuildID: s.BuildID, BuildNumber: s.BuildNumber, Image: s.Image})
		}
	}
	return out, nil
}
func (*fakeRuntime) Close() error { return nil }

func testManifest(t *testing.T) Manifest {
	t.Helper()
	m, err := ParseManifest([]byte("version: 1\nimage: example.com/agent@sha256:" + strings.Repeat("a", 64) + "\n"))
	require.NoError(t, err)
	return m
}

func testMetadata() *proto.Metadata {
	return &proto.Metadata{WorkspaceId: uuid.NewString(), WorkspaceBuildId: uuid.NewString(), WorkspaceBuildNumber: 1, CoderUrl: "http://127.0.0.1:3000", WorkspaceTransition: proto.WorkspaceTransition_START}
}

func TestEngineLifecycleAndRecovery(t *testing.T) {
	t.Parallel()
	rt, dir := newFakeRuntime(), t.TempDir()
	e, err := NewEngine(rt, dir)
	require.NoError(t, err)
	m, meta := testManifest(t), testMetadata()
	first, err := e.Apply(t.Context(), m, meta)
	require.NoError(t, err)
	require.Equal(t, "running", first.Phase)
	require.NotEmpty(t, first.AgentToken)
	stat, err := os.Stat(e.operationPath(first.WorkspaceID, first.BuildID))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), stat.Mode().Perm())

	// A new worker reuses the journal, including the original credentials.
	recovered, err := NewEngine(rt, dir)
	require.NoError(t, err)
	again, err := recovered.Apply(t.Context(), m, meta)
	require.NoError(t, err)
	require.Equal(t, first, again)
	require.Equal(t, 1, rt.starts)
	// Simulate a crash after runtime creation, before recording success.
	first.Phase = "intent"
	require.NoError(t, e.save(first))
	again, err = recovered.Apply(t.Context(), m, meta)
	require.NoError(t, err)
	require.Equal(t, first.AgentToken, again.AgentToken)
	require.Equal(t, 1, rt.starts)

	stop := protobuf.CloneOf(meta)
	stop.WorkspaceBuildId = uuid.NewString()
	stop.WorkspaceBuildNumber = 2
	stop.WorkspaceTransition = proto.WorkspaceTransition_STOP
	stopped, err := recovered.Apply(t.Context(), m, stop)
	require.NoError(t, err)
	require.Equal(t, "stopped", stopped.Phase)
	require.Empty(t, rt.allocations)
	_, err = recovered.Apply(t.Context(), m, meta)
	require.ErrorContains(t, err, "superseded")
	_, err = recovered.Apply(t.Context(), m, stop)
	require.NoError(t, err)

	next := protobuf.CloneOf(meta)
	next.WorkspaceBuildId = uuid.NewString()
	next.WorkspaceBuildNumber = 3
	restarted, err := recovered.Apply(t.Context(), m, next)
	require.NoError(t, err)
	require.NotEqual(t, first.AgentToken, restarted.AgentToken)
	require.NotEqual(t, first.RuntimeID, restarted.RuntimeID)
	// A delayed old stop replay must leave the new runtime alive.
	_, err = recovered.Apply(t.Context(), m, stop)
	require.ErrorContains(t, err, "superseded")
	running, err := rt.Inspect(t.Context(), restarted.RuntimeID)
	require.NoError(t, err)
	require.True(t, running)
	remove := protobuf.CloneOf(next)
	remove.WorkspaceBuildId = uuid.NewString()
	remove.WorkspaceBuildNumber = 4
	remove.WorkspaceTransition = proto.WorkspaceTransition_DESTROY
	_, err = recovered.Apply(t.Context(), m, remove)
	require.NoError(t, err)
	_, err = recovered.Apply(t.Context(), m, remove)
	require.NoError(t, err)
	require.Empty(t, rt.allocations)
}

func TestEngineCancellationCleanupAndRetry(t *testing.T) {
	t.Parallel()
	rt := newFakeRuntime()
	started := make(chan struct{})
	rt.onStart = func(ctx context.Context) error { close(started); <-ctx.Done(); return ctx.Err() }
	e, err := NewEngine(rt, t.TempDir())
	require.NoError(t, err)
	m, meta := testManifest(t), testMetadata()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := e.Apply(ctx, m, meta); done <- err }()
	<-started
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	require.Empty(t, rt.allocations)
	_, err = e.Apply(t.Context(), m, meta)
	require.ErrorContains(t, err, "retired")
}

func TestEngineRetainsFailedCleanup(t *testing.T) {
	t.Parallel()
	rt := newFakeRuntime()
	rt.onStart = func(context.Context) error { return xerrors.New("runtime start failed") }
	rt.deleteErr = xerrors.New("runtime unavailable")
	e, err := NewEngine(rt, t.TempDir())
	require.NoError(t, err)
	m, meta := testManifest(t), testMetadata()
	s, err := e.Apply(t.Context(), m, meta)
	require.ErrorContains(t, err, "record retained")
	require.Equal(t, "cleanup", s.Phase)
	rt.deleteErr = nil
	_, err = e.Apply(t.Context(), m, meta)
	require.ErrorContains(t, err, "create a new build")
	require.Empty(t, rt.allocations)
}

func TestEngineOwnershipFailurePreservesAllocation(t *testing.T) {
	t.Parallel()
	rt := newFakeRuntime()
	rt.onStart = func(context.Context) error {
		return xerrors.Errorf("existing container: %w", ErrRuntimeOwnership)
	}
	e, err := NewEngine(rt, t.TempDir())
	require.NoError(t, err)
	_, err = e.Apply(t.Context(), testManifest(t), testMetadata())
	require.ErrorIs(t, err, ErrRuntimeOwnership)
	require.Len(t, rt.allocations, 1, "an ownership conflict must never authorize runtime deletion")
}

func TestEngineMissingStateAndWorkspaceIsolation(t *testing.T) {
	t.Parallel()
	rt, dir := newFakeRuntime(), t.TempDir()
	e, err := NewEngine(rt, dir)
	require.NoError(t, err)
	m, first, second := testManifest(t), testMetadata(), testMetadata()
	a, err := e.Apply(t.Context(), m, first)
	require.NoError(t, err)
	b, err := e.Apply(t.Context(), m, second)
	require.NoError(t, err)
	// Lost control-plane state does not prevent an authorized delete from
	// discovering this workspace's allocations on the sole configured host.
	require.NoError(t, os.RemoveAll(filepath.Join(dir, "operations", first.WorkspaceId)))
	first.WorkspaceBuildId = uuid.NewString()
	first.WorkspaceBuildNumber++
	first.WorkspaceTransition = proto.WorkspaceTransition_DESTROY
	_, err = e.Apply(t.Context(), m, first)
	require.NoError(t, err)
	running, err := rt.Inspect(t.Context(), a.RuntimeID)
	require.NoError(t, err)
	require.False(t, running)
	running, err = rt.Inspect(t.Context(), b.RuntimeID)
	require.NoError(t, err)
	require.True(t, running)
}

func TestEngineRejectsInvalidIdentity(t *testing.T) {
	t.Parallel()
	e, err := NewEngine(newFakeRuntime(), t.TempDir())
	require.NoError(t, err)
	meta := testMetadata()
	meta.WorkspaceId = "../another-workspace"
	_, err = e.Apply(t.Context(), testManifest(t), meta)
	require.ErrorContains(t, err, "canonical")
	_, err = DecodeState([]byte(`{"version":1,"workspace_id":"../bad"}`), "")
	require.Error(t, err)
}

func TestEngineFencesPreviouslyUnseenOldBuild(t *testing.T) {
	t.Parallel()
	for _, lostJournal := range []bool{false, true} {
		t.Run(fmt.Sprint(lostJournal), func(t *testing.T) {
			t.Parallel()
			rt, dir := newFakeRuntime(), t.TempDir()
			e, err := NewEngine(rt, dir)
			require.NoError(t, err)
			m, newest := testManifest(t), testMetadata()
			newest.WorkspaceBuildNumber = 3
			current, err := e.Apply(t.Context(), m, newest)
			require.NoError(t, err)
			if lostJournal {
				require.NoError(t, os.RemoveAll(filepath.Join(dir, "operations", newest.WorkspaceId)))
				_, err = e.Apply(t.Context(), m, newest)
				require.ErrorContains(t, err, "no matching operation journal")
			}
			for _, transition := range []proto.WorkspaceTransition{proto.WorkspaceTransition_START, proto.WorkspaceTransition_STOP, proto.WorkspaceTransition_DESTROY} {
				old := protobuf.CloneOf(newest)
				old.WorkspaceBuildId = uuid.NewString()
				old.WorkspaceBuildNumber = 2
				old.WorkspaceTransition = transition
				_, err = e.Apply(t.Context(), m, old)
				require.ErrorContains(t, err, "superseded")
			}
			require.Len(t, rt.allocations, 1)
			require.Equal(t, current.AgentToken, rt.allocations[current.RuntimeID].AgentToken)
			require.Equal(t, 1, rt.starts)
		})
	}
}
