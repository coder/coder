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

// orgResolverStore is the subset of store queries needed by orgResolver.
type orgResolverStore interface {
	GetWorkspaceByID(ctx context.Context, id uuid.UUID) (database.Workspace, error)
	GetOrganizationsByUserID(ctx context.Context, arg database.GetOrganizationsByUserIDParams) ([]database.Organization, error)
}

// orgResolver determines the organization an AI Gateway request acts in.
// All org resolution for AI Gateway requests lives here.
type orgResolver struct {
	store orgResolverStore
}

// resolve returns the organization for a request made by userID. When the
// request was authenticated with a workspace session token, workspaceID
// identifies that workspace and its organization is returned. The workspace
// must still exist and userID must still be a member of its organization.
// The result is invalid when no org source applies to the request.
func (r orgResolver) resolve(ctx context.Context, userID uuid.UUID, workspaceID uuid.NullUUID) (uuid.NullUUID, error) {
	if !workspaceID.Valid {
		return uuid.NullUUID{}, nil
	}

	workspace, err := r.store.GetWorkspaceByID(ctx, workspaceID.UUID)
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

	orgs, err := r.store.GetOrganizationsByUserID(ctx, database.GetOrganizationsByUserIDParams{
		UserID:  userID,
		Deleted: sql.NullBool{Bool: false, Valid: true},
	})
	if err != nil {
		return uuid.NullUUID{}, xerrors.Errorf("get organizations for user %s: %w", userID, err)
	}
	if !slices.ContainsFunc(orgs, func(org database.Organization) bool {
		return org.ID == workspace.OrganizationID
	}) {
		return uuid.NullUUID{}, ErrNotOrganizationMember
	}

	return uuid.NullUUID{UUID: workspace.OrganizationID, Valid: true}, nil
}
