package workspaceexec_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/workspaceexec"
	"github.com/coder/coder/v2/testutil"
)

func TestControllerConcurrentRetentionAndOwnership(t *testing.T) {
	t.Parallel()
	for _, mutation := range []string{"retain", "transfer"} {
		t.Run(mutation, func(t *testing.T) {
			t.Parallel()
			f := newControllerFixture(t)
			path := filepath.Join(t.TempDir(), "result")
			require.NoError(t, os.WriteFile(path, []byte("result"), 0o600))
			session := f.session(t, []string{path}, nil)
			collecting, release := make(chan struct{}), make(chan struct{})
			f.agent.beforeBundle = func(ctx context.Context) error {
				close(collecting)
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			f.controller.Start(t.Context())
			done := make(chan struct{})
			go func() { defer close(done); f.tick(t) }()
			select {
			case <-collecting:
			case <-time.After(testutil.WaitLong):
				t.Fatal("preservation did not start")
			}
			current := f.current(t, session.ID)
			require.Equal(t, "preserving", current.State)
			require.ErrorIs(t, f.db.InTx(func(tx database.Store) error { return workspaceexec.CheckAdmission(t.Context(), tx, f.workspace.ID) }, nil), workspaceexec.ErrAdmissionClosed)
			_, err := workspaceexec.Renew(t.Context(), f.db, session.ID, current.Revision, f.clock.Now().Add(time.Hour), f.clock.Now())
			require.ErrorIs(t, err, workspaceexec.ErrAdmissionClosed)
			if mutation == "retain" {
				retained, err := workspaceexec.Retain(t.Context(), f.db, session.ID, current.Revision, f.clock.Now())
				require.NoError(t, err)
				require.Greater(t, retained.Revision, current.Revision)
			} else {
				owner := dbgen.User(t, f.db, database.User{})
				_, err := f.sqlDB.ExecContext(t.Context(), "UPDATE workspaces SET owner_id=$1 WHERE id=$2", owner.ID, f.workspace.ID)
				require.NoError(t, err)
			}
			close(release)
			select {
			case <-done:
			case <-time.After(testutil.WaitLong):
				t.Fatal("controller did not settle")
			}
			current = f.current(t, session.ID)
			require.False(t, current.DeleteBuildID.Valid)
			latest, err := f.db.GetLatestWorkspaceBuildByWorkspaceID(t.Context(), f.workspace.ID)
			require.NoError(t, err)
			require.Equal(t, int32(1), latest.BuildNumber)
		})
	}
}
