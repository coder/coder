package agentsocket_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/testutil"
)

// fakeContextManager is an in-memory agentsocket.ContextManager for tests.
type fakeContextManager struct {
	sources   []agentcontext.Source
	snapshot  agentcontext.Snapshot
	resyncErr error
	resynced  bool
}

func (f *fakeContextManager) Sources() []agentcontext.Source { return f.sources }

func (f *fakeContextManager) HasSource(path string) (string, bool) {
	for _, s := range f.sources {
		if s.Path == path {
			return s.Path, true
		}
	}
	return "", false
}

func (f *fakeContextManager) AddSource(s agentcontext.Source) (agentcontext.Source, error) {
	for _, existing := range f.sources {
		if existing.Path == s.Path {
			return existing, nil
		}
	}
	f.sources = append(f.sources, s)
	return s, nil
}

func (f *fakeContextManager) RemoveSource(path string) error {
	for i, s := range f.sources {
		if s.Path == path {
			f.sources = append(f.sources[:i], f.sources[i+1:]...)
			return nil
		}
	}
	return agentcontext.ErrSourceNotFound
}

func (f *fakeContextManager) Snapshot() agentcontext.Snapshot { return f.snapshot }

func (f *fakeContextManager) Resync(_ context.Context) (agentcontext.Snapshot, error) {
	if f.resyncErr != nil {
		return agentcontext.Snapshot{}, f.resyncErr
	}
	f.resynced = true
	return f.snapshot, nil
}

func TestDRPCAgentSocketService_Context(t *testing.T) {
	t.Parallel()

	t.Run("SourceCRUDAndSnapshot", func(t *testing.T) {
		t.Parallel()

		const sourcePath = "/home/coder/project"
		cm := &fakeContextManager{
			snapshot: agentcontext.Snapshot{
				Version: 7,
				Resources: []agentcontext.Resource{{
					ID:          "instruction_file:" + sourcePath + "/AGENTS.md",
					Kind:        agentcontext.KindInstructionFile,
					Source:      sourcePath + "/AGENTS.md",
					SourcePath:  sourcePath,
					SizeBytes:   42,
					Status:      agentcontext.StatusOK,
					Description: "be concise",
				}, {
					// A built-in resource (no source path) the show filter must skip.
					ID:     "instruction_file:/home/coder/.coder/AGENTS.md",
					Kind:   agentcontext.KindInstructionFile,
					Source: "/home/coder/.coder/AGENTS.md",
					Status: agentcontext.StatusOK,
				}},
			},
		}

		socketPath := testutil.AgentSocketPath(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		server, err := agentsocket.NewServer(
			slog.Make().Leveled(slog.LevelDebug),
			agentsocket.WithPath(socketPath),
			agentsocket.WithContextManager(cm),
		)
		require.NoError(t, err)
		defer server.Close()

		client := newSocketClient(ctx, t, socketPath)

		// Add a source.
		src, err := client.AddContextSource(ctx, sourcePath)
		require.NoError(t, err)
		require.Equal(t, sourcePath, src.Path)

		// It shows up in the list.
		sources, err := client.ContextSources(ctx)
		require.NoError(t, err)
		require.Len(t, sources, 1)
		require.Equal(t, sourcePath, sources[0].Path)

		// Get the registered source.
		got, err := client.GetContextSource(ctx, sourcePath)
		require.NoError(t, err)
		require.Equal(t, sourcePath, got.Path)

		// Getting an unregistered source errors.
		_, err = client.GetContextSource(ctx, "/nope")
		require.Error(t, err)

		// Snapshot carries resources with their source path stamped.
		snap, err := client.GetContextSnapshot(ctx)
		require.NoError(t, err)
		require.EqualValues(t, 7, snap.Version)
		require.Len(t, snap.Resources, 2)
		require.Equal(t, agentcontext.KindInstructionFile.String(), snap.Resources[0].Kind)
		require.Equal(t, sourcePath, snap.Resources[0].SourcePath)
		require.EqualValues(t, 42, snap.Resources[0].SizeBytes)

		// Remove the source; removing again reports not found.
		require.NoError(t, client.RemoveContextSource(ctx, sourcePath))
		err = client.RemoveContextSource(ctx, sourcePath)
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
	})

	t.Run("Resync", func(t *testing.T) {
		t.Parallel()

		cm := &fakeContextManager{snapshot: agentcontext.Snapshot{Version: 3}}
		socketPath := testutil.AgentSocketPath(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		server, err := agentsocket.NewServer(
			slog.Make().Leveled(slog.LevelDebug),
			agentsocket.WithPath(socketPath),
			agentsocket.WithContextManager(cm),
		)
		require.NoError(t, err)
		defer server.Close()

		client := newSocketClient(ctx, t, socketPath)

		snap, err := client.ResyncContext(ctx)
		require.NoError(t, err)
		require.EqualValues(t, 3, snap.Version)
		require.True(t, cm.resynced)
	})

	t.Run("NoManagerErrors", func(t *testing.T) {
		t.Parallel()

		socketPath := testutil.AgentSocketPath(t)
		ctx := testutil.Context(t, testutil.WaitShort)
		// No WithContextManager: the context RPCs must fail cleanly.
		server, err := agentsocket.NewServer(
			slog.Make().Leveled(slog.LevelDebug),
			agentsocket.WithPath(socketPath),
		)
		require.NoError(t, err)
		defer server.Close()

		client := newSocketClient(ctx, t, socketPath)

		// Every context RPC independently guards a nil context manager;
		// exercise all of them so dropping a guard surfaces as a test
		// failure rather than an agent-killing nil dereference in a DRPC
		// handler.
		_, err = client.ContextSources(ctx)
		require.Error(t, err)
		_, err = client.GetContextSource(ctx, "/tmp/x")
		require.Error(t, err)
		_, err = client.AddContextSource(ctx, "/tmp/x")
		require.Error(t, err)
		err = client.RemoveContextSource(ctx, "/tmp/x")
		require.Error(t, err)
		_, err = client.GetContextSnapshot(ctx)
		require.Error(t, err)
		_, err = client.ResyncContext(ctx)
		require.Error(t, err)
	})
}
