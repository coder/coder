package aiagentidentity

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/entity"
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
// Resolution is check-then-create, so it takes the workspace's row lock and
// holds it to the end. Two concurrent resolutions would otherwise both find
// nothing and both create, leaving the workspace with two live agents. A unique
// index over agents used to make the second insert fail; capacity is a fact
// about the container rather than about agents, so that index is gone and the
// race is dealt with where it happens.
func ResolveWorkspaceOrigin(ctx context.Context, db database.Store, workspace database.Workspace) (database.AIAgentLedger, error) {
	var resolved database.AIAgentLedger
	err := db.InTx(func(tx database.Store) error {
		if _, err := tx.LockWorkspaceByID(ctx, workspace.ID); err != nil {
			return xerrors.Errorf("lock workspace to resolve its AI agent: %w", err)
		}

		agent, err := tx.GetLiveAIAgentByCreationSite(ctx, database.GetLiveAIAgentByCreationSiteParams{
			CreationSiteType: string(entity.CreationSiteTypeWorkspace),
			CreationSiteID:   workspace.ID,
		})
		switch {
		case err == nil:
			if agent.OwnerID != workspace.OwnerID {
				if err := revokeWorkspaceOrigin(ctx, tx, agent, workspace.OwnerID); err != nil {
					return xerrors.Errorf("revoke AI agent identity after ownership change: %w", err)
				}
				resolved, err = createWorkspaceOrigin(ctx, tx, workspace)
				return err
			}
			resolved = agent
			return nil
		case errors.Is(err, sql.ErrNoRows):
			resolved, err = createWorkspaceOrigin(ctx, tx, workspace)
			return err
		default:
			return xerrors.Errorf("get the workspace's live AI agent: %w", err)
		}
	}, nil)
	if err != nil {
		return database.AIAgentLedger{}, err
	}
	return resolved, nil
}

func createWorkspaceOrigin(ctx context.Context, db database.Store, workspace database.Workspace) (database.AIAgentLedger, error) {
	_, agent, err := Create(ctx, db, CreateParams{
		OwnerID:        workspace.OwnerID,
		OrganizationID: workspace.OrganizationID,
		OriginType:     database.AIAgentOriginWorkspace,
		OriginID:       workspace.ID,
	})
	if err != nil {
		return database.AIAgentLedger{}, xerrors.Errorf("create workspace AI agent identity: %w", err)
	}
	//nolint:gocritic // Reading back what Create just wrote requires system access.
	return db.GetAIAgentLedgerRowByID(dbauthz.AsSystemRestricted(ctx), agent.UserID)
}

// revokeWorkspaceOrigin retires the agent, drops its workspace-pinned key and
// marks the mirror deleted. Revocation is a soft delete so audit history keeps
// resolving the agent.
//
// **The event is `kill` and that is a proof of concept cheat.** Nobody ordered
// this agent's death. The workspace's owner changed at some earlier moment, and
// this is the next resolution noticing, which makes it observed rather than
// commanded. Modeling it properly needs a transition the machine does not have
// and entities that are not modeled yet. Eric, 2026-08-23: use `kill` for now
// and record it. `killer` is the workspace's current owner, being the party
// whose acquisition of it ended the old sponsorship.
func revokeWorkspaceOrigin(ctx context.Context, db database.Store, agent database.AIAgentLedger, killer uuid.UUID) error {
	profile := WorkspaceAgentIdentityProfile(agent.CreationSiteID)
	//nolint:gocritic // Managing internal AI agent identities requires system access.
	systemCtx := dbauthz.AsSystemRestricted(ctx)
	key, err := db.GetAPIKeyByName(systemCtx, database.GetAPIKeyByNameParams{
		HolderID:  database.HolderID(agent.ID),
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
	if err := entity.RetireAIAgent(systemCtx, db, agent.ID, entity.EventAIAgentKill,
		entity.Ref{Type: entity.TypeUser, ID: killer}, dbtime.Now()); err != nil {
		return xerrors.Errorf("retire AI agent: %w", err)
	}
	rewrite2026augustlog.AIAgentRevoked(ctx, rewrite2026augustlog.F{
		"ai_agent_user_id": agent.ID,
		"owner_user_id":    agent.OwnerID,
		"origin_type":      agent.CreationSiteType,
		"origin_id":        agent.CreationSiteID,
		"route":            "workspace origin",
	})
	return nil
}
