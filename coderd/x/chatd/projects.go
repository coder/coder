package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/codersdk"
)

// usableChatProject loads the project and reports whether the chat owner can
// use it. It reports false when the project is missing, the lookup or access
// check fails, or the owner cannot use it, and logs every case except a
// missing project.
func (p *Server) usableChatProject(ctx context.Context, logger slog.Logger, projectID uuid.UUID, chat database.Chat) (database.ChatProject, bool) {
	logger = logger.With(slog.F("project_id", projectID))
	project, err := p.db.GetChatProjectByID(ctx, projectID)
	if err != nil {
		// A project deleted mid-turn is expected.
		if !errors.Is(err, sql.ErrNoRows) {
			logger.Warn(ctx, "failed to load chat project, turn runs without project memory, instructions, or skills", slog.Error(err))
		}
		return database.ChatProject{}, false
	}
	usable, err := ChatProjectUsableBy(ctx, p.db, project, chat.OwnerID)
	if err != nil {
		logger.Warn(ctx, "failed to check chat project access, turn runs without project memory, instructions, or skills", slog.Error(err))
		return database.ChatProject{}, false
	}
	if !usable {
		logger.Debug(ctx, "chat owner cannot use chat project, turn runs without project memory, instructions, or skills",
			slog.F("project_owner_id", project.OwnerID))
		return database.ChatProject{}, false
	}
	return project, true
}

// ChatProjectUsableBy reports whether userID owns project or has an ACL
// grant on it. Role grants do not count, because the chat would read memory,
// instructions, and skills the project owner did not share. Memory tools and
// instruction and skill loading run as chatd, so they must use this check,
// not dbauthz.
func ChatProjectUsableBy(ctx context.Context, db database.Store, project database.ChatProject, userID uuid.UUID) (bool, error) {
	if project.OwnerID == userID {
		return true, nil
	}
	//nolint:gocritic // The check answers for the chat owner, not the caller.
	usable, err := db.IsChatProjectUsableByUserID(dbauthz.AsChatd(ctx), database.IsChatProjectUsableByUserIDParams{
		ProjectID: project.ID,
		UserID:    userID,
	})
	if err != nil {
		return false, xerrors.Errorf("check chat project access: %w", err)
	}
	return usable, nil
}

// RemoveChatsFromUnusableProjects moves chats out of projects their owners
// can no longer use. Call it in the transaction that removed the access, so
// the API never shows a chat in a project its owner was removed from, and
// pass the returned chat IDs to PublishChatsLeftProject after the commit.
func RemoveChatsFromUnusableProjects(ctx context.Context, tx database.Store, arg database.RemoveChatsFromUnusableChatProjectsParams) ([]uuid.UUID, error) {
	//nolint:gocritic // The caller authorized the access change; this is its side effect on other users' chats.
	ids, err := tx.RemoveChatsFromUnusableChatProjects(dbauthz.AsSystemRestricted(ctx), arg)
	if err != nil {
		return nil, xerrors.Errorf("remove chats from unusable chat projects: %w", err)
	}
	return ids, nil
}

// PublishChatsLeftProject tells the owners' watching clients that the chats
// left their project. Failures are logged, because the move is already
// committed and clients see it on their next fetch.
func PublishChatsLeftProject(ctx context.Context, logger slog.Logger, db database.Store, ps pubsub.Pubsub, chatIDs []uuid.UUID) {
	if len(chatIDs) == 0 {
		return
	}
	//nolint:gocritic // The chats belong to other users; only their watch events are built here.
	chats, err := db.GetChatsByIDs(dbauthz.AsSystemRestricted(ctx), chatIDs)
	if err != nil {
		logger.Error(ctx, "failed to load chats that left their project", slog.Error(err))
		return
	}
	for _, chat := range chats {
		payload, err := json.Marshal(codersdk.ChatWatchEvent{
			Kind: codersdk.ChatWatchEventKindProjectChange,
			Chat: chatWatchEventSDKChat(chat, nil),
		})
		if err != nil {
			logger.Error(ctx, "failed to marshal chat project change event", slog.F("chat_id", chat.ID), slog.Error(err))
			continue
		}
		if err := ps.Publish(coderdpubsub.ChatWatchEventChannel(chat.OwnerID), payload); err != nil {
			logger.Error(ctx, "failed to publish chat project change event", slog.F("chat_id", chat.ID), slog.Error(err))
		}
	}
}
