package aibridgedserver

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/aibridge/budget"
	"github.com/coder/coder/v2/coderd/database"
)

// ErrAmbiguousOrganizationInput is returned when a request carries more than
// one organization input. The login paths are mutually exclusive.
var ErrAmbiguousOrganizationInput = xerrors.New("workspace and chat organization inputs are mutually exclusive")

// orgResolverInput holds one input per login path. At most one is set. Every
// input is validated: resolveOrganization only returns an organization the
// user is a live member of.
type orgResolverInput struct {
	// WorkspaceID is set on the direct path, for a workspace session token.
	WorkspaceID uuid.NullUUID
	// ChatOrganizationID is set on the delegated path, for a chatd request.
	// chatd sets it in-process from the chat, so clients cannot supply it.
	ChatOrganizationID uuid.NullUUID
}

// resolvedOrganization is the organization a request acts in.
type resolvedOrganization struct {
	ID     uuid.UUID
	Source database.AIBridgeInterceptionOrganizationSource
}

// resolveOrganization determines the organization an AI Gateway request acts
// in. All org resolution for AI Gateway requests lives here. It runs once per
// request, in IsAuthorized. The budget check, the interception record and
// cost reuse the result.
//
// The inputs are tried in order:
//  1. A workspace session token resolves to the workspace's organization. The
//     workspace must still exist and userID must still be a member of its
//     organization.
//  2. A chatd request resolves to the chat's organization. userID must still
//     be a member of it.
//  3. Otherwise, the organization of the user's site-wide effective group.
//
// Every AI Gateway request is scoped to an organization, so it returns
// [ErrNoOrganization] when none resolves, for example when the user belongs to
// no live organization.
func (s *Server) resolveOrganization(ctx context.Context, userID uuid.UUID, in orgResolverInput) (resolvedOrganization, error) {
	if in.WorkspaceID.Valid && in.ChatOrganizationID.Valid {
		return resolvedOrganization{}, ErrAmbiguousOrganizationInput
	}

	if in.WorkspaceID.Valid {
		orgID, err := s.workspaceOrganization(ctx, userID, in.WorkspaceID.UUID)
		if err != nil {
			return resolvedOrganization{}, err
		}
		return resolvedOrganization{ID: orgID, Source: database.AibridgeInterceptionOrganizationSourceWorkspace}, nil
	}

	if in.ChatOrganizationID.Valid {
		orgID := in.ChatOrganizationID.UUID
		if err := s.requireOrganizationMember(ctx, userID, orgID); err != nil {
			return resolvedOrganization{}, err
		}
		return resolvedOrganization{ID: orgID, Source: database.AibridgeInterceptionOrganizationSourceChatd}, nil
	}

	group, ok, err := budget.ResolveUserEffectiveGroup(ctx, s.store, userID, uuid.NullUUID{}, s.budgetPolicy)
	if err != nil {
		return resolvedOrganization{}, xerrors.Errorf("resolve effective AI group for user %s: %w", userID, err)
	}
	if !ok {
		return resolvedOrganization{}, ErrNoOrganization
	}
	return resolvedOrganization{ID: group.OrganizationID, Source: database.AibridgeInterceptionOrganizationSourceEffectiveGroup}, nil
}

// workspaceOrganization returns the organization of workspaceID. It rejects the
// request when the workspace is missing or deleted, or when userID is not a
// member of its organization.
func (s *Server) workspaceOrganization(ctx context.Context, userID, workspaceID uuid.UUID) (uuid.UUID, error) {
	workspace, err := s.store.GetWorkspaceByID(ctx, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, ErrUnknownWorkspace
	}
	if err != nil {
		return uuid.Nil, xerrors.Errorf("get workspace %s: %w", workspaceID, err)
	}
	// Workspaces are soft-deleted, so a deleted row still resolves by ID.
	if workspace.Deleted {
		return uuid.Nil, ErrUnknownWorkspace
	}

	if err := s.requireOrganizationMember(ctx, userID, workspace.OrganizationID); err != nil {
		return uuid.Nil, err
	}
	return workspace.OrganizationID, nil
}

// requireOrganizationMember returns ErrUnknownOrganizationMember unless userID
// is a member of orgID and orgID is not deleted.
func (s *Server) requireOrganizationMember(ctx context.Context, userID, orgID uuid.UUID) error {
	orgs, err := s.store.GetOrganizationsByUserID(ctx, database.GetOrganizationsByUserIDParams{
		UserID:  userID,
		Deleted: sql.NullBool{Bool: false, Valid: true},
	})
	if err != nil {
		return xerrors.Errorf("get organizations for user %s: %w", userID, err)
	}
	if !slices.ContainsFunc(orgs, func(org database.Organization) bool {
		return org.ID == orgID
	}) {
		return ErrUnknownOrganizationMember
	}
	return nil
}
