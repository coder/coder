package jobreaper

import (
	"context"
	"database/sql"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
)

// reapTemplateResourceRun is called with the job locked. An acquired mutation
// may have changed real infrastructure, so its state must never be retried blindly.
func reapTemplateResourceRun(ctx context.Context, db database.Store, job database.ProvisionerJob) (sql.NullString, error) {
	run, err := db.GetTemplateResourceRunByJobID(ctx, job.ID)
	if err != nil {
		return sql.NullString{}, xerrors.Errorf("get shared resource run: %w", err)
	}
	state, err := db.GetTemplateResourceStateForUpdate(ctx, run.FoundationID)
	if err != nil {
		return sql.NullString{}, xerrors.Errorf("lock shared resource state: %w", err)
	}
	if !state.ActiveRunID.Valid || state.ActiveRunID.UUID != run.ID {
		return sql.NullString{}, xerrors.New("shared resource run reservation is stale")
	}
	code := "TEMPLATE_RESOURCE_STATE_UNCHANGED"
	if run.Action != "plan" && (job.StartedAt.Valid || job.WorkerID.Valid) {
		_, err := db.UpdateTemplateResourceState(ctx, database.UpdateTemplateResourceStateParams{
			FoundationID: run.FoundationID, RunID: state.ActiveRunID, Status: "unknown", State: state.State, UpdatedAt: dbtime.Now(),
		})
		return sql.NullString{String: "TEMPLATE_RESOURCE_STATE_UNKNOWN", Valid: true}, err
	}
	_, err = db.CompleteTemplateResourceRun(ctx, database.CompleteTemplateResourceRunParams{
		FoundationID: run.FoundationID, RunID: state.ActiveRunID, Status: state.Status,
		State: state.State, Outputs: state.Outputs, AppliedRevisionID: state.AppliedRevisionID,
		CheckpointRevisionID: state.CheckpointRevisionID, UpdatedAt: dbtime.Now(),
	})
	if err != nil {
		return sql.NullString{}, xerrors.Errorf("release shared resource run: %w", err)
	}
	return sql.NullString{String: code, Valid: true}, nil
}
