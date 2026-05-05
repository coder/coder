package dbgen_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
)

func TestGenerator(t *testing.T) {
	t.Parallel()

	t.Run("AuditLog", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		_ = dbgen.AuditLog(t, db, database.AuditLog{})
		logs := must(db.GetAuditLogsOffset(context.Background(), database.GetAuditLogsOffsetParams{LimitOpt: 1}))
		require.Len(t, logs, 1)
	})

	t.Run("APIKey", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp, _ := dbgen.APIKey(t, db, database.APIKey{})
		require.Equal(t, exp, must(db.GetAPIKeyByID(context.Background(), exp.ID)))
	})

	t.Run("File", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		exp := dbgen.File(t, db, database.File{})
		require.Equal(t, exp, must(db.GetFileByID(context.Background(), exp.ID)))
	})

	t.Run("UserLink", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		u := dbgen.User(t, db, database.User{})
		exp := dbgen.UserLink(t, db, database.UserLink{UserID: u.ID})
		require.Equal(t, exp, must(db.GetUserLinkByLinkedID(context.Background(), exp.LinkedID)))
	})

	t.Run("GitAuthLink", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		exp := dbgen.ExternalAuthLink(t, db, database.ExternalAuthLink{})
		require.Equal(t, exp, must(db.GetExternalAuthLink(context.Background(), database.GetExternalAuthLinkParams{
			ProviderID: exp.ProviderID,
			UserID:     exp.UserID,
		})))
	})

	t.Run("WorkspaceResource", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{})
		require.Equal(t, exp, must(db.GetWorkspaceResourceByID(context.Background(), exp.ID)))
	})

	t.Run("WorkspaceApp", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.WorkspaceApp(t, db, database.WorkspaceApp{})
		require.Equal(t, exp, must(db.GetWorkspaceAppsByAgentID(context.Background(), exp.AgentID))[0])
	})

	t.Run("WorkspaceResourceMetadata", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.WorkspaceResourceMetadatums(t, db, database.WorkspaceResourceMetadatum{})
		require.Equal(t, exp, must(db.GetWorkspaceResourceMetadataByResourceIDs(context.Background(), []uuid.UUID{exp[0].WorkspaceResourceID})))
	})

	t.Run("WorkspaceProxy", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		exp, secret := dbgen.WorkspaceProxy(t, db, database.WorkspaceProxy{})
		require.Len(t, secret, 64)
		require.Equal(t, exp, must(db.GetWorkspaceProxyByID(context.Background(), exp.ID)))
	})

	t.Run("Job", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		exp := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{})
		require.Equal(t, exp, must(db.GetProvisionerJobByID(context.Background(), exp.ID)))
	})

	t.Run("Group", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.Group(t, db, database.Group{})
		require.Equal(t, exp, must(db.GetGroupByID(context.Background(), exp.ID)))
	})

	t.Run("GroupMember", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		g := dbgen.Group(t, db, database.Group{})
		u := dbgen.User(t, db, database.User{})
		gm := dbgen.GroupMember(t, db, database.GroupMemberTable{GroupID: g.ID, UserID: u.ID})
		exp := []database.GroupMember{gm}

		require.Equal(t, exp, must(db.GetGroupMembersByGroupID(context.Background(), database.GetGroupMembersByGroupIDParams{
			GroupID:       g.ID,
			IncludeSystem: false,
		})))
	})

	t.Run("Organization", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		exp := dbgen.Organization(t, db, database.Organization{})
		require.Equal(t, exp, must(db.GetOrganizationByID(context.Background(), exp.ID)))
	})

	t.Run("OrganizationMember", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		o := dbgen.Organization(t, db, database.Organization{})
		u := dbgen.User(t, db, database.User{})
		exp := dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: o.ID, UserID: u.ID})
		require.Equal(t, exp, must(database.ExpectOne(db.OrganizationMembers(context.Background(), database.OrganizationMembersParams{
			OrganizationID: exp.OrganizationID,
			UserID:         exp.UserID,
		}))).OrganizationMember)
	})

	t.Run("Workspace", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		u := dbgen.User(t, db, database.User{})
		org := dbgen.Organization(t, db, database.Organization{})
		tpl := dbgen.Template(t, db, database.Template{
			OrganizationID: org.ID,
			CreatedBy:      u.ID,
		})
		exp := dbgen.Workspace(t, db, database.WorkspaceTable{
			OwnerID:        u.ID,
			OrganizationID: org.ID,
			TemplateID:     tpl.ID,
		})
		w := must(db.GetWorkspaceByID(context.Background(), exp.ID))
		table := database.WorkspaceTable{
			ID:                w.ID,
			CreatedAt:         w.CreatedAt,
			UpdatedAt:         w.UpdatedAt,
			OwnerID:           w.OwnerID,
			OrganizationID:    w.OrganizationID,
			TemplateID:        w.TemplateID,
			Deleted:           w.Deleted,
			Name:              w.Name,
			AutostartSchedule: w.AutostartSchedule,
			Ttl:               w.Ttl,
			LastUsedAt:        w.LastUsedAt,
			DormantAt:         w.DormantAt,
			DeletingAt:        w.DeletingAt,
			AutomaticUpdates:  w.AutomaticUpdates,
			Favorite:          w.Favorite,
			GroupACL:          database.WorkspaceACL{},
			UserACL:           database.WorkspaceACL{},
		}
		require.Equal(t, exp, table)
	})

	t.Run("WorkspaceAgent", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{})
		require.Equal(t, exp, must(db.GetWorkspaceAgentByID(context.Background(), exp.ID)))
	})

	t.Run("Template", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.Template(t, db, database.Template{})
		require.Equal(t, exp, must(db.GetTemplateByID(context.Background(), exp.ID)))
	})

	t.Run("TemplateVersion", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.TemplateVersion(t, db, database.TemplateVersion{})
		require.Equal(t, exp, must(db.GetTemplateVersionByID(context.Background(), exp.ID)))
	})

	t.Run("WorkspaceBuild", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{})
		require.Equal(t, exp, must(db.GetWorkspaceBuildByID(context.Background(), exp.ID)))
	})

	t.Run("User", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		exp := dbgen.User(t, db, database.User{})
		require.Equal(t, exp, must(db.GetUserByID(context.Background(), exp.ID)))
	})

	t.Run("ServiceAccountUser", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		user := dbgen.User(t, db, database.User{
			IsServiceAccount: true,
			Email:            "should-be-overridden@coder.com",
			LoginType:        database.LoginTypePassword,
		})
		require.True(t, user.IsServiceAccount)
		require.Empty(t, user.Email)
		require.Equal(t, database.LoginTypeNone, user.LoginType)
		require.Equal(t, user, must(db.GetUserByID(context.Background(), user.ID)))
	})

	t.Run("SSHKey", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.GitSSHKey(t, db, database.GitSSHKey{})
		require.Equal(t, exp, must(db.GetGitSSHKey(context.Background(), exp.UserID)))
	})

	t.Run("WorkspaceBuildParameters", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.WorkspaceBuildParameters(t, db, []database.WorkspaceBuildParameter{{Name: "name1", Value: "value1"}, {Name: "name2", Value: "value2"}, {Name: "name3", Value: "value3"}})
		require.Equal(t, exp, must(db.GetWorkspaceBuildParameters(context.Background(), exp[0].WorkspaceBuildID)))
	})

	t.Run("TemplateVersionParameter", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		dbtestutil.DisableForeignKeysAndTriggers(t, db)
		exp := dbgen.TemplateVersionParameter(t, db, database.TemplateVersionParameter{})
		actual := must(db.GetTemplateVersionParameters(context.Background(), exp.TemplateVersionID))
		require.Len(t, actual, 1)
		require.Equal(t, exp, actual[0])
	})

	t.Run("ChatProvider", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)

		// Defaults.
		p := dbgen.ChatProvider(t, db, database.ChatProvider{})
		require.NotEqual(t, uuid.Nil, p.ID)
		require.Equal(t, "openai", p.Provider)
		require.Equal(t, "openai", p.DisplayName)
		require.True(t, p.Enabled)
		require.True(t, p.CentralApiKeyEnabled)
		require.Equal(t, "test-key", p.APIKey)

		// Overrides.
		p2 := dbgen.ChatProvider(t, db, database.ChatProvider{
			Provider:    "anthropic",
			DisplayName: "Claude",
			APIKey:      "sk-custom",
		})
		require.Equal(t, "anthropic", p2.Provider)
		require.Equal(t, "Claude", p2.DisplayName)
		require.Equal(t, "sk-custom", p2.APIKey)

		p3 := dbgen.ChatProvider(t, db, database.ChatProvider{
			Provider: "openrouter",
		}, func(params *database.InsertChatProviderParams) {
			params.APIKey = ""
		})
		require.Empty(t, p3.APIKey)
	})

	t.Run("ChatModelConfig", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		_ = dbgen.ChatProvider(t, db, database.ChatProvider{})

		// Defaults.
		cfg := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{})
		require.NotEqual(t, uuid.Nil, cfg.ID)
		require.Equal(t, "openai", cfg.Provider)
		require.Equal(t, "gpt-4o-mini", cfg.Model)
		require.Equal(t, "Test Model", cfg.DisplayName)
		require.True(t, cfg.Enabled)
		require.Equal(t, int64(128000), cfg.ContextLimit)
		require.Equal(t, int32(70), cfg.CompressionThreshold)

		// Overrides.
		_ = dbgen.ChatProvider(t, db, database.ChatProvider{Provider: "anthropic"})
		cfg2 := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
			Provider:     "anthropic",
			Model:        "claude-4",
			ContextLimit: 200000,
		})
		require.Equal(t, "anthropic", cfg2.Provider)
		require.Equal(t, "claude-4", cfg2.Model)
		require.Equal(t, int64(200000), cfg2.ContextLimit)
	})

	t.Run("Chat", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		u := dbgen.User(t, db, database.User{})
		o := dbgen.Organization(t, db, database.Organization{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         u.ID,
			OrganizationID: o.ID,
		})
		p := dbgen.ChatProvider(t, db, database.ChatProvider{})
		m := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{Provider: p.Provider})

		// Defaults.
		chat := dbgen.Chat(t, db, database.Chat{
			OwnerID:           u.ID,
			OrganizationID:    o.ID,
			LastModelConfigID: m.ID,
		})
		require.NotEqual(t, uuid.Nil, chat.ID)
		require.Equal(t, database.ChatStatusWaiting, chat.Status)
		require.Equal(t, database.ChatClientTypeUi, chat.ClientType)
		require.NotEmpty(t, chat.Title)

		// Overrides.
		chat2 := dbgen.Chat(t, db, database.Chat{
			OwnerID:           u.ID,
			OrganizationID:    o.ID,
			LastModelConfigID: m.ID,
			Title:             "custom-title",
			Status:            database.ChatStatusRunning,
		})
		require.Equal(t, "custom-title", chat2.Title)
		require.Equal(t, database.ChatStatusRunning, chat2.Status)
	})

	t.Run("ChatMessage", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)
		u := dbgen.User(t, db, database.User{})
		o := dbgen.Organization(t, db, database.Organization{})
		dbgen.OrganizationMember(t, db, database.OrganizationMember{
			UserID:         u.ID,
			OrganizationID: o.ID,
		})
		p := dbgen.ChatProvider(t, db, database.ChatProvider{})
		m := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{Provider: p.Provider})
		chat := dbgen.Chat(t, db, database.Chat{
			OwnerID:           u.ID,
			OrganizationID:    o.ID,
			LastModelConfigID: m.ID,
		})

		// Defaults.
		msg := dbgen.ChatMessage(t, db, database.ChatMessage{
			ChatID: chat.ID,
		})
		require.NotZero(t, msg.ID)
		require.Equal(t, database.ChatMessageRoleUser, msg.Role)
		require.Equal(t, database.ChatMessageVisibilityBoth, msg.Visibility)
		require.Equal(t, chatprompt.CurrentContentVersion, msg.ContentVersion)

		// Overrides.
		rawContent := json.RawMessage(`[{"type":"text","text":"hello"}]`)
		msg2 := dbgen.ChatMessage(t, db, database.ChatMessage{
			ChatID: chat.ID,
			Role:   database.ChatMessageRoleAssistant,
			Content: pqtype.NullRawMessage{
				RawMessage: rawContent,
				Valid:      true,
			},
			InputTokens:         sql.NullInt64{Int64: 11, Valid: true},
			OutputTokens:        sql.NullInt64{Int64: 22, Valid: true},
			TotalTokens:         sql.NullInt64{Int64: 33, Valid: true},
			ReasoningTokens:     sql.NullInt64{Int64: 44, Valid: true},
			CacheCreationTokens: sql.NullInt64{Int64: 55, Valid: true},
			CacheReadTokens:     sql.NullInt64{Int64: 66, Valid: true},
			ContextLimit:        sql.NullInt64{Int64: 77, Valid: true},
			Compressed:          true,
			TotalCostMicros:     sql.NullInt64{Int64: 88, Valid: true},
			ProviderResponseID:  sql.NullString{String: "resp-123", Valid: true},
		})
		require.Equal(t, database.ChatMessageRoleAssistant, msg2.Role)
		require.True(t, msg2.Content.Valid)
		require.JSONEq(t, string(rawContent), string(msg2.Content.RawMessage))
		require.Equal(t, sql.NullInt64{Int64: 11, Valid: true}, msg2.InputTokens)
		require.Equal(t, sql.NullInt64{Int64: 22, Valid: true}, msg2.OutputTokens)
		require.Equal(t, sql.NullInt64{Int64: 33, Valid: true}, msg2.TotalTokens)
		require.Equal(t, sql.NullInt64{Int64: 44, Valid: true}, msg2.ReasoningTokens)
		require.Equal(t, sql.NullInt64{Int64: 55, Valid: true}, msg2.CacheCreationTokens)
		require.Equal(t, sql.NullInt64{Int64: 66, Valid: true}, msg2.CacheReadTokens)
		require.Equal(t, sql.NullInt64{Int64: 77, Valid: true}, msg2.ContextLimit)
		require.True(t, msg2.Compressed)
		require.Equal(t, sql.NullInt64{Int64: 88, Valid: true}, msg2.TotalCostMicros)
		require.Equal(t, sql.NullString{String: "resp-123", Valid: true}, msg2.ProviderResponseID)
	})

	t.Run("MCPServerConfig", func(t *testing.T) {
		t.Parallel()
		db, _ := dbtestutil.NewDB(t)

		// Defaults.
		cfg := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{})
		require.NotEqual(t, uuid.Nil, cfg.ID)
		require.Equal(t, "streamable_http", cfg.Transport)
		require.Equal(t, "none", cfg.AuthType)
		require.Equal(t, "default_off", cfg.Availability)
		require.True(t, cfg.Enabled)
		require.Empty(t, cfg.ToolAllowList)
		require.Empty(t, cfg.ToolDenyList)
		require.NotEmpty(t, cfg.Slug)
		require.NotEmpty(t, cfg.Url)

		// Overrides.
		cfg2 := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
			DisplayName:     "Custom MCP",
			Slug:            "custom-mcp",
			Url:             "https://custom.example.com",
			AuthType:        "oauth2",
			AllowInPlanMode: true,
		})
		require.Equal(t, "Custom MCP", cfg2.DisplayName)
		require.Equal(t, "custom-mcp", cfg2.Slug)
		require.Equal(t, "https://custom.example.com", cfg2.Url)
		require.Equal(t, "oauth2", cfg2.AuthType)
		require.True(t, cfg2.AllowInPlanMode)
	})
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
