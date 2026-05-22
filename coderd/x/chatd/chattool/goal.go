package chattool

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
)

const (
	GetGoalToolName      = "get_goal"
	CompleteGoalToolName = "complete_goal"
)

// GoalToolOptions configures the goal tools.
type GoalToolOptions struct {
	ChatID        uuid.UUID
	RootChatID    uuid.UUID
	IsRootChat    bool
	OnGoalUpdated func(context.Context, database.Chat, database.ChatGoal)
}

type getGoalArgs struct{}

type completeGoalArgs struct {
	GoalID  uuid.UUID `json:"goal_id" description:"The expected current goal ID. The tool fails if the current goal changed."`
	Summary string    `json:"summary" description:"A concise non-empty summary of how the goal was completed."`
}

// GetGoal returns a read-only tool for inspecting the current root goal.
func GetGoal(db database.Store, options GoalToolOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		GetGoalToolName,
		"Inspect the current durable goal for this root chat. Returns null when no active or paused goal exists.",
		func(ctx context.Context, _ getGoalArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			goal, err := db.GetCurrentChatGoalByRootChatID(ctx, options.RootChatID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return toolResponse(map[string]any{"goal": nil}), nil
				}
				return fantasy.NewTextErrorResponse("get goal: " + err.Error()), nil
			}
			sdkGoal := chatGoalToSDK(goal)
			return marshalToolResponse(struct {
				Goal any `json:"goal"`
			}{Goal: sdkGoal}), nil
		},
	)
}

// CompleteGoal returns a root-only tool that marks the active goal complete.
func CompleteGoal(db database.Store, options GoalToolOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		CompleteGoalToolName,
		"Mark the active chat goal complete after the objective is done. Requires the current goal_id and a concise completion summary. Only use this when the active goal has been satisfied.",
		func(ctx context.Context, args completeGoalArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if !options.IsRootChat {
				return fantasy.NewTextErrorResponse("complete_goal can only be used from the root chat"), nil
			}
			if args.GoalID == uuid.Nil {
				return fantasy.NewTextErrorResponse("goal_id is required"), nil
			}
			summary := strings.TrimSpace(args.Summary)
			if summary == "" {
				return fantasy.NewTextErrorResponse("summary is required"), nil
			}

			var completed database.ChatGoal
			var chat database.Chat
			if err := db.InTx(func(tx database.Store) error {
				current, err := tx.GetCurrentChatGoalByRootChatID(ctx, options.RootChatID)
				if err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						return sql.ErrNoRows
					}
					return err
				}
				if current.ID != args.GoalID {
					return sql.ErrNoRows
				}
				if current.Status != database.ChatGoalStatusActive {
					return errGoalNotActive
				}
				completed, err = tx.CompleteChatGoalByID(ctx, database.CompleteChatGoalByIDParams{
					RootChatID: options.RootChatID,
					ID:         args.GoalID,
					CompletionSummary: sql.NullString{
						String: summary,
						Valid:  true,
					},
					CompletedByUserID: uuid.NullUUID{},
					CompletedByAgent:  true,
				})
				if err != nil {
					return err
				}
				chat, err = tx.GetChatByID(ctx, options.ChatID)
				return err
			}, nil); err != nil {
				switch {
				case errors.Is(err, sql.ErrNoRows):
					return fantasy.NewTextErrorResponse("current active goal does not match goal_id"), nil
				case errors.Is(err, errGoalNotActive):
					return fantasy.NewTextErrorResponse("current goal is not active"), nil
				default:
					return fantasy.NewTextErrorResponse("complete goal: " + err.Error()), nil
				}
			}

			if options.OnGoalUpdated != nil {
				options.OnGoalUpdated(ctx, chat, completed)
			}
			sdkGoal := chatGoalToSDK(completed)
			return marshalToolResponse(struct {
				Goal      any    `json:"goal"`
				Completed bool   `json:"completed"`
				Summary   string `json:"summary"`
			}{Goal: sdkGoal, Completed: true, Summary: summary}), nil
		},
	)
}

func chatGoalToSDK(goal database.ChatGoal) codersdk.ChatGoal {
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

var errGoalNotActive = xerrors.New("goal is not active")
