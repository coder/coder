package dbauthz

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// Submission receipts belong to their initiating actor, even for shared chats.
// Knowing a chat ID must not permit reading another actor's retry identities.
func (q *querier) InsertChatSubmission(ctx context.Context, arg database.InsertChatSubmissionParams) (database.ChatSubmission, error) {
	if err := q.authorizeContext(ctx, policy.ActionCreate, rbac.ResourceChat.WithOwner(arg.ActorID.String()).InOrg(arg.OrganizationID)); err != nil {
		return database.ChatSubmission{}, err
	}
	return q.db.InsertChatSubmission(ctx, arg)
}

func (q *querier) GetChatSubmission(ctx context.Context, arg database.GetChatSubmissionParams) (database.ChatSubmission, error) {
	if err := q.authorizeContext(ctx, policy.ActionRead, rbac.ResourceChat.WithOwner(arg.ActorID.String()).InOrg(arg.OrganizationID)); err != nil {
		return database.ChatSubmission{}, err
	}
	return q.db.GetChatSubmission(ctx, arg)
}

func (q *querier) CompleteChatSubmission(ctx context.Context, arg database.CompleteChatSubmissionParams) (database.ChatSubmission, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceChat.WithOwner(arg.ActorID.String()).InOrg(arg.OrganizationID)); err != nil {
		return database.ChatSubmission{}, err
	}
	return q.db.CompleteChatSubmission(ctx, arg)
}

func (q *querier) FinishChatSubmission(ctx context.Context, arg database.FinishChatSubmissionParams) (database.ChatSubmission, error) {
	if err := q.authorizeContext(ctx, policy.ActionUpdate, rbac.ResourceChat.WithOwner(arg.ActorID.String()).InOrg(arg.OrganizationID)); err != nil {
		return database.ChatSubmission{}, err
	}
	return q.db.FinishChatSubmission(ctx, arg)
}

func (q *querier) GetLatestChatSubmissionSettings(ctx context.Context, chatID uuid.UUID) (json.RawMessage, error) {
	if _, err := q.GetChatByID(ctx, chatID); err != nil {
		return nil, err
	}
	return q.db.GetLatestChatSubmissionSettings(ctx, chatID)
}
