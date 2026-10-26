//go:build linux

package agentnative

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/quartz"
)

func TestManagerAttachWithFaultedObserver(t *testing.T) {
	t.Parallel()
	for _, harness := range []string{"claude", "codex", "opencode"} {
		for _, invalidNative := range []bool{false, true} {
			name := harness + "/verified"
			if invalidNative {
				name = harness + "/invalid-native"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				b := &managerTestBackend{}
				b.create = func(_ context.Context, cwd string) (Session, error) {
					id := uuid.NewString()
					nativeID := id
					if harness == "opencode" {
						nativeID = "ses_" + id
					}
					if harness != "claude" {
						id = harness + ":" + nativeID
					}
					return Session{ID: id, NativeID: nativeID, Harness: harness, Cwd: cwd, Mode: "human"}, nil
				}
				attachErr := xerrors.New("native attachment identity verification failed")
				b.attach = func(_ context.Context, s Session) (Command, error) {
					if invalidNative {
						return Command{}, attachErr
					}
					return Command{Path: harness, Args: []string{"attach", s.NativeID}, Dir: s.Cwd}, nil
				}
				m, err := New(Options{StateDir: filepath.Join(t.TempDir(), "state"), Clock: quartz.NewMock(t), Factories: map[string]func(BackendOptions) (Backend, error){
					harness: func(o BackendOptions) (Backend, error) { b.host = o.Host; return b, nil },
				}})
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, m.Close()) })
				s, err := m.Create(t.Context(), harness, t.TempDir())
				require.NoError(t, err)
				require.NoError(t, m.SetMode(t.Context(), s.ID, "automation"))
				require.NoError(t, m.Publish(s.ID, []Event{{Type: "error", Data: map[string]any{"message": "observation unavailable"}}}, nil))
				b.setMode = func(context.Context, Session, string) error { return xerrors.New("observer unavailable") }
				command, err := m.Attach(t.Context(), s.ID)
				switch {
				case invalidNative:
					require.ErrorIs(t, err, attachErr, "native verification failures must not be masked by observer errors")
				case harness == "claude":
					require.Error(t, err, "Claude mod human control must be acknowledged")
				default:
					require.NoError(t, err)
					require.Equal(t, harness, command.Path)
					require.Equal(t, []string{"attach", s.NativeID}, command.Args)
				}
				require.EqualValues(t, 1, b.attaches.Load())
				require.EqualValues(t, 1, b.creates.Load())
				require.EqualValues(t, 1, b.resumes.Load())
				require.Zero(t, b.prompts.Load())
				current, err := m.Session(s.ID)
				require.NoError(t, err)
				require.Equal(t, "human", current.Mode)
				require.NotEmpty(t, current.Fault)
			})
		}
	}
}
