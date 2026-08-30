package chattool_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestGoalTools(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "goal-tools",
	})
	goal, err := db.InsertActiveChatGoal(dbauthz.AsSystemRestricted(ctx), database.InsertActiveChatGoalParams{
		RootChatID:      chat.ID,
		Objective:       "finish the work",
		CreatedByUserID: user.ID,
	})
	require.NoError(t, err)

	getTool := chattool.GetGoal(db, chattool.GoalToolOptions{
		ChatID:     chat.ID,
		RootChatID: chat.ID,
		IsRootChat: true,
	})
	getResp, err := getTool.Run(dbauthz.AsSystemRestricted(ctx), fantasy.ToolCall{ID: "call-1", Name: chattool.GetGoalToolName, Input: "{}"})
	require.NoError(t, err)
	require.False(t, getResp.IsError)
	require.Contains(t, getResp.Content, goal.ID.String())

	var published bool
	completeTool := chattool.CompleteGoal(db, chattool.GoalToolOptions{
		ChatID:     chat.ID,
		RootChatID: chat.ID,
		IsRootChat: true,
		OnGoalUpdated: func(_ context.Context, updated database.Chat, completed database.ChatGoal) {
			published = true
			require.Equal(t, chat.ID, updated.ID)
			require.Equal(t, goal.ID, completed.ID)
		},
	})
	completeResp, err := completeTool.Run(dbauthz.AsSystemRestricted(ctx), fantasy.ToolCall{
		ID:    "call-2",
		Name:  chattool.CompleteGoalToolName,
		Input: `{"goal_id":"` + goal.ID.String() + `","summary":"done"}`,
	})
	require.NoError(t, err)
	require.False(t, completeResp.IsError)
	require.True(t, published)
	var payload struct {
		Completed bool `json:"completed"`
	}
	require.NoError(t, json.Unmarshal([]byte(completeResp.Content), &payload))
	require.True(t, payload.Completed)

	completedGoal, err := chattool.CurrentChatGoalByRootChatID(dbauthz.AsSystemRestricted(ctx), db, chat.ID)
	require.NoError(t, err)
	require.Equal(t, goal.ID, completedGoal.ID)
	require.Equal(t, database.ChatGoalStatusComplete, completedGoal.Status)
}

func TestCompleteGoalRejectsFenceMismatch(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "goal-fence",
	})
	goal, err := db.InsertActiveChatGoal(dbauthz.AsSystemRestricted(ctx), database.InsertActiveChatGoalParams{
		RootChatID:      chat.ID,
		Objective:       "finish the work",
		CreatedByUserID: user.ID,
	})
	require.NoError(t, err)

	// The chat is not owned by the fenced worker/runner, so the tool
	// must refuse to mutate the goal.
	completeTool := chattool.CompleteGoal(db, chattool.GoalToolOptions{
		ChatID:     chat.ID,
		RootChatID: chat.ID,
		IsRootChat: true,
		Fence: &chattool.GoalToolFence{
			WorkerID:       uuid.New(),
			RunnerID:       uuid.New(),
			HistoryVersion: chat.HistoryVersion,
		},
		OnGoalUpdated: func(context.Context, database.Chat, database.ChatGoal) {
			t.Fatal("goal update published despite fence mismatch")
		},
	})
	resp, err := completeTool.Run(dbauthz.AsSystemRestricted(ctx), fantasy.ToolCall{
		ID:    "call-1",
		Name:  chattool.CompleteGoalToolName,
		Input: `{"goal_id":"` + goal.ID.String() + `","summary":"done"}`,
	})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "the goal was not modified")

	current, err := chattool.CurrentChatGoalByRootChatID(dbauthz.AsSystemRestricted(ctx), db, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatGoalStatusActive, current.Status)
}

func TestCompleteGoalSchemaUsesStringGoalID(t *testing.T) {
	t.Parallel()

	tool := chattool.CompleteGoal(nil, chattool.GoalToolOptions{})
	info := tool.Info()
	goalIDParam, ok := info.Parameters["goal_id"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "string", goalIDParam["type"])
	require.Contains(t, goalIDParam["description"], "UUIDv4 string")
}

func TestGetGoalReturnsNullWithoutCurrentGoal(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "goal-tools-empty",
	})

	getTool := chattool.GetGoal(db, chattool.GoalToolOptions{
		ChatID:     chat.ID,
		RootChatID: chat.ID,
		IsRootChat: true,
	})
	getResp, err := getTool.Run(dbauthz.AsSystemRestricted(ctx), fantasy.ToolCall{ID: "call-1", Name: chattool.GetGoalToolName, Input: "{}"})
	require.NoError(t, err)
	require.False(t, getResp.IsError)
	var payload struct {
		Goal *json.RawMessage `json:"goal"`
	}
	require.NoError(t, json.Unmarshal([]byte(getResp.Content), &payload))
	require.Nil(t, payload.Goal)
}

func TestCompleteGoalValidatesInput(t *testing.T) {
	t.Parallel()

	completeTool := chattool.CompleteGoal(nil, chattool.GoalToolOptions{
		ChatID:     uuid.New(),
		RootChatID: uuid.New(),
		IsRootChat: true,
	})

	longSummaryInput := `{"goal_id":"00000000-0000-4000-8000-000000000001","summary":"` +
		strings.Repeat("x", codersdk.MaxChatGoalCompletionSummaryBytes+1) +
		`"}`

	for _, tt := range []struct {
		name    string
		input   string
		message string
	}{
		{
			name:    "missing goal id",
			input:   `{"summary":"done"}`,
			message: "goal_id is required",
		},
		{
			name:    "empty goal id",
			input:   `{"goal_id":"   ","summary":"done"}`,
			message: "goal_id is required",
		},
		{
			name:    "invalid goal id",
			input:   `{"goal_id":"not-a-uuid","summary":"done"}`,
			message: "goal_id is required",
		},
		{
			name:    "empty summary",
			input:   `{"goal_id":"00000000-0000-4000-8000-000000000001","summary":"  "}`,
			message: "summary is required",
		},
		{
			name:    "long summary",
			input:   longSummaryInput,
			message: "at most",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp, err := completeTool.Run(context.Background(), fantasy.ToolCall{
				ID:    "call-" + tt.name,
				Name:  chattool.CompleteGoalToolName,
				Input: tt.input,
			})
			require.NoError(t, err)
			require.True(t, resp.IsError)
			require.Contains(t, resp.Content, tt.message)
		})
	}
}

func TestCompleteGoalRejectsChildAndPaused(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "goal-tools-paused",
	})
	goal, err := db.InsertActiveChatGoal(dbauthz.AsSystemRestricted(ctx), database.InsertActiveChatGoalParams{
		RootChatID:      chat.ID,
		Objective:       "finish the work",
		CreatedByUserID: user.ID,
	})
	require.NoError(t, err)
	_, err = db.PauseChatGoalByID(dbauthz.AsSystemRestricted(ctx), database.PauseChatGoalByIDParams{
		RootChatID: chat.ID,
		ID:         goal.ID,
	})
	require.NoError(t, err)

	childTool := chattool.CompleteGoal(db, chattool.GoalToolOptions{
		ChatID:     chat.ID,
		RootChatID: chat.ID,
		IsRootChat: false,
	})
	childResp, err := childTool.Run(dbauthz.AsSystemRestricted(ctx), fantasy.ToolCall{
		ID:    "call-1",
		Name:  chattool.CompleteGoalToolName,
		Input: `{"goal_id":"` + goal.ID.String() + `","summary":"done"}`,
	})
	require.NoError(t, err)
	require.True(t, childResp.IsError)
	require.Contains(t, childResp.Content, "root chat")

	rootTool := chattool.CompleteGoal(db, chattool.GoalToolOptions{
		ChatID:     chat.ID,
		RootChatID: chat.ID,
		IsRootChat: true,
	})
	pausedResp, err := rootTool.Run(dbauthz.AsSystemRestricted(ctx), fantasy.ToolCall{
		ID:    "call-2",
		Name:  chattool.CompleteGoalToolName,
		Input: `{"goal_id":"` + goal.ID.String() + `","summary":"done"}`,
	})
	require.NoError(t, err)
	require.True(t, pausedResp.IsError)
	require.Contains(t, pausedResp.Content, "not active")
}

// A crash, takeover, or interrupt between the goal transition and its
// tool-result commit re-executes complete_goal; the replay must succeed
// from the durable row instead of erroring for work that already landed.
func TestCompleteGoalReplaysAgentCompletedGoal(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "goal-replay",
	})
	goal, err := db.InsertActiveChatGoal(dbauthz.AsSystemRestricted(ctx), database.InsertActiveChatGoalParams{
		RootChatID:      chat.ID,
		Objective:       "finish the work",
		CreatedByUserID: user.ID,
	})
	require.NoError(t, err)

	var publishes int
	completeTool := chattool.CompleteGoal(db, chattool.GoalToolOptions{
		ChatID:     chat.ID,
		RootChatID: chat.ID,
		IsRootChat: true,
		OnGoalUpdated: func(context.Context, database.Chat, database.ChatGoal) {
			publishes++
		},
	})
	input := `{"goal_id":"` + goal.ID.String() + `","summary":"done"}`
	firstResp, err := completeTool.Run(dbauthz.AsSystemRestricted(ctx), fantasy.ToolCall{
		ID:    "call-1",
		Name:  chattool.CompleteGoalToolName,
		Input: input,
	})
	require.NoError(t, err)
	require.False(t, firstResp.IsError)
	require.Equal(t, 1, publishes)

	replayResp, err := completeTool.Run(dbauthz.AsSystemRestricted(ctx), fantasy.ToolCall{
		ID:    "call-2",
		Name:  chattool.CompleteGoalToolName,
		Input: input,
	})
	require.NoError(t, err)
	require.False(t, replayResp.IsError, "re-executing a durably completed goal must replay success")
	var payload struct {
		Completed bool   `json:"completed"`
		Summary   string `json:"summary"`
	}
	require.NoError(t, json.Unmarshal([]byte(replayResp.Content), &payload))
	require.True(t, payload.Completed)
	require.Equal(t, "done", payload.Summary)
	require.Equal(t, 2, publishes,
		"a replay must re-publish the goal change because the original run may have crashed before publishing")
}

// A goal completed by the user is not proof the agent's call succeeded,
// so the tool must keep reporting an error instead of replaying success.
func TestCompleteGoalUserCompletedGoalDoesNotReplay(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "goal-user-complete",
	})
	goal, err := db.InsertActiveChatGoal(dbauthz.AsSystemRestricted(ctx), database.InsertActiveChatGoalParams{
		RootChatID:      chat.ID,
		Objective:       "finish the work",
		CreatedByUserID: user.ID,
	})
	require.NoError(t, err)
	_, err = db.CompleteChatGoalByID(dbauthz.AsSystemRestricted(ctx), database.CompleteChatGoalByIDParams{
		RootChatID:        chat.ID,
		ID:                goal.ID,
		CompletedByUserID: uuid.NullUUID{UUID: user.ID, Valid: true},
		CompletedByAgent:  false,
	})
	require.NoError(t, err)

	completeTool := chattool.CompleteGoal(db, chattool.GoalToolOptions{
		ChatID:     chat.ID,
		RootChatID: chat.ID,
		IsRootChat: true,
	})
	resp, err := completeTool.Run(dbauthz.AsSystemRestricted(ctx), fantasy.ToolCall{
		ID:    "call-1",
		Name:  chattool.CompleteGoalToolName,
		Input: `{"goal_id":"` + goal.ID.String() + `","summary":"done"}`,
	})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "not active")
}

// A crash, takeover, or interrupt between the goal transition and its
// tool-result commit re-executes block_goal; the replay must succeed
// from the durable row instead of erroring for work that already landed.
func TestBlockGoalReplaysAgentBlockedGoal(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "goal-block-replay",
	})
	goal, err := db.InsertActiveChatGoal(dbauthz.AsSystemRestricted(ctx), database.InsertActiveChatGoalParams{
		RootChatID:      chat.ID,
		Objective:       "finish the work",
		CreatedByUserID: user.ID,
	})
	require.NoError(t, err)

	var publishes int
	blockTool := chattool.BlockGoal(db, chattool.GoalToolOptions{
		ChatID:     chat.ID,
		RootChatID: chat.ID,
		IsRootChat: true,
		OnGoalUpdated: func(context.Context, database.Chat, database.ChatGoal) {
			publishes++
		},
	})
	input := `{"goal_id":"` + goal.ID.String() + `","reason":"waiting on user"}`
	firstResp, err := blockTool.Run(dbauthz.AsSystemRestricted(ctx), fantasy.ToolCall{
		ID:    "call-1",
		Name:  chattool.BlockGoalToolName,
		Input: input,
	})
	require.NoError(t, err)
	require.False(t, firstResp.IsError)
	require.Equal(t, 1, publishes)

	replayResp, err := blockTool.Run(dbauthz.AsSystemRestricted(ctx), fantasy.ToolCall{
		ID:    "call-2",
		Name:  chattool.BlockGoalToolName,
		Input: input,
	})
	require.NoError(t, err)
	require.False(t, replayResp.IsError, "re-executing a durably blocked goal must replay success")
	var payload struct {
		Blocked bool   `json:"blocked"`
		Reason  string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal([]byte(replayResp.Content), &payload))
	require.True(t, payload.Blocked)
	require.Equal(t, "waiting on user", payload.Reason)
	require.Equal(t, 2, publishes,
		"a replay must re-publish the goal change because the original run may have crashed before publishing")
}

// A blocked goal whose durable reason differs from the call's reason is
// not proof this call succeeded, so the tool must keep reporting an
// error instead of replaying success.
func TestBlockGoalDifferentReasonDoesNotReplay(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t, dbtestutil.WithDumpOnFailure())
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    org.ID,
		OwnerID:           user.ID,
		LastModelConfigID: model.ID,
		Title:             "goal-block-reason-mismatch",
	})
	goal, err := db.InsertActiveChatGoal(dbauthz.AsSystemRestricted(ctx), database.InsertActiveChatGoalParams{
		RootChatID:      chat.ID,
		Objective:       "finish the work",
		CreatedByUserID: user.ID,
	})
	require.NoError(t, err)
	_, err = db.BlockChatGoalByID(dbauthz.AsSystemRestricted(ctx), database.BlockChatGoalByIDParams{
		RootChatID:    chat.ID,
		ID:            goal.ID,
		BlockedReason: "different obstacle",
	})
	require.NoError(t, err)

	blockTool := chattool.BlockGoal(db, chattool.GoalToolOptions{
		ChatID:     chat.ID,
		RootChatID: chat.ID,
		IsRootChat: true,
	})
	resp, err := blockTool.Run(dbauthz.AsSystemRestricted(ctx), fantasy.ToolCall{
		ID:    "call-1",
		Name:  chattool.BlockGoalToolName,
		Input: `{"goal_id":"` + goal.ID.String() + `","reason":"waiting on user"}`,
	})
	require.NoError(t, err)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "not active")
}
