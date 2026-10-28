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
// The boolean reports a bound Foundation independently of published outputs.
func (b *Builder) templateResourceSnapshot() (uuid.NullUUID, json.RawMessage, bool, error) {
	empty := json.RawMessage(`{}`)
	if b.trans != database.WorkspaceTransitionStart {
		revision, outputs, err := b.stateTemplateResourceSnapshot()
		return revision, outputs, false, err
	}
	// Workspace create permission does not grant template update permission.
	// Read only the internal snapshot after normal build authorization.
	//nolint:gocritic
	ctx := dbauthz.AsSystemRestricted(b.ctx)
	if err := b.store.LockTemplateResourceTemplate(ctx, b.workspace.TemplateID); err != nil {
		return uuid.NullUUID{}, nil, false, BuildError{http.StatusInternalServerError, "lock shared resources", err}
	}
	foundation, err := b.store.GetFoundationByTemplateID(ctx, uuid.NullUUID{UUID: b.workspace.TemplateID, Valid: true})
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.NullUUID{}, empty, false, nil
	}
	if err != nil {
		return uuid.NullUUID{}, nil, false, BuildError{http.StatusInternalServerError, "get Foundation binding", err}
	}
	if err := b.store.LockFoundation(ctx, foundation.ID); err != nil {
		return uuid.NullUUID{}, nil, false, BuildError{http.StatusInternalServerError, "lock Foundation", err}
	}
	state, err := b.store.GetTemplateResourceStateForUpdate(ctx, foundation.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return uuid.NullUUID{}, nil, false, BuildError{http.StatusInternalServerError, "get shared resource snapshot", err}
	}
	// Pin published outputs, regardless of an active or failed mutation. An
	// unapplied Foundation has no outputs; Terraform decides whether the
	// workspace requires them or can use a fallback.
	if errors.Is(err, sql.ErrNoRows) || !state.AppliedRevisionID.Valid {
		return uuid.NullUUID{}, empty, true, nil
	}
	outputs := state.Outputs
	if len(outputs) == 0 {
		outputs = empty
	}
	return state.AppliedRevisionID, outputs, true, nil
}

// stateTemplateResourceSnapshot follows the retained state, not the latest
// attempted inputs. Failed planning preserves inherited state; partial apply
// replaces both the checkpoint and its associated snapshot.
func (b *Builder) stateTemplateResourceSnapshot() (uuid.NullUUID, json.RawMessage, error) {
	empty := json.RawMessage(`{}`)
	if b.state.orphan {
		return uuid.NullUUID{}, empty, nil
	}
	previous, err := b.getLastBuild()
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.NullUUID{}, empty, nil
	}
	if err != nil {
		return uuid.NullUUID{}, nil, BuildError{http.StatusInternalServerError, "get state-associated Foundation snapshot", err}
	}
	outputs := previous.StateTemplateResourceOutputs
	if len(outputs) == 0 {
		outputs = empty
	}
	return previous.StateTemplateResourceRevisionID, outputs, nil
}
