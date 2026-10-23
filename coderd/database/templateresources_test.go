package database_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/testutil"
)

func TestTemplateResourceInitialConfigurationSerialization(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
	observedAbsent, configured := make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	var attempts atomic.Int32
	go func() {
		result <- database.ReadModifyUpdate(db, func(tx database.Store) error {
			attempt := attempts.Add(1)
			state, err := tx.GetTemplateResourceState(t.Context(), template.ID)
			switch {
			case attempt == 1:
				if !errors.Is(err, sql.ErrNoRows) {
					return err
				}
				close(observedAbsent)
				select {
				case <-configured:
				case <-ctx.Done():
					return ctx.Err()
				}
			case err != nil:
				return err
			case state.TemplateID != template.ID:
				return sql.ErrNoRows
			}
			// This must force a transaction retry rather than silently allowing a
			// build using a snapshot from before the first resource configuration.
			return tx.LockTemplateResourceTemplate(t.Context(), template.ID)
		})
	}()
	testutil.TryReceive(ctx, t, observedAbsent)
	require.NoError(t, db.InTx(func(tx database.Store) error {
		if err := tx.LockTemplateResourceTemplate(t.Context(), template.ID); err != nil {
			return err
		}
		_, err := tx.InsertTemplateResourceState(t.Context(), database.InsertTemplateResourceStateParams{TemplateID: template.ID, OrganizationID: org.ID, UpdatedAt: dbtime.Now()})
		return err
	}, nil))
	close(configured)
	require.NoError(t, testutil.TryReceive(ctx, t, result))
	require.GreaterOrEqual(t, attempts.Load(), int32(2))
}

func TestTemplateResourcePersistence(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, _ := dbtestutil.NewDB(t)
	org := dbgen.Organization(t, db, database.Organization{})
	foreignOrg := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{OrganizationID: org.ID, CreatedBy: user.ID})
	other := dbgen.Template(t, db, database.Template{OrganizationID: foreignOrg.ID, CreatedBy: user.ID})
	file := dbgen.File(t, db, database.File{CreatedBy: user.ID})
	now := dbtime.Now()
	_, err := db.InsertTemplateResourceState(ctx, database.InsertTemplateResourceStateParams{TemplateID: template.ID, OrganizationID: foreignOrg.ID, UpdatedAt: now})
	require.Error(t, err, "template state must share the template organization")
	state, err := db.InsertTemplateResourceState(ctx, database.InsertTemplateResourceStateParams{TemplateID: template.ID, OrganizationID: org.ID, UpdatedAt: now})
	require.NoError(t, err)
	require.Equal(t, "unconfigured", state.Status)
	require.JSONEq(t, `{}`, string(state.Outputs))
	rev, err := db.InsertTemplateResourceRevision(ctx, database.InsertTemplateResourceRevisionParams{ID: uuid.New(), TemplateID: template.ID, OrganizationID: org.ID, FileID: file.ID, Source: "# immutable root", CreatedBy: user.ID, CreatedAt: now})
	require.NoError(t, err)
	foreign, err := db.InsertTemplateResourceRevision(ctx, database.InsertTemplateResourceRevisionParams{ID: uuid.New(), TemplateID: other.ID, OrganizationID: foreignOrg.ID, FileID: file.ID, Source: "# foreign root", CreatedBy: user.ID, CreatedAt: now})
	require.NoError(t, err)
	_, err = db.SetTemplateResourceCurrentRevision(ctx, database.SetTemplateResourceCurrentRevisionParams{TemplateID: template.ID, CurrentRevisionID: uuid.NullUUID{UUID: foreign.ID, Valid: true}, UpdatedAt: now})
	require.Error(t, err, "foreign revisions cannot be attached to a template")
	_, err = db.SetTemplateResourceCurrentRevision(ctx, database.SetTemplateResourceCurrentRevisionParams{TemplateID: template.ID, CurrentRevisionID: uuid.NullUUID{UUID: rev.ID, Valid: true}, UpdatedAt: now})
	require.NoError(t, err)
	job := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{OrganizationID: org.ID, InitiatorID: user.ID, FileID: file.ID, Type: database.ProvisionerJobTypeTemplateResources})
	args := database.InsertTemplateResourceRunParams{ID: uuid.New(), TemplateID: template.ID, OrganizationID: org.ID, RevisionID: foreign.ID, JobID: job.ID, Action: "apply", CreatedAt: now}
	_, err = db.InsertTemplateResourceRun(ctx, args)
	require.Error(t, err, "run revision must belong to the same template and organization")
	args.RevisionID = rev.ID
	run, err := db.InsertTemplateResourceRun(ctx, args)
	require.NoError(t, err)
	_, err = db.StartTemplateResourceRun(ctx, database.StartTemplateResourceRunParams{TemplateID: template.ID, UpdatedAt: now})
	require.ErrorIs(t, err, sql.ErrNoRows, "null reservation cannot bypass fencing")
	state, err = db.StartTemplateResourceRun(ctx, database.StartTemplateResourceRunParams{TemplateID: template.ID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, UpdatedAt: now})
	require.NoError(t, err)
	_, err = db.UpdateTemplateResourceState(ctx, database.UpdateTemplateResourceStateParams{TemplateID: template.ID, RunID: uuid.NullUUID{UUID: uuid.New(), Valid: true}, State: []byte("stale"), Status: "unknown", UpdatedAt: now})
	require.ErrorIs(t, err, sql.ErrNoRows)
	snapshot := json.RawMessage(`{"nested":{"list":[1,true]}}`)
	state, err = db.CompleteTemplateResourceRun(ctx, database.CompleteTemplateResourceRunParams{TemplateID: template.ID, RunID: state.ActiveRunID, State: []byte("private state"), Status: "applied", Outputs: snapshot, AppliedRevisionID: uuid.NullUUID{UUID: rev.ID, Valid: true}, UpdatedAt: now})
	require.NoError(t, err)
	require.False(t, state.ActiveRunID.Valid)
	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{TemplateID: template.ID, OrganizationID: org.ID, OwnerID: user.ID})
	version := dbgen.TemplateVersion(t, db, database.TemplateVersion{TemplateID: uuid.NullUUID{UUID: template.ID, Valid: true}, OrganizationID: org.ID, JobID: job.ID, CreatedBy: user.ID})
	build := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{WorkspaceID: workspace.ID, TemplateVersionID: version.ID, JobID: job.ID, InitiatorID: user.ID})
	require.NoError(t, db.PinWorkspaceBuildTemplateResources(ctx, database.PinWorkspaceBuildTemplateResourcesParams{ID: build.ID, TemplateResourceRevisionID: state.AppliedRevisionID, TemplateResourceOutputs: snapshot}))
	pinned, err := db.GetWorkspaceBuildByID(ctx, build.ID)
	require.NoError(t, err)
	require.Equal(t, rev.ID, pinned.TemplateResourceRevisionID.UUID)
	require.JSONEq(t, string(snapshot), string(pinned.TemplateResourceOutputs))
	// Both completion and interim persistence reject a released reservation.
	_, err = db.CompleteTemplateResourceRun(ctx, database.CompleteTemplateResourceRunParams{TemplateID: template.ID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, State: []byte("stale"), Status: "failed", Outputs: json.RawMessage(`{}`), UpdatedAt: now})
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = db.UpdateTemplateResourceState(ctx, database.UpdateTemplateResourceStateParams{TemplateID: template.ID, RunID: uuid.NullUUID{UUID: run.ID, Valid: true}, State: []byte("stale"), Status: "unknown", UpdatedAt: now})
	require.ErrorIs(t, err, sql.ErrNoRows)
	persisted, err := db.GetTemplateResourceState(ctx, template.ID)
	require.NoError(t, err)
	require.Equal(t, state, persisted)
}
