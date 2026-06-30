package chatgoal

import (
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

// ToSDK converts a database.ChatGoal to a codersdk.ChatGoal.
func ToSDK(goal database.ChatGoal) codersdk.ChatGoal {
	converted := codersdk.ChatGoal{
		ID:               goal.ID,
		RootChatID:       goal.RootChatID,
		Objective:        goal.Objective,
		Status:           codersdk.ChatGoalStatus(goal.Status),
		CreatedByUserID:  goal.CreatedByUserID,
		CompletedByAgent: goal.CompletedByAgent,
		CreatedAt:        goal.CreatedAt,
		UpdatedAt:        goal.UpdatedAt,
	}
	if goal.CreatedFromChatID.Valid {
		createdFromChatID := goal.CreatedFromChatID.UUID
		converted.CreatedFromChatID = &createdFromChatID
	}
	if goal.CompletionSummary.Valid {
		converted.CompletionSummary = &goal.CompletionSummary.String
	}
	if goal.CompletedByUserID.Valid {
		completedByUserID := goal.CompletedByUserID.UUID
		converted.CompletedByUserID = &completedByUserID
	}
	if goal.CompletedAt.Valid {
		completedAt := goal.CompletedAt.Time
		converted.CompletedAt = &completedAt
	}
	if goal.ClearedAt.Valid {
		clearedAt := goal.ClearedAt.Time
		converted.ClearedAt = &clearedAt
	}
	if goal.ReplacedAt.Valid {
		replacedAt := goal.ReplacedAt.Time
		converted.ReplacedAt = &replacedAt
	}
	return converted
}
