package chattool_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"charm.land/fantasy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/testutil"
)

func runNotesTool(ctx context.Context, t *testing.T, tool fantasy.AgentTool, args chattool.NotesArgs) (fantasy.ToolResponse, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(args)
	require.NoError(t, err)
	response, err := tool.Run(ctx, fantasy.ToolCall{ID: "call-1", Name: chattool.NotesToolName, Input: string(raw)})
	require.NoError(t, err)
	var result map[string]any
	if !response.IsError {
		require.NoError(t, json.Unmarshal([]byte(response.Content), &result))
	}
	return response, result
}

func TestNotesTool(t *testing.T) {
	t.Parallel()
	ctx := dbauthz.AsChatd(testutil.Context(t, testutil.WaitLong))
	db, _ := dbtestutil.NewDB(t)
	user := dbgen.User(t, db, database.User{})
	org := dbgen.Organization(t, db, database.Organization{})
	modelCfg := seedModelConfig(t, db)
	chat := dbgen.Chat(t, db, database.Chat{OrganizationID: org.ID, OwnerID: user.ID, LastModelConfigID: modelCfg.ID})
	other := dbgen.Chat(t, db, database.Chat{OrganizationID: org.ID, OwnerID: user.ID, LastModelConfigID: modelCfg.ID})
	authzDB := dbauthz.New(db, rbac.NewStrictCachingAuthorizer(prometheus.NewRegistry()), slogtest.Make(t, nil), testAccessControlStorePointer())

	tool := chattool.Notes(chattool.NewNotesStore(authzDB, chat.ID))
	otherTool := chattool.Notes(chattool.NewNotesStore(authzDB, other.ID))

	// write, append, read.
	resp, _ := runNotesTool(ctx, t, tool, chattool.NotesArgs{Action: "write", Path: "checkpoint.md", Content: "goal"})
	require.False(t, resp.IsError, resp.Content)
	resp, _ = runNotesTool(ctx, t, tool, chattool.NotesArgs{Action: "append", Path: "checkpoint.md", Content: "next"})
	require.False(t, resp.IsError, resp.Content)
	resp, result := runNotesTool(ctx, t, tool, chattool.NotesArgs{Action: "read", Path: "checkpoint.md"})
	require.False(t, resp.IsError, resp.Content)
	require.Equal(t, "goal\nnext", result["content"])
	// write replaces.
	resp, _ = runNotesTool(ctx, t, tool, chattool.NotesArgs{Action: "write", Path: "checkpoint.md", Content: "replaced"})
	require.False(t, resp.IsError, resp.Content)
	_, result = runNotesTool(ctx, t, tool, chattool.NotesArgs{Action: "read", Path: "checkpoint.md"})
	require.Equal(t, "replaced", result["content"])
	// append creates a missing note.
	resp, _ = runNotesTool(ctx, t, tool, chattool.NotesArgs{Action: "append", Path: "findings/a.md", Content: "first"})
	require.False(t, resp.IsError, resp.Content)

	// list.
	resp, result = runNotesTool(ctx, t, tool, chattool.NotesArgs{Action: "list"})
	require.False(t, resp.IsError, resp.Content)
	notes, ok := result["notes"].([]any)
	require.True(t, ok)
	require.Len(t, notes, 2)
	first, ok := notes[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "checkpoint.md", first["path"])
	require.EqualValues(t, len("replaced"), first["bytes"])

	// Missing note and unknown action.
	resp, _ = runNotesTool(ctx, t, tool, chattool.NotesArgs{Action: "read", Path: "missing.md"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "does not exist")
	resp, _ = runNotesTool(ctx, t, tool, chattool.NotesArgs{Action: "delete", Path: "checkpoint.md"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "unknown action")

	// Bad paths.
	for _, path := range []string{"", "Checkpoint.md", "../x", "/abs", "a b", strings.Repeat("a", 129)} {
		resp, _ = runNotesTool(ctx, t, tool, chattool.NotesArgs{Action: "write", Path: path, Content: "x"})
		require.True(t, resp.IsError, path)
		require.Contains(t, resp.Content, "invalid note path", path)
	}

	// 16 KiB cap on write and on append.
	resp, _ = runNotesTool(ctx, t, tool, chattool.NotesArgs{Action: "write", Path: "big.md", Content: strings.Repeat("b", chattool.MaxNoteBytes+1)})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "bytes at most")
	resp, _ = runNotesTool(ctx, t, tool, chattool.NotesArgs{Action: "write", Path: "big.md", Content: strings.Repeat("b", chattool.MaxNoteBytes)})
	require.False(t, resp.IsError, resp.Content)
	resp, _ = runNotesTool(ctx, t, tool, chattool.NotesArgs{Action: "append", Path: "big.md", Content: "b"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "bytes at most")

	// 20-file cap: three notes exist, so 17 more fit.
	for i := range chattool.MaxNotesPerChat - 3 {
		resp, _ = runNotesTool(ctx, t, tool, chattool.NotesArgs{Action: "write", Path: fmt.Sprintf("n%02d.md", i), Content: "x"})
		require.False(t, resp.IsError, resp.Content)
	}
	resp, _ = runNotesTool(ctx, t, tool, chattool.NotesArgs{Action: "write", Path: "one-too-many.md", Content: "x"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "the limit")
	// Existing notes stay writable at the cap.
	resp, _ = runNotesTool(ctx, t, tool, chattool.NotesArgs{Action: "append", Path: "checkpoint.md", Content: "more"})
	require.False(t, resp.IsError, resp.Content)

	// Another chat does not see these notes.
	resp, result = runNotesTool(ctx, t, otherTool, chattool.NotesArgs{Action: "list"})
	require.False(t, resp.IsError, resp.Content)
	require.Empty(t, result["notes"])
	resp, _ = runNotesTool(ctx, t, otherTool, chattool.NotesArgs{Action: "read", Path: "checkpoint.md"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "does not exist")
}
