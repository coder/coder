package chatd //nolint:testpackage // Exercises unexported tool-allow-list helpers.

import (
	"database/sql"
	"encoding/json"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
)

func TestParseBuiltinToolAllowSet(t *testing.T) {
	t.Parallel()

	t.Run("NullAllowsAll", func(t *testing.T) {
		t.Parallel()
		set, restrict, err := parseBuiltinToolAllowSet(pqtype.NullRawMessage{})
		require.NoError(t, err)
		require.False(t, restrict)
		require.Nil(t, set)
	})

	t.Run("EmptyArrayAllowsNone", func(t *testing.T) {
		t.Parallel()
		set, restrict, err := parseBuiltinToolAllowSet(pqtype.NullRawMessage{
			RawMessage: json.RawMessage(`[]`),
			Valid:      true,
		})
		require.NoError(t, err)
		require.True(t, restrict)
		require.Empty(t, set)
	})

	t.Run("SubsetAllowsListed", func(t *testing.T) {
		t.Parallel()
		set, restrict, err := parseBuiltinToolAllowSet(pqtype.NullRawMessage{
			RawMessage: json.RawMessage(`["read_file","execute"]`),
			Valid:      true,
		})
		require.NoError(t, err)
		require.True(t, restrict)
		require.Equal(t, map[string]bool{"read_file": true, "execute": true}, set)
	})

	t.Run("MalformedReturnsError", func(t *testing.T) {
		t.Parallel()
		_, _, err := parseBuiltinToolAllowSet(pqtype.NullRawMessage{
			RawMessage: json.RawMessage(`{`),
			Valid:      true,
		})
		require.Error(t, err)
	})
}

// TestPrepareGenerationBuiltinToolAllowList verifies that the builtin_tools
// column controls which built-in tools prepareGeneration advertises. The
// allow-list is applied before MCP, dynamic, and provider tools, so it only
// affects Coder built-in tools.
func TestPrepareGenerationBuiltinToolAllowList(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T, builtinTools pqtype.NullRawMessage) (*Server, database.Chat) {
		t.Helper()
		db, ps := dbtestutil.NewDB(t)
		ctx := chatdTestContext(t)

		user := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         user.ID,
			OrganizationID: org.ID,
		})
		dbgen.ChatProvider(t, db, database.ChatProvider{
			Provider:    "openai",
			DisplayName: "OpenAI",
			APIKey:      "test-key",
			Enabled:     true,
			CreatedBy:   uuid.NullUUID{UUID: user.ID, Valid: true},
		})
		modelCfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
			Provider:    "openai",
			Model:       "gpt-4o-mini",
			DisplayName: "gpt-4o-mini",
			Options:     json.RawMessage(`{}`),
		}, func(p *database.InsertChatModelConfigParams) {
			p.Enabled = true
			p.IsDefault = true
		})
		apiKey, _ := dbgen.APIKey(t, db, database.APIKey{UserID: user.ID})

		created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
			OrganizationID:    org.ID,
			OwnerID:           user.ID,
			LastModelConfigID: modelCfg.ID,
			Title:             "builtin-tools",
			ClientType:        database.ChatClientTypeApi,
			BuiltinTools:      builtinTools,
			InitialMessages: []chatstate.Message{
				{
					Role:           database.ChatMessageRoleUser,
					Content:        mustMarshalText(t, "hello"),
					Visibility:     database.ChatMessageVisibilityBoth,
					ContentVersion: chatprompt.CurrentContentVersion,
					CreatedBy:      uuid.NullUUID{UUID: user.ID, Valid: true},
					ModelConfigID:  uuid.NullUUID{UUID: modelCfg.ID, Valid: true},
					APIKeyID:       sql.NullString{String: apiKey.ID, Valid: true},
				},
			},
		})
		require.NoError(t, err)

		server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})
		return server, created.Chat
	}

	builtinNames := func(t *testing.T, server *Server, chat database.Chat) []string {
		t.Helper()
		ctx := chatdTestContext(t)
		prepared, err := server.prepareGeneration(ctx, generationPrepareInput{Chat: chat})
		require.NoError(t, err)
		t.Cleanup(prepared.Cleanup)
		names := make([]string, 0, len(prepared.BuiltinToolNames))
		for name := range prepared.BuiltinToolNames {
			names = append(names, name)
		}
		slices.Sort(names)
		return names
	}

	t.Run("NilAllowsAllBuiltins", func(t *testing.T) {
		t.Parallel()
		server, chat := setup(t, pqtype.NullRawMessage{})
		names := builtinNames(t, server, chat)
		// A root chat with no workspace still exposes the file, execute,
		// template, and sub-agent built-ins by default.
		require.Contains(t, names, "read_file")
		require.Contains(t, names, "execute")
		require.Contains(t, names, "create_workspace")
		require.Contains(t, names, "spawn_agent")
	})

	t.Run("EmptyRemovesAllBuiltins", func(t *testing.T) {
		t.Parallel()
		server, chat := setup(t, pqtype.NullRawMessage{
			RawMessage: json.RawMessage(`[]`),
			Valid:      true,
		})
		names := builtinNames(t, server, chat)
		require.Empty(t, names)
	})

	t.Run("SubsetKeepsOnlyListed", func(t *testing.T) {
		t.Parallel()
		server, chat := setup(t, pqtype.NullRawMessage{
			RawMessage: json.RawMessage(`["read_file","execute"]`),
			Valid:      true,
		})
		names := builtinNames(t, server, chat)
		require.Equal(t, []string{"execute", "read_file"}, names)
	})
}
