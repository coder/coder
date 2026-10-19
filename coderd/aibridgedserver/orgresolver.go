package aibridgedserver

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// resolveOrganization determines the organization an AI Gateway request acts
// in. All org resolution for AI Gateway requests lives here.
//
// It returns the organization for a request made by userID. When the
// request was authenticated with a workspace session token, workspaceID
// identifies that workspace and its organization is returned. The workspace
// must still exist and userID must still be a member of its organization.
// The result is invalid when no org source applies to the request.
func (s *Server) resolveOrganization(ctx context.Context, userID uuid.UUID, workspaceID uuid.NullUUID) (uuid.NullUUID, error) {
	if !workspaceID.Valid {
		return uuid.NullUUID{}, nil
	}

	workspace, err := s.store.GetWorkspaceByID(ctx, workspaceID.UUID)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.NullUUID{}, ErrUnknownWorkspace
	}
	if err != nil {
		return uuid.NullUUID{}, xerrors.Errorf("get workspace %s: %w", workspaceID.UUID, err)
	}
	// Workspaces are soft-deleted, so a deleted row still resolves by ID.
	if workspace.Deleted {
		return uuid.NullUUID{}, ErrUnknownWorkspace
	}

	orgs, err := s.store.GetOrganizationsByUserID(ctx, database.GetOrganizationsByUserIDParams{
		UserID:  userID,
		Deleted: sql.NullBool{Bool: false, Valid: true},
	})
	if err != nil {
		return uuid.NullUUID{}, xerrors.Errorf("get organizations for user %s: %w", userID, err)
	}
	if !slices.ContainsFunc(orgs, func(org database.Organization) bool {
		return org.ID == workspace.OrganizationID
	}) {
		return uuid.NullUUID{}, ErrUnknownOrganizationMember
	}

	return uuid.NullUUID{UUID: workspace.OrganizationID, Valid: true}, nil
}
