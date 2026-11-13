package chatstate

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

// lockChatProjectAccess locks the rows that can grant ownerID access to the
// project and then checks that ownerID can use it, inside the transaction
// that attaches a chat to the project. Every removal deletes or updates one
// of these rows before it moves chats out, so the two serialize: a removal
// that commits first fails this check, and one that commits later sees the
// attached chat. Membership rows are locked before the project row, matching
// the order of organization member deletion, and both before any chat row.
func lockChatProjectAccess(ctx context.Context, store database.Store, projectID, ownerID uuid.UUID) error {
	//nolint:gocritic // The check answers for the chat owner, not the caller.
	sysCtx := dbauthz.AsSystemRestricted(ctx)
	project, err := store.GetChatProjectByID(sysCtx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrChatProjectNotUsable
	}
	if err != nil {
		return xerrors.Errorf("get chat project: %w", err)
	}
	if project.OwnerID == ownerID {
		return nil
	}
	if _, err := store.LockOrganizationMembersByUserIDsForShare(sysCtx, database.LockOrganizationMembersByUserIDsForShareParams{
		OrganizationID: project.OrganizationID,
		UserIds:        []uuid.UUID{ownerID},
	}); err != nil {
		return xerrors.Errorf("lock organization membership: %w", err)
	}
	if _, err := store.LockGroupMembersByUserIDForShare(sysCtx, ownerID); err != nil {
		return xerrors.Errorf("lock group memberships: %w", err)
	}
	if _, err := store.GetChatProjectByIDForShare(sysCtx, projectID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrChatProjectNotUsable
		}
		return xerrors.Errorf("lock chat project: %w", err)
	}
	//nolint:gocritic // Matches chatd.ChatProjectUsableBy, which runs as chatd.
	usable, err := store.IsChatProjectUsableByUserID(dbauthz.AsChatd(ctx), database.IsChatProjectUsableByUserIDParams{
		ProjectID: projectID,
		UserID:    ownerID,
	})
	if err != nil {
		return xerrors.Errorf("check chat project access: %w", err)
	}
	if !usable {
		return ErrChatProjectNotUsable
	}
	return nil
}
