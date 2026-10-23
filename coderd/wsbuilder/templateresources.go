package wsbuilder

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

// templateResourceSnapshot holds the non-sensitive values used by this build.
// Cleanup must keep the previous snapshot even if template mutations failed.
func (b *Builder) templateResourceSnapshot() (uuid.NullUUID, json.RawMessage, error) {
	empty := json.RawMessage(`{}`)
	if b.trans != database.WorkspaceTransitionStart {
		previous, err := b.getLastBuild()
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.NullUUID{}, empty, nil
		}
		if err != nil {
			return uuid.NullUUID{}, nil, BuildError{http.StatusInternalServerError, "get previous template resource snapshot", err}
		}
		outputs := previous.TemplateResourceOutputs
		if len(outputs) == 0 {
			outputs = empty
		}
		return previous.TemplateResourceRevisionID, outputs, nil
	}
	// Workspace create permission does not grant template update permission.
	// Read only the internal snapshot after normal build authorization.
	//nolint:gocritic
	ctx := dbauthz.AsSystemRestricted(b.ctx)
	if err := b.store.LockTemplateResourceTemplate(ctx, b.workspace.TemplateID); err != nil {
		return uuid.NullUUID{}, nil, BuildError{http.StatusInternalServerError, "lock template resources", err}
	}
	state, err := b.store.GetTemplateResourceStateForUpdate(ctx, b.workspace.TemplateID)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.NullUUID{}, empty, nil
	}
	if err != nil {
		return uuid.NullUUID{}, nil, BuildError{http.StatusInternalServerError, "get template resource snapshot", err}
	}
	if !state.CurrentRevisionID.Valid && !state.AppliedRevisionID.Valid && !state.ActiveRunID.Valid && state.Status == "unconfigured" {
		return uuid.NullUUID{}, empty, nil
	}
	if state.Status != "applied" || !state.AppliedRevisionID.Valid || state.ActiveRunID.Valid {
		return uuid.NullUUID{}, nil, BuildError{http.StatusConflict, "Workspace startup requires a successful template resource apply with no active run.", nil}
	}
	return state.AppliedRevisionID, state.Outputs, nil
}
