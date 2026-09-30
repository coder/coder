package workspaceexec

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/workspaceartifacts"
)

// otherSessionProtection runs only while holding the workspace lifecycle and
// row locks. Preservation can proceed before siblings' outputs are exported,
// but no deletion may proceed until every required generation is available.
func otherSessionProtection(ctx context.Context, tx database.Store, current database.WorkspaceExecutionSession, now time.Time) ([]database.WorkspaceExecutionSession, string, error) {
	others, err := tx.GetOtherWorkspaceExecutionSessionsByWorkspaceID(ctx, database.GetOtherWorkspaceExecutionSessionsByWorkspaceIDParams{WorkspaceID: current.WorkspaceID, ID: current.ID})
	if err != nil {
		return nil, "", err
	}
	for _, other := range others {
		if other.Retained {
			return others, "another workspace session is retained", nil
		}
		if other.State == "completed" {
			continue
		}
		if other.LeaseExpiresAt.After(now) {
			return others, "another workspace session has an active lease", nil
		}
		if other.State == "preserving" || other.State == "deleting" {
			return others, "another workspace session is changing lifecycle state", nil
		}
	}
	return others, "", nil
}

func requiredResultsAvailable(ctx context.Context, tx database.Store, session database.WorkspaceExecutionSession, now time.Time) error {
	var declaration struct {
		ResultPaths []string `json:"result_paths"`
	}
	if err := json.Unmarshal(session.Declarations, &declaration); err != nil {
		return err
	}
	if len(declaration.ResultPaths) == 0 {
		return nil
	}
	if session.State != "preserved" && session.State != "completed" {
		return xerrors.New("another session's required results have not been preserved")
	}
	artifacts, err := tx.GetWorkspaceExecutionArtifactsBySessionID(ctx, session.ID)
	if err != nil {
		return err
	}
	count := 0
	// Preserved records the successful commit of one complete generation.
	// Expiry remains detectable rather than shrinking the required set.
	for _, artifact := range artifacts {
		if artifact.PreservationRevision != session.Revision {
			continue
		}
		count++
		if artifact.ExpiresAt.Valid && !artifact.ExpiresAt.Time.After(now) {
			return xerrors.New("required preserved results are unavailable")
		}
		stored, err := tx.ReadWorkspaceExecutionArtifact(ctx, database.ReadWorkspaceExecutionArtifactParams{ID: artifact.ID, ByteLimit: workspaceartifacts.MaxBundleBytes})
		if err != nil {
			return err
		}
		digest := sha256.Sum256(stored.Data)
		if int64(len(stored.Data)) != artifact.SizeBytes || !bytes.Equal(digest[:], artifact.Sha256) {
			return xerrors.New("required preserved results failed integrity verification")
		}
	}
	if count == 0 {
		return xerrors.New("required preserved result generation is missing")
	}
	return nil
}

func (c *Controller) commitPreservedSession(ctx context.Context, expected database.WorkspaceExecutionSession) (database.WorkspaceExecutionSession, error) {
	var result database.WorkspaceExecutionSession
	err := c.Database.InTx(func(tx database.Store) error {
		current, workspace, err := lockSession(ctx, tx, expected)
		if err != nil {
			return err
		}
		if workspace.Deleted || current.State != "preserving" || current.Revision != expected.Revision {
			return ErrSessionChanged
		}
		next := current
		next.State = "preserved"
		next.Error = ""
		next.NextRetryAt = sql.NullTime{}
		result, err = saveSession(ctx, tx, current, next, c.Clock.Now())
		return err
	}, nil)
	return result, err
}
