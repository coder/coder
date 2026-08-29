package chattool

import (
	"context"
	"database/sql"
	"encoding/json"
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

// CurrentChatGoalByRootChatID returns the current goal for rootChatID, or
// sql.ErrNoRows when no current goal exists.
func CurrentChatGoalByRootChatID(ctx context.Context, db database.Store, rootChatID uuid.UUID) (database.ChatGoal, error) {
	goals, err := db.GetCurrentChatGoalsByRootChatIDs(ctx, []uuid.UUID{rootChatID})
	if err != nil {
		return database.ChatGoal{}, err
	}
	if len(goals) == 0 {
		return database.ChatGoal{}, sql.ErrNoRows
	}
	return goals[0], nil
}

// GetGoal returns a read-only tool for inspecting the current root goal.
func GetGoal(db database.Store, options GoalToolOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		GetGoalToolName,
		"Inspect the current durable goal for this root chat. Returns null when no current goal exists.",
		func(ctx context.Context, _ getGoalArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			goal, err := CurrentChatGoalByRootChatID(ctx, db, options.RootChatID)
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
				current, err := CurrentChatGoalByRootChatID(ctx, tx, options.RootChatID)
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
				case errors.Is(err, errGoalNotActive):
					if result, ok := agentCompletedGoalReplay(ctx, db, options.RootChatID, goalID); ok {
						return marshalToolResponse(result), nil
					}
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

var errGoalNotActive = xerrors.New("goal is not active")

// agentCompletedGoalReplay rebuilds the successful complete_goal result
// from the durable goal row when the current goal is the same goal the
// agent already completed. A crash, takeover, or interrupt between the
// goal transition and its tool-result commit replays or cancels the
// call; the replay must not report an error for work that durably
// succeeded.
func agentCompletedGoalReplay(ctx context.Context, db database.Store, rootChatID, goalID uuid.UUID) (completeGoalResult, bool) {
	current, err := CurrentChatGoalByRootChatID(ctx, db, rootChatID)
	if err != nil || current.ID != goalID ||
		current.Status != database.ChatGoalStatusComplete || !current.CompletedByAgent {
		return completeGoalResult{}, false
	}
	sdkGoal := chatgoal.ToSDK(current)
	return completeGoalResult{
		Goal:      &sdkGoal,
		Completed: true,
		Summary:   current.CompletionSummary.String,
	}, true
}

// AgentCompletedGoalReplayPayload returns the successful complete_goal
// result payload for a committed call whose result was never committed,
// when rawArgs names the current agent-completed goal. ok is false when
// the durable goal state does not prove the call succeeded.
func AgentCompletedGoalReplayPayload(ctx context.Context, db database.Store, rootChatID uuid.UUID, rawArgs string) (json.RawMessage, bool) {
	var args completeGoalArgs
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return nil, false
	}
	goalID, err := uuid.Parse(strings.TrimSpace(args.GoalID))
	if err != nil {
		return nil, false
	}
	result, ok := agentCompletedGoalReplay(ctx, db, rootChatID, goalID)
	if !ok {
		return nil, false
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return nil, false
	}
	return payload, true
}
