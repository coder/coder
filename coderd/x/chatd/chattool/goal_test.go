package chattool_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
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

	_, err = db.GetCurrentChatGoalByRootChatID(dbauthz.AsSystemRestricted(ctx), chat.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
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
