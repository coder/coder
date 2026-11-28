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
)

// LoadRefs reads the owners and chats that the automations name, one query
// each. Every owner is read: a caller who can read an automation sees who
// owns it. Only the chats that the actor in ctx can read are returned.
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
		//nolint:gocritic // Readers of an automation see its owner without user read permission.
		users, err := db.GetUsersByIDs(dbauthz.AsSystemRestricted(ctx), slices.Collect(maps.Keys(ownerIDs)))
		if err != nil {
			return refs, xerrors.Errorf("get owners: %w", err)
		}
		for _, u := range users {
			refs.Owners[u.ID] = u
		}
	}
	if len(chatIDs) > 0 {
		chats, err := db.GetReadableChatsByIDs(ctx, slices.Collect(maps.Keys(chatIDs)))
		if err != nil {
			return refs, xerrors.Errorf("get chats: %w", err)
		}
		for _, c := range chats {
			refs.Chats[c.ID] = c
		}
	}
	return refs, nil
}
