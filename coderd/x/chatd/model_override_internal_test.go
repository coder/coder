package chatd

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestResolveModelOverride(t *testing.T) {
	t.Parallel()

	flows := []struct {
		name string
		spec modelOverrideSpec
	}{
		{
			name: "Title",
			spec: modelOverrideSpec{context: titleGenerationOverrideContext},
		},
		{
			name: "Compaction",
			spec: modelOverrideSpec{
				context:                  compactionOverrideContext,
				ignoreUnavailable:        true,
				ignoreMissingCredentials: true,
			},
		},
		{
			name: "Subagent",
			spec: modelOverrideSpec{
				context:                  string(codersdk.ChatModelOverrideContextGeneral),
				ignoreUnavailable:        true,
				ignoreMissingCredentials: true,
			},
		},
		{
			name: "Advisor",
			spec: modelOverrideSpec{
				context:           advisorOverrideContext,
				ignoreUnavailable: true,
			},
		},
	}

	always := func(modelOverrideSpec) bool { return true }
	unlessIgnoreUnavailable := func(spec modelOverrideSpec) bool { return !spec.ignoreUnavailable }
	tests := []struct {
		name      string
		setup     func(*dbmock.MockStore, database.Chat, database.ChatModelConfig, uuid.UUID, modelOverrideSpec)
		wantErr   func(modelOverrideSpec) bool
		wantSet   bool
		wantModel bool
	}{
		{
			name: "NoRow",
			setup: func(db *dbmock.MockStore, chat database.Chat, _ database.ChatModelConfig, _ uuid.UUID, spec modelOverrideSpec) {
				db.EXPECT().GetChatOrganizationModelOverride(gomock.Any(), modelOverrideParams(chat, spec.context)).Return(database.ChatOrganizationModelOverride{}, sql.ErrNoRows)
			},
		},
		{
			name: "QueryError",
			setup: func(db *dbmock.MockStore, chat database.Chat, _ database.ChatModelConfig, _ uuid.UUID, spec modelOverrideSpec) {
				db.EXPECT().GetChatOrganizationModelOverride(gomock.Any(), modelOverrideParams(chat, spec.context)).Return(database.ChatOrganizationModelOverride{}, sql.ErrConnDone)
			},
			wantErr: always,
		},
		{
			name: "MissingConfig",
			setup: func(db *dbmock.MockStore, chat database.Chat, config database.ChatModelConfig, _ uuid.UUID, spec modelOverrideSpec) {
				db.EXPECT().GetChatOrganizationModelOverride(gomock.Any(), modelOverrideParams(chat, spec.context)).Return(orgModelOverride(chat, spec.context, config.ID, "high"), nil)
				db.EXPECT().GetChatModelConfigByID(gomock.Any(), config.ID).Return(database.ChatModelConfig{}, sql.ErrNoRows)
			},
			wantErr: unlessIgnoreUnavailable,
			wantSet: true,
		},
		{
			name: "NotAuthorizedConfig",
			setup: func(db *dbmock.MockStore, chat database.Chat, config database.ChatModelConfig, _ uuid.UUID, spec modelOverrideSpec) {
				db.EXPECT().GetChatOrganizationModelOverride(gomock.Any(), modelOverrideParams(chat, spec.context)).Return(orgModelOverride(chat, spec.context, config.ID, "high"), nil)
				db.EXPECT().GetChatModelConfigByID(gomock.Any(), config.ID).Return(database.ChatModelConfig{}, dbauthz.NotAuthorizedError{})
			},
			wantErr: unlessIgnoreUnavailable,
			wantSet: true,
		},
		{
			name: "ConfigError",
			setup: func(db *dbmock.MockStore, chat database.Chat, config database.ChatModelConfig, _ uuid.UUID, spec modelOverrideSpec) {
				db.EXPECT().GetChatOrganizationModelOverride(gomock.Any(), modelOverrideParams(chat, spec.context)).Return(orgModelOverride(chat, spec.context, config.ID, "high"), nil)
				db.EXPECT().GetChatModelConfigByID(gomock.Any(), config.ID).Return(database.ChatModelConfig{}, sql.ErrConnDone)
			},
			wantErr: always,
			wantSet: true,
		},
		{
			name: "DisabledProvider",
			setup: func(db *dbmock.MockStore, chat database.Chat, config database.ChatModelConfig, providerID uuid.UUID, spec modelOverrideSpec) {
				db.EXPECT().GetChatOrganizationModelOverride(gomock.Any(), modelOverrideParams(chat, spec.context)).Return(orgModelOverride(chat, spec.context, config.ID, "high"), nil)
				db.EXPECT().GetChatModelConfigByID(gomock.Any(), config.ID).Return(config, nil)
				db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(database.AIProvider{ID: providerID, Type: database.AIProviderTypeOpenai}, nil)
			},
			wantErr: unlessIgnoreUnavailable,
			wantSet: true,
		},
		{
			name: "InvalidConfig",
			setup: func(db *dbmock.MockStore, chat database.Chat, config database.ChatModelConfig, providerID uuid.UUID, spec modelOverrideSpec) {
				db.EXPECT().GetChatOrganizationModelOverride(gomock.Any(), modelOverrideParams(chat, spec.context)).Return(orgModelOverride(chat, spec.context, config.ID, "high"), nil)
				db.EXPECT().GetChatModelConfigByID(gomock.Any(), config.ID).Return(config, nil)
				db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(database.AIProvider{ID: providerID, Type: database.AIProviderType("invalid"), Enabled: true}, nil)
			},
			wantErr: unlessIgnoreUnavailable,
			wantSet: true,
		},
		{
			name: "ProviderError",
			setup: func(db *dbmock.MockStore, chat database.Chat, config database.ChatModelConfig, providerID uuid.UUID, spec modelOverrideSpec) {
				db.EXPECT().GetChatOrganizationModelOverride(gomock.Any(), modelOverrideParams(chat, spec.context)).Return(orgModelOverride(chat, spec.context, config.ID, "high"), nil)
				db.EXPECT().GetChatModelConfigByID(gomock.Any(), config.ID).Return(config, nil)
				db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(database.AIProvider{}, sql.ErrConnDone)
			},
			wantErr: always,
			wantSet: true,
		},
		{
			name: "MissingCredentials",
			setup: func(db *dbmock.MockStore, chat database.Chat, config database.ChatModelConfig, providerID uuid.UUID, spec modelOverrideSpec) {
				db.EXPECT().GetChatOrganizationModelOverride(gomock.Any(), modelOverrideParams(chat, spec.context)).Return(orgModelOverride(chat, spec.context, config.ID, "high"), nil)
				db.EXPECT().GetChatModelConfigByID(gomock.Any(), config.ID).Return(config, nil)
				db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(aibridgeTestAIProvider(providerID, "primary-openai", database.AIProviderTypeOpenai), nil).AnyTimes()
				db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return(nil, nil)
			},
			wantErr: func(spec modelOverrideSpec) bool { return !spec.ignoreMissingCredentials },
			wantSet: true,
		},
		{
			name: "Usable",
			setup: func(db *dbmock.MockStore, chat database.Chat, config database.ChatModelConfig, providerID uuid.UUID, spec modelOverrideSpec) {
				db.EXPECT().GetChatOrganizationModelOverride(gomock.Any(), modelOverrideParams(chat, spec.context)).Return(orgModelOverride(chat, spec.context, config.ID, "high"), nil)
				db.EXPECT().GetChatModelConfigByID(gomock.Any(), config.ID).Return(config, nil)
				db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(aibridgeTestAIProvider(providerID, "primary-openai", database.AIProviderTypeOpenai), nil).AnyTimes()
				db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return([]database.AIProviderKey{{ProviderID: providerID, APIKey: "test-key"}}, nil)
			},
			wantSet:   true,
			wantModel: true,
		},
	}

	for _, flow := range flows {
		for _, test := range tests {
			t.Run(flow.name+"/"+test.name, func(t *testing.T) {
				t.Parallel()

				ctx := testutil.Context(t, testutil.WaitShort)
				ctrl := gomock.NewController(t)
				db := dbmock.NewMockStore(ctrl)
				chat, _ := titleOverrideTestChatAndMessages(t)
				providerID := uuid.New()
				config := titleOverrideModelConfig("gpt-4.1", true)
				config.OrganizationID = chat.OrganizationID
				config.AIProviderID = uuid.NullUUID{UUID: providerID, Valid: true}
				spec := flow.spec
				spec.ownerID = chat.OwnerID
				spec.organizationID = chat.OrganizationID
				test.setup(db, chat, config, providerID, spec)

				server := titleOverrideTestServer(db, slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}))
				resolved, err := server.resolveModelOverride(ctx, spec)
				wantErr := test.wantErr != nil && test.wantErr(spec)
				if wantErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				require.Equal(t, test.wantSet && wantErr || test.wantModel, resolved.Set)
				if test.wantModel {
					require.Equal(t, config.ID, resolved.Config.ID)
					require.Equal(t, ptr.Ref("high"), resolved.ReasoningEffort)
					require.Equal(t, "openai", resolved.ResolvedProvider)
					require.Equal(t, "gpt-4.1", resolved.ResolvedModel)
				}
			})
		}
	}
}

func TestUserCanUseProviderKeys_AmbientCredentials(t *testing.T) {
	t.Parallel()

	keys := chatprovider.ProviderAPIKeys{ByProvider: map[string]string{"bedrock": ""}}
	require.True(t, userCanUseProviderKeys(keys, "bedrock"))

	keys = chatprovider.ProviderAPIKeys{ByProvider: map[string]string{"anthropic": ""}}
	require.True(t, userCanUseProviderKeys(keys, "anthropic"))

	require.False(t, userCanUseProviderKeys(chatprovider.ProviderAPIKeys{}, "anthropic"))
}

func modelOverrideParams(chat database.Chat, context string) database.GetChatOrganizationModelOverrideParams {
	return database.GetChatOrganizationModelOverrideParams{
		OrganizationID: chat.OrganizationID,
		Context:        context,
	}
}

func orgModelOverride(chat database.Chat, context string, modelConfigID uuid.UUID, effort string) database.ChatOrganizationModelOverride {
	return database.ChatOrganizationModelOverride{
		OrganizationID:  chat.OrganizationID,
		Context:         context,
		ModelConfigID:   modelConfigID,
		ReasoningEffort: sql.NullString{String: effort, Valid: effort != ""},
	}
}
