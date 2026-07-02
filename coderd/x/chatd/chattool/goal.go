package chattool

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk/chatgoal"
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
	// Fence, when set, must still describe the chat when complete_goal
	// commits. It prevents a stale generation (interrupted or taken over
	// by another worker) from completing the durable goal after its tool
	// result would be rejected by the generation fence.
	Fence *GoalToolFence
}

// GoalToolFence pins the goal mutation to the generation turn that
// offered the tool.
type GoalToolFence struct {
	WorkerID       uuid.UUID
	RunnerID       uuid.UUID
	HistoryVersion int64
}

var errGoalFenceMismatch = xerrors.New("goal tool fence mismatch")

// verifyGoalToolFence locks the chat row and checks that the turn that
// offered complete_goal still owns the chat.
func verifyGoalToolFence(ctx context.Context, tx database.Store, chatID uuid.UUID, fence *GoalToolFence) error {
	if fence == nil {
		return nil
	}
	chat, err := tx.GetChatByIDForUpdate(ctx, chatID)
	if err != nil {
		return err
	}
	if !chat.WorkerID.Valid || chat.WorkerID.UUID != fence.WorkerID ||
		!chat.RunnerID.Valid || chat.RunnerID.UUID != fence.RunnerID ||
		chat.Status != database.ChatStatusRunning ||
		chat.HistoryVersion != fence.HistoryVersion {
		return errGoalFenceMismatch
	}
	return nil
}

type getGoalArgs struct{}

type completeGoalArgs struct {
	GoalID  string `json:"goal_id" description:"The expected current goal ID as a UUIDv4 string. The tool fails if the current goal changed."`
	Summary string `json:"summary" description:"A concise non-empty summary of how the goal was completed."`
}

type goalResult struct {
	Goal *codersdk.ChatGoal `json:"goal"`
}

type completeGoalResult struct {
	Goal      *codersdk.ChatGoal `json:"goal"`
	Completed bool               `json:"completed"`
	Summary   string             `json:"summary"`
}

// GetGoal returns a read-only tool for inspecting the current root goal.
func GetGoal(db database.Store, options GoalToolOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		GetGoalToolName,
		"Inspect the current durable goal for this root chat. Returns null when no current goal exists.",
		func(ctx context.Context, _ getGoalArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			goal, err := db.GetCurrentChatGoalByRootChatID(ctx, options.RootChatID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return marshalToolResponse(goalResult{}), nil
				}
				return fantasy.NewTextErrorResponse("get goal: " + err.Error()), nil
			}
			sdkGoal := chatgoal.ToSDK(goal)
			return marshalToolResponse(goalResult{Goal: &sdkGoal}), nil
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
			goalIDStr := strings.TrimSpace(args.GoalID)
			if goalIDStr == "" {
				return fantasy.NewTextErrorResponse("goal_id is required"), nil
			}
			goalID, err := uuid.Parse(goalIDStr)
			if err != nil {
				return fantasy.NewTextErrorResponse("goal_id is required"), nil
			}
			summary := strings.TrimSpace(args.Summary)
			if summary == "" {
				return fantasy.NewTextErrorResponse("summary is required"), nil
			}
			if len(summary) > codersdk.MaxChatGoalCompletionSummaryBytes {
				return fantasy.NewTextErrorResponse(fmt.Sprintf(
					"summary must be at most %d bytes",
					codersdk.MaxChatGoalCompletionSummaryBytes,
				)), nil
			}

			var completed database.ChatGoal
			var chat database.Chat
			if err := db.InTx(func(tx database.Store) error {
				// Lock the chat row first (matching the API mutation
				// paths) so the fence check and goal update are atomic
				// with respect to interrupts and worker takeovers.
				if err := verifyGoalToolFence(ctx, tx, options.ChatID, options.Fence); err != nil {
					return err
				}
				current, err := tx.GetCurrentChatGoalByRootChatID(ctx, options.RootChatID)
				if err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						return sql.ErrNoRows
					}
					return err
				}
				if current.ID != goalID {
					return sql.ErrNoRows
				}
				if current.Status != database.ChatGoalStatusActive {
					return errGoalNotActive
				}
				if len(current.Objective)+len(summary) > codersdk.MaxChatGoalTextPayloadBytes {
					return errGoalTextPayloadTooLong
				}
				completed, err = tx.CompleteChatGoalByID(ctx, database.CompleteChatGoalByIDParams{
					RootChatID: options.RootChatID,
					ID:         goalID,
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
				case errors.Is(err, errGoalTextPayloadTooLong):
					return fantasy.NewTextErrorResponse(fmt.Sprintf(
						"goal objective and summary must be at most %d bytes combined",
						codersdk.MaxChatGoalTextPayloadBytes,
					)), nil
				case errors.Is(err, errGoalNotActive):
					return fantasy.NewTextErrorResponse("current goal is not active"), nil
				case errors.Is(err, errGoalFenceMismatch):
					return fantasy.NewTextErrorResponse("the chat turn changed before the goal could be completed; the goal was not modified"), nil
				default:
					return fantasy.NewTextErrorResponse("complete goal: " + err.Error()), nil
				}
			}

			if options.OnGoalUpdated != nil {
				options.OnGoalUpdated(ctx, chat, completed)
			}
			sdkGoal := chatgoal.ToSDK(completed)
			return marshalToolResponse(completeGoalResult{
				Goal:      &sdkGoal,
				Completed: true,
				Summary:   summary,
			}), nil
		},
	)
}

var (
	errGoalNotActive          = xerrors.New("goal is not active")
	errGoalTextPayloadTooLong = xerrors.New("goal text payload too long")
)
