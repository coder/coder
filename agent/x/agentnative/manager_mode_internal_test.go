//go:build linux

package agentnative

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManagerCanonicalModeEvents(t *testing.T) {
	t.Parallel()
	for _, nativeEvents := range []bool{false, true} {
		t.Run(map[bool]string{false: "policy-only", true: "native-acknowledgement"}[nativeEvents], func(t *testing.T) {
			t.Parallel()
			b := &managerTestBackend{}
			if !nativeEvents {
				b.setMode = func(context.Context, Session, string) error { return nil }
			}
			m := newTestManager(t, filepath.Join(t.TempDir(), "state"), b)
			s := testManagerSession(t, m)
			require.NoError(t, m.SetMode(t.Context(), s.ID, "automation"))
			events, cursor, err := m.Events(t.Context(), s.ID, s.Cursor, false)
			require.NoError(t, err)
			require.Equal(t, []Event{{Seq: s.Cursor + 1, Type: "mode", Data: map[string]any{"mode": "automation"}}}, events)
			require.NoError(t, m.SetMode(t.Context(), s.ID, "automation"))
			events, _, err = m.Events(t.Context(), s.ID, cursor, false)
			require.NoError(t, err)
			require.Empty(t, events, "matching native acknowledgements do not duplicate a mode transition")
			require.NoError(t, m.Publish(s.ID, []Event{{Type: "needs.input", Data: map[string]any{"pending": true}}}, nil))
			events, _, err = m.Events(t.Context(), s.ID, cursor, false)
			require.NoError(t, err)
			require.Len(t, events, 2)
			require.Equal(t, "needs.input", events[0].Type)
			require.Equal(t, "mode", events[1].Type)
			require.Equal(t, "human", events[1].Data["mode"])
			current, err := m.Session(s.ID)
			require.NoError(t, err)
			require.Equal(t, "human", current.Mode)
		})
	}
}

func TestManagerArchivedEventWaitAndRestoreMode(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	m := newTestManager(t, dir, &managerTestBackend{})
	s := testManagerSession(t, m)
	require.NoError(t, m.Close())
	b := &managerTestBackend{setMode: func(context.Context, Session, string) error { return nil }}
	reopened := newTestManager(t, dir, b)
	s, err := reopened.Session(s.ID)
	require.NoError(t, err)
	require.True(t, s.Archived)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err = reopened.Events(ctx, s.ID, s.Cursor, true)
	require.ErrorIs(t, err, context.Canceled, "an archive at its current cursor waits instead of returning an empty batch")
	require.NoError(t, reopened.Restore(t.Context()))
	events, _, err := reopened.Events(t.Context(), s.ID, s.Cursor, false)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "mode", events[0].Type)
	require.Equal(t, "automation", events[0].Data["mode"])
	require.EqualValues(t, 1, b.resumes.Load())
}
