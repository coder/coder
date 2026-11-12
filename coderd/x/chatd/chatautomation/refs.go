package chatautomation

import (
	"context"
	"maps"
	"slices"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
)

// LoadRefs reads what a chat automation response adds to the rows: their
// owners and the chats that they name. It reads the owners and the chats in
// one query each, and reads the owners one by one only when the caller can
// read some of them and not others. The result holds only what the caller can
// read, so a response never shows an owner or a chat title that the caller
// cannot see.
func LoadRefs(ctx context.Context, db database.Store, automations []database.ChatAutomation) (db2sdk.ChatAutomationRefs, error) {
	refs := db2sdk.ChatAutomationRefs{
		Owners: make(map[uuid.UUID]database.User),
		Chats:  make(map[uuid.UUID]database.Chat),
	}
	ownerIDs := make(map[uuid.UUID]struct{})
	chatIDs := make(map[uuid.UUID]struct{})
	for _, a := range automations {
		ownerIDs[a.OwnerID] = struct{}{}
		for _, id := range []uuid.NullUUID{a.TargetChatID, a.CreatedByChatID} {
			if id.Valid {
				chatIDs[id.UUID] = struct{}{}
			}
		}
	}
	if len(ownerIDs) > 0 {
		users, err := db.GetUsersByIDs(ctx, slices.Collect(maps.Keys(ownerIDs)))
		switch {
		case dbauthz.IsNotAuthorizedError(err):
			// The batch read is all or nothing. Read the owners one by one and
			// leave out each one that the caller cannot read.
			for id := range ownerIDs {
				user, err := db.GetUserByID(ctx, id)
				if httpapi.Is404Error(err) {
					continue
				}
				if err != nil {
					return refs, xerrors.Errorf("get owner: %w", err)
				}
				refs.Owners[id] = user
			}
		case err != nil:
			return refs, xerrors.Errorf("get owners: %w", err)
		default:
			for _, u := range users {
				refs.Owners[u.ID] = u
			}
		}
	}
	if len(chatIDs) > 0 {
		chats, err := db.GetChatsByIDs(ctx, slices.Collect(maps.Keys(chatIDs)))
		if err != nil {
			return refs, xerrors.Errorf("get chats: %w", err)
		}
		for _, c := range chats {
			refs.Chats[c.ID] = c
		}
	}
	return refs, nil
}
