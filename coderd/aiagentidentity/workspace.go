package aiagentidentity

import (
	"context"
	"database/sql"
	"errors"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rewrite2026augustlog"
)

// ResolveWorkspaceOrigin creates or reuses the workspace-origin identity for
// a workspace, the human-to-AI boundary crossed by a direct opt-in or a
// human-declared sandbox. Reuse keeps one unbroken lineage across rebuilds.
//
// An identity is only valid while it is sponsored by the workspace's current
// owner: after an ownership transfer the old sponsorship no longer bounds the
// agent's permissions, so the stale identity is revoked and a fresh one is
// created under the new owner.
func ResolveWorkspaceOrigin(ctx context.Context, db database.Store, workspace database.Workspace) (database.AIAgent, error) {
	agent, err := db.GetAIAgentByOrigin(ctx, database.GetAIAgentByOriginParams{
		OriginType: database.AIAgentOriginWorkspace,
		OriginID:   workspace.ID,
	})
	switch {
	case err == nil:
		if agent.OwnerUserID != workspace.OwnerID {
			if err := revokeWorkspaceOrigin(ctx, db, agent); err != nil {
				return database.AIAgent{}, xerrors.Errorf("revoke AI agent identity after ownership change: %w", err)
			}
			return createWorkspaceOrigin(ctx, db, workspace)
		}
		return agent, nil
	case errors.Is(err, sql.ErrNoRows):
		return createWorkspaceOrigin(ctx, db, workspace)
	default:
		return database.AIAgent{}, xerrors.Errorf("get AI agent by origin: %w", err)
	}
}

func createWorkspaceOrigin(ctx context.Context, db database.Store, workspace database.Workspace) (database.AIAgent, error) {
	_, agent, err := Create(ctx, db, CreateParams{
		OwnerID:        workspace.OwnerID,
		OrganizationID: workspace.OrganizationID,
		OriginType:     database.AIAgentOriginWorkspace,
		OriginID:       workspace.ID,
	})
	if err != nil {
		return database.AIAgent{}, xerrors.Errorf("create workspace AI agent identity: %w", err)
	}
	return agent, nil
}

// revokeWorkspaceOrigin drops the identity's workspace-pinned key and marks
// the identity deleted. Revocation is a soft delete so audit history keeps
// resolving the agent.
func revokeWorkspaceOrigin(ctx context.Context, db database.Store, agent database.AIAgent) error {
	profile := WorkspaceAgentIdentityProfile(agent.OriginID)
	//nolint:gocritic // Managing internal AI agent identities requires system access.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	key, err := db.GetAPIKeyByName(systemCtx, database.GetAPIKeyByNameParams{
		UserID:    agent.UserID,
		TokenName: profile.TokenName,
	})
	switch {
	case err == nil:
		if err := db.DeleteAPIKeyByID(systemCtx, key.ID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return xerrors.Errorf("delete AI agent API key: %w", err)
		}
	case errors.Is(err, sql.ErrNoRows):
	default:
		return xerrors.Errorf("get AI agent API key by name: %w", err)
	}
	if _, err := db.UpdateAIAgentDeleted(systemCtx, database.UpdateAIAgentDeletedParams{
		UserID:  agent.UserID,
		Deleted: true,
	}); err != nil {
		return xerrors.Errorf("mark AI agent deleted: %w", err)
	}
	rewrite2026augustlog.AIAgentRevoked(ctx, rewrite2026augustlog.F{
		"ai_agent_user_id": agent.UserID,
		"owner_user_id":    agent.OwnerUserID,
		"origin_type":      agent.OriginType,
		"origin_id":        agent.OriginID,
		"route":            "workspace origin",
	})
	return nil
}
