package chatopenai_test

import (
	"database/sql"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatopenai"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
)

func TestIsResponsesStoreEnabled(t *testing.T) {
	t.Parallel()

	storeTrue := true
	storeFalse := false

	tests := []struct {
		name string
		opts fantasy.ProviderOptions
		want bool
	}{
		{
			name: "NilOptions",
		},
		{
			name: "NonOpenAIKeysOnly",
			opts: fantasy.ProviderOptions{
				"other": &fantasyopenai.ProviderOptions{},
			},
		},
		{
			name: "OpenAIKeyWithNonResponsesOptions",
			opts: fantasy.ProviderOptions{
				fantasyopenai.Name: &fantasyopenai.ProviderOptions{},
			},
		},
		{
			name: "OpenAIKeyWithNilStore",
			opts: fantasy.ProviderOptions{
				fantasyopenai.Name: &fantasyopenai.ResponsesProviderOptions{},
			},
		},
		{
			name: "OpenAIKeyWithFalseStore",
			opts: fantasy.ProviderOptions{
				fantasyopenai.Name: &fantasyopenai.ResponsesProviderOptions{Store: &storeFalse},
			},
		},
		{
			name: "OpenAIKeyWithTrueStore",
			opts: fantasy.ProviderOptions{
				fantasyopenai.Name: &fantasyopenai.ResponsesProviderOptions{Store: &storeTrue},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := chatopenai.IsResponsesStoreEnabled(tt.opts)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestIsResponsesStoreEnabledIgnoresMalformedNonOpenAIKey(t *testing.T) {
	t.Parallel()

	store := true
	// This intentionally documents the only synthetic mismatch from the old
	// chatloop value scan: a malformed map with OpenAI Responses options under a
	// non-OpenAI key is not treated as enabled.
	opts := fantasy.ProviderOptions{
		"not-openai": &fantasyopenai.ResponsesProviderOptions{Store: &store},
	}

	require.False(t, chatopenai.IsResponsesStoreEnabled(opts))
}

func TestShouldActivateChainMode(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	baseInfo := chatopenai.ResolveChainMode([]database.ChatMessage{
		chainModeSystemMessage(),
		chainModeUserMessage("prior user message"),
		chainModeAssistantMessage(modelConfigID, nil),
		chainModeUserMessage("latest user message"),
	})

	localCall := codersdk.ChatMessageToolCall(
		"call-local",
		"read_file",
		json.RawMessage(`{"path":"main.go"}`),
	)
	unresolvedLocalInfo := chatopenai.ResolveChainMode([]database.ChatMessage{
		chainModeSystemMessage(),
		chainModeUserMessage("prior user message"),
		chainModeAssistantMessage(modelConfigID, []codersdk.ChatMessagePart{localCall}),
		chainModeUserMessage("latest user message"),
	})
	localResult := codersdk.ChatMessageToolResult(
		"call-local",
		"read_file",
		json.RawMessage(`{"ok":true}`),
		false,
		false,
	)
	missingToolResultsInfo := chatopenai.ResolveChainMode([]database.ChatMessage{
		chainModeSystemMessage(),
		chainModeUserMessage("prior user message"),
		chainModeAssistantMessage(modelConfigID, []codersdk.ChatMessagePart{localCall}),
		chainModeToolMessage([]codersdk.ChatMessagePart{localResult}),
		chainModeUserMessage("latest user message"),
	})
	skillOnlyInfo := chatopenai.ResolveChainMode([]database.ChatMessage{
		chainModeSystemMessage(),
		chainModeUserMessage("prior user message"),
		chainModeAssistantMessage(modelConfigID, nil),
		chainModeSkillOnlyUserMessage(),
	})
	missingResponseInfo := chatopenai.ResolveChainMode([]database.ChatMessage{
		chainModeSystemMessage(),
		chainModeUserMessage("prior user message"),
		chainModeAssistantMessageWithoutResponse(modelConfigID),
		chainModeUserMessage("latest user message"),
	})

	tests := []struct {
		name           string
		providerOpts   fantasy.ProviderOptions
		info           chatopenai.ChainModeInfo
		modelConfigID  uuid.UUID
		isPlanModeTurn bool
		want           bool
	}{
		{
			name:          "StoreDisabled",
			providerOpts:  chainModeProviderOptions(false),
			info:          baseInfo,
			modelConfigID: modelConfigID,
		},
		{
			name:          "MissingPreviousResponseID",
			providerOpts:  chainModeProviderOptions(true),
			info:          missingResponseInfo,
			modelConfigID: modelConfigID,
		},
		{
			name:          "MismatchedModelConfigID",
			providerOpts:  chainModeProviderOptions(true),
			info:          baseInfo,
			modelConfigID: uuid.New(),
		},
		{
			name:           "PlanMode",
			providerOpts:   chainModeProviderOptions(true),
			info:           baseInfo,
			modelConfigID:  modelConfigID,
			isPlanModeTurn: true,
		},
		{
			name:          "NoContributingTrailingUser",
			providerOpts:  chainModeProviderOptions(true),
			info:          skillOnlyInfo,
			modelConfigID: modelConfigID,
		},
		{
			name:          "UnresolvedLocalToolCalls",
			providerOpts:  chainModeProviderOptions(true),
			info:          unresolvedLocalInfo,
			modelConfigID: modelConfigID,
		},
		{
			name:          "ProviderMissingToolResults",
			providerOpts:  chainModeProviderOptions(true),
			info:          missingToolResultsInfo,
			modelConfigID: modelConfigID,
		},
		{
			name:          "AllConditionsMet",
			providerOpts:  chainModeProviderOptions(true),
			info:          baseInfo,
			modelConfigID: modelConfigID,
			want:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := chatopenai.ShouldActivateChainMode(
				tt.providerOpts,
				tt.info,
				tt.modelConfigID,
				tt.isPlanModeTurn,
			)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestWithPreviousResponseID(t *testing.T) {
	t.Parallel()

	store := true
	originalResponses := &fantasyopenai.ResponsesProviderOptions{Store: &store}
	otherOptions := &fantasyopenai.ProviderOptions{}
	opts := fantasy.ProviderOptions{
		fantasyopenai.Name: originalResponses,
		"other":            otherOptions,
	}

	got := chatopenai.WithPreviousResponseID(opts, "resp-next")

	gotOtherOptions, ok := got["other"].(*fantasyopenai.ProviderOptions)
	require.True(t, ok)
	require.True(t, otherOptions == gotOtherOptions)
	gotOriginalResponses, ok := opts[fantasyopenai.Name].(*fantasyopenai.ResponsesProviderOptions)
	require.True(t, ok)
	require.True(t, originalResponses == gotOriginalResponses)
	require.Nil(t, originalResponses.PreviousResponseID)

	clonedResponses, ok := got[fantasyopenai.Name].(*fantasyopenai.ResponsesProviderOptions)
	require.True(t, ok)
	require.NotSame(t, originalResponses, clonedResponses)
	require.NotNil(t, clonedResponses.PreviousResponseID)
	require.Equal(t, "resp-next", *clonedResponses.PreviousResponseID)
	require.True(t, originalResponses.Store == clonedResponses.Store)

	got["new"] = otherOptions
	require.NotContains(t, opts, "new")
}

func TestWithPreviousResponseIDNilInput(t *testing.T) {
	t.Parallel()

	got := chatopenai.WithPreviousResponseID(nil, "resp-next")

	require.NotNil(t, got)
	require.Empty(t, got)
}

func TestHasPreviousResponseID(t *testing.T) {
	t.Parallel()

	emptyID := ""
	responseID := "resp-123"

	tests := []struct {
		name string
		opts fantasy.ProviderOptions
		want bool
	}{
		{
			name: "NilOptions",
		},
		{
			name: "EmptyID",
			opts: fantasy.ProviderOptions{
				fantasyopenai.Name: &fantasyopenai.ResponsesProviderOptions{
					PreviousResponseID: &emptyID,
				},
			},
		},
		{
			name: "NonEmptyID",
			opts: fantasy.ProviderOptions{
				fantasyopenai.Name: &fantasyopenai.ResponsesProviderOptions{
					PreviousResponseID: &responseID,
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := chatopenai.HasPreviousResponseID(tt.opts)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestClearPreviousResponseID(t *testing.T) {
	t.Parallel()

	responseID := "resp-123"
	options := &fantasyopenai.ResponsesProviderOptions{
		PreviousResponseID: &responseID,
	}
	otherOptions := &fantasyopenai.ProviderOptions{}
	opts := fantasy.ProviderOptions{
		fantasyopenai.Name: options,
		"other":            otherOptions,
	}

	got := chatopenai.ClearPreviousResponseID(opts)

	got["new"] = otherOptions
	require.NotContains(t, opts, "new")
	require.NotNil(t, options.PreviousResponseID)
	require.Equal(t, "resp-123", *options.PreviousResponseID)

	gotOtherOptions, ok := got["other"].(*fantasyopenai.ProviderOptions)
	require.True(t, ok)
	require.True(t, otherOptions == gotOtherOptions)
	clonedOptions, ok := got[fantasyopenai.Name].(*fantasyopenai.ResponsesProviderOptions)
	require.True(t, ok)
	require.NotSame(t, options, clonedOptions)
	require.Nil(t, clonedOptions.PreviousResponseID)

	require.NotPanics(t, func() {
		got := chatopenai.ClearPreviousResponseID(nil)
		require.NotNil(t, got)
		chatopenai.ClearPreviousResponseID(fantasy.ProviderOptions{
			fantasyopenai.Name: &fantasyopenai.ProviderOptions{},
		})
	})
}

func TestExtractResponseIDIfStoredMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata fantasy.ProviderMetadata
		want     string
	}{
		{
			name: "NilMetadata",
		},
		{
			name: "NoResponsesMetadata",
			metadata: fantasy.ProviderMetadata{
				"other": &fantasyopenai.ProviderOptions{},
			},
		},
		{
			name: "ResponsesMetadataUnderNonOpenAIKey",
			metadata: fantasy.ProviderMetadata{
				"other": &fantasyopenai.ResponsesProviderMetadata{
					ResponseID: "resp-123",
				},
			},
		},
		{
			name: "ResponsesMetadata",
			metadata: fantasy.ProviderMetadata{
				fantasyopenai.Name: &fantasyopenai.ResponsesProviderMetadata{
					ResponseID: "resp-123",
				},
			},
			want: "resp-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := chatopenai.ExtractResponseIDIfStored(
				chainModeProviderOptions(true),
				tt.metadata,
			)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestExtractResponseIDIfStored(t *testing.T) {
	t.Parallel()

	metadata := fantasy.ProviderMetadata{
		fantasyopenai.Name: &fantasyopenai.ResponsesProviderMetadata{
			ResponseID: "resp-123",
		},
	}

	require.Empty(t, chatopenai.ExtractResponseIDIfStored(
		chainModeProviderOptions(false),
		metadata,
	))
	require.Equal(t, "resp-123", chatopenai.ExtractResponseIDIfStored(
		chainModeProviderOptions(true),
		metadata,
	))
}

func TestResolveChainModeIgnoresSkillOnlySentinelMessages(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	assistant := database.ChatMessage{
		Role:               database.ChatMessageRoleAssistant,
		ProviderResponseID: sql.NullString{String: "resp-123", Valid: true},
		ModelConfigID:      uuid.NullUUID{UUID: modelConfigID, Valid: true},
	}
	skillOnly := chainModeSkillOnlyUserMessage()
	user := chattest.ChatMessageWithParts([]codersdk.ChatMessagePart{{
		Type: codersdk.ChatMessagePartTypeText,
		Text: "latest user message",
	}})
	user.Role = database.ChatMessageRoleUser

	got := chatopenai.ResolveChainMode([]database.ChatMessage{assistant, skillOnly, user})
	require.Equal(t, "resp-123", got.PreviousResponseID())
	require.Equal(t, modelConfigID, got.ModelConfigID())
	require.Equal(t, 1, got.ContributingTrailingUserCount())
}

func TestResolveChainMode_BlocksOnUnresolvedLocalToolCall(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	toolCall := codersdk.ChatMessageToolCall(
		"call-local",
		"read_file",
		json.RawMessage(`{"path":"main.go"}`),
	)

	chainInfo := chatopenai.ResolveChainMode([]database.ChatMessage{
		chainModeSystemMessage(),
		chainModeUserMessage("prior user message"),
		chainModeAssistantMessage(modelConfigID, []codersdk.ChatMessagePart{toolCall}),
		chainModeUserMessage("latest user message"),
	})

	require.Equal(t, "resp-123", chainInfo.PreviousResponseID())
	require.True(t, chainInfo.HasUnresolvedLocalToolCalls())
	require.False(t, chatopenai.ShouldActivateChainMode(
		chainModeProviderOptions(true),
		chainInfo,
		modelConfigID,
		false,
	))
}

func TestResolveChainMode_BlocksWhenAssistantContentCannotParse(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	chainInfo := chatopenai.ResolveChainMode([]database.ChatMessage{
		chainModeSystemMessage(),
		chainModeUserMessage("prior user message"),
		chainModeCorruptAssistantMessage(modelConfigID),
		chainModeUserMessage("latest user message"),
	})

	require.Equal(t, "resp-123", chainInfo.PreviousResponseID())
	require.True(t, chainInfo.HasUnresolvedLocalToolCalls())
	require.False(t, chatopenai.ShouldActivateChainMode(
		chainModeProviderOptions(true),
		chainInfo,
		modelConfigID,
		false,
	))
}

func TestResolveChainMode_BlocksWhenToolContentCannotParse(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	toolCall := codersdk.ChatMessageToolCall(
		"call-local",
		"read_file",
		json.RawMessage(`{"path":"main.go"}`),
	)

	chainInfo := chatopenai.ResolveChainMode([]database.ChatMessage{
		chainModeSystemMessage(),
		chainModeUserMessage("prior user message"),
		chainModeAssistantMessage(modelConfigID, []codersdk.ChatMessagePart{toolCall}),
		chainModeCorruptToolMessage(),
		chainModeUserMessage("latest user message"),
	})

	require.Equal(t, "resp-123", chainInfo.PreviousResponseID())
	require.True(t, chainInfo.HasUnresolvedLocalToolCalls())
	require.False(t, chatopenai.ShouldActivateChainMode(
		chainModeProviderOptions(true),
		chainInfo,
		modelConfigID,
		false,
	))
}

func TestResolveChainMode_AllowsProviderExecutedOnly(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	toolCall := codersdk.ChatMessageToolCall(
		"call-web-search",
		"web_search",
		json.RawMessage(`{"query":"coder docs"}`),
	)
	toolCall.ProviderExecuted = true

	chainInfo := chatopenai.ResolveChainMode([]database.ChatMessage{
		chainModeSystemMessage(),
		chainModeUserMessage("prior user message"),
		chainModeAssistantMessage(modelConfigID, []codersdk.ChatMessagePart{toolCall}),
		chainModeUserMessage("latest user message"),
	})

	require.Equal(t, "resp-123", chainInfo.PreviousResponseID())
	require.False(t, chainInfo.HasUnresolvedLocalToolCalls())
	require.False(t, chainInfo.ProviderMissingToolResults())
	require.True(t, chatopenai.ShouldActivateChainMode(
		chainModeProviderOptions(true),
		chainInfo,
		modelConfigID,
		false,
	))
}

func TestResolveChainMode_BlocksOnMixedProviderExecutedAndUnresolvedLocalCall(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	providerCall := codersdk.ChatMessageToolCall(
		"call-web-search",
		"web_search",
		json.RawMessage(`{"query":"coder docs"}`),
	)
	providerCall.ProviderExecuted = true
	localCall := codersdk.ChatMessageToolCall(
		"call-local",
		"read_file",
		json.RawMessage(`{"path":"main.go"}`),
	)

	chainInfo := chatopenai.ResolveChainMode([]database.ChatMessage{
		chainModeSystemMessage(),
		chainModeUserMessage("prior user message"),
		chainModeAssistantMessage(
			modelConfigID,
			[]codersdk.ChatMessagePart{providerCall, localCall},
		),
		chainModeUserMessage("latest user message"),
	})

	require.Equal(t, "resp-123", chainInfo.PreviousResponseID())
	require.True(t, chainInfo.HasUnresolvedLocalToolCalls())
	require.False(t, chatopenai.ShouldActivateChainMode(
		chainModeProviderOptions(true),
		chainInfo,
		modelConfigID,
		false,
	))
}

func TestResolveChainMode_AllowsResolvedLocalCall(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	toolCall := codersdk.ChatMessageToolCall(
		"call-local",
		"read_file",
		json.RawMessage(`{"path":"main.go"}`),
	)
	toolResult := codersdk.ChatMessageToolResult(
		"call-local",
		"read_file",
		json.RawMessage(`{"ok":true}`),
		false,
		false,
	)
	followUp := chainModeAssistantMessage(modelConfigID, nil)
	followUp.ProviderResponseID = sql.NullString{String: "resp-follow-up", Valid: true}

	chainInfo := chatopenai.ResolveChainMode([]database.ChatMessage{
		chainModeSystemMessage(),
		chainModeUserMessage("prior user message"),
		chainModeAssistantMessage(modelConfigID, []codersdk.ChatMessagePart{toolCall}),
		chainModeToolMessage([]codersdk.ChatMessagePart{toolResult}),
		followUp,
		chainModeUserMessage("latest user message"),
	})

	require.Equal(t, "resp-follow-up", chainInfo.PreviousResponseID())
	require.False(t, chainInfo.HasUnresolvedLocalToolCalls())
	require.False(t, chainInfo.ProviderMissingToolResults())
	require.True(t, chatopenai.ShouldActivateChainMode(
		chainModeProviderOptions(true),
		chainInfo,
		modelConfigID,
		false,
	))
}

func TestResolveChainMode_BlocksOnMixedResolvedAndUnresolved(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	firstCall := codersdk.ChatMessageToolCall(
		"call-first",
		"read_file",
		json.RawMessage(`{"path":"main.go"}`),
	)
	secondCall := codersdk.ChatMessageToolCall(
		"call-second",
		"read_file",
		json.RawMessage(`{"path":"README.md"}`),
	)
	toolResult := codersdk.ChatMessageToolResult(
		"call-first",
		"read_file",
		json.RawMessage(`{"ok":true}`),
		false,
		false,
	)

	chainInfo := chatopenai.ResolveChainMode([]database.ChatMessage{
		chainModeSystemMessage(),
		chainModeUserMessage("prior user message"),
		chainModeAssistantMessage(
			modelConfigID,
			[]codersdk.ChatMessagePart{firstCall, secondCall},
		),
		chainModeToolMessage([]codersdk.ChatMessagePart{toolResult}),
		chainModeUserMessage("latest user message"),
	})

	require.Equal(t, "resp-123", chainInfo.PreviousResponseID())
	require.True(t, chainInfo.HasUnresolvedLocalToolCalls())
	require.False(t, chatopenai.ShouldActivateChainMode(
		chainModeProviderOptions(true),
		chainInfo,
		modelConfigID,
		false,
	))
}

func TestResolveChainMode_BlocksWhenToolResultNeverSentToProvider(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	toolCall := codersdk.ChatMessageToolCall(
		"call-local",
		"propose_plan",
		json.RawMessage(`{"path":"plan.md"}`),
	)
	toolResult := codersdk.ChatMessageToolResult(
		"call-local",
		"propose_plan",
		json.RawMessage(`{"ok":true}`),
		false,
		false,
	)

	chainInfo := chatopenai.ResolveChainMode([]database.ChatMessage{
		chainModeSystemMessage(),
		chainModeUserMessage("make a plan"),
		chainModeAssistantMessage(modelConfigID, []codersdk.ChatMessagePart{toolCall}),
		chainModeToolMessage([]codersdk.ChatMessagePart{toolResult}),
		chainModeUserMessage("implement the plan"),
	})

	require.Equal(t, "resp-123", chainInfo.PreviousResponseID())
	require.False(t, chainInfo.HasUnresolvedLocalToolCalls())
	require.True(t, chainInfo.ProviderMissingToolResults())
	require.False(t, chatopenai.ShouldActivateChainMode(
		chainModeProviderOptions(true),
		chainInfo,
		modelConfigID,
		false,
	))
}

func TestResolveChainMode_BlocksProviderMissingWithMultipleToolCalls(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	call1 := codersdk.ChatMessageToolCall(
		"call-1",
		"propose_plan",
		json.RawMessage(`{"path":"plan.md"}`),
	)
	call2 := codersdk.ChatMessageToolCall(
		"call-2",
		"write_file",
		json.RawMessage(`{"path":"foo.go"}`),
	)
	result1 := codersdk.ChatMessageToolResult(
		"call-1",
		"propose_plan",
		json.RawMessage(`{"ok":true}`),
		false,
		false,
	)
	result2 := codersdk.ChatMessageToolResult(
		"call-2",
		"write_file",
		json.RawMessage(`{"ok":true}`),
		false,
		false,
	)

	chainInfo := chatopenai.ResolveChainMode([]database.ChatMessage{
		chainModeSystemMessage(),
		chainModeUserMessage("do it"),
		chainModeAssistantMessage(modelConfigID, []codersdk.ChatMessagePart{call1, call2}),
		chainModeToolMessage([]codersdk.ChatMessagePart{result1, result2}),
		chainModeUserMessage("next"),
	})

	require.False(t, chainInfo.HasUnresolvedLocalToolCalls())
	require.True(t, chainInfo.ProviderMissingToolResults())
	require.False(t, chatopenai.ShouldActivateChainMode(
		chainModeProviderOptions(true),
		chainInfo,
		modelConfigID,
		false,
	))
}

func TestResolveChainMode_AllowsWhenNoToolCalls(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()

	chainInfo := chatopenai.ResolveChainMode([]database.ChatMessage{
		chainModeSystemMessage(),
		chainModeUserMessage("hello"),
		chainModeAssistantMessage(modelConfigID, nil),
		chainModeUserMessage("thanks"),
	})

	require.Equal(t, "resp-123", chainInfo.PreviousResponseID())
	require.False(t, chainInfo.HasUnresolvedLocalToolCalls())
	require.False(t, chainInfo.ProviderMissingToolResults())
	require.True(t, chatopenai.ShouldActivateChainMode(
		chainModeProviderOptions(true),
		chainInfo,
		modelConfigID,
		false,
	))
}

func TestFilterPromptForChainModeKeepsContributingUsersAcrossSkippedSentinelTurns(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	priorUser := chattest.ChatMessageWithParts([]codersdk.ChatMessagePart{{
		Type: codersdk.ChatMessagePartTypeText,
		Text: "prior user message",
	}})
	priorUser.Role = database.ChatMessageRoleUser
	assistant := database.ChatMessage{
		Role:               database.ChatMessageRoleAssistant,
		ProviderResponseID: sql.NullString{String: "resp-123", Valid: true},
		ModelConfigID:      uuid.NullUUID{UUID: modelConfigID, Valid: true},
	}
	firstTrailingUser := chattest.ChatMessageWithParts([]codersdk.ChatMessagePart{{
		Type: codersdk.ChatMessagePartTypeText,
		Text: "first trailing user",
	}})
	firstTrailingUser.Role = database.ChatMessageRoleUser
	skillOnly := chainModeSkillOnlyUserMessage()
	lastTrailingUser := chattest.ChatMessageWithParts([]codersdk.ChatMessagePart{{
		Type: codersdk.ChatMessagePartTypeText,
		Text: "last trailing user",
	}})
	lastTrailingUser.Role = database.ChatMessageRoleUser

	chainInfo := chatopenai.ResolveChainMode([]database.ChatMessage{
		priorUser,
		assistant,
		firstTrailingUser,
		skillOnly,
		lastTrailingUser,
	})
	require.Equal(t, 2, chainInfo.ContributingTrailingUserCount())

	prompt := []fantasy.Message{
		{
			Role: fantasy.MessageRoleSystem,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "system instruction"},
			},
		},
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "prior user message"},
			},
		},
		{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "assistant reply"},
			},
		},
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "first trailing user"},
			},
		},
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "last trailing user"},
			},
		},
	}

	got := chatopenai.FilterPromptForChainMode(prompt, chainInfo)
	require.Len(t, got, 3)
	require.Equal(t, fantasy.MessageRoleSystem, got[0].Role)
	require.Equal(t, fantasy.MessageRoleUser, got[1].Role)
	require.Equal(t, fantasy.MessageRoleUser, got[2].Role)

	firstPart, ok := fantasy.AsMessagePart[fantasy.TextPart](got[1].Content[0])
	require.True(t, ok)
	require.Equal(t, "first trailing user", firstPart.Text)
	lastPart, ok := fantasy.AsMessagePart[fantasy.TextPart](got[2].Content[0])
	require.True(t, ok)
	require.Equal(t, "last trailing user", lastPart.Text)
}

func TestFilterPromptForChainModeUsesContributingTrailingUsers(t *testing.T) {
	t.Parallel()

	modelConfigID := uuid.New()
	priorUser := chattest.ChatMessageWithParts([]codersdk.ChatMessagePart{{
		Type: codersdk.ChatMessagePartTypeText,
		Text: "prior user message",
	}})
	priorUser.Role = database.ChatMessageRoleUser
	assistant := database.ChatMessage{
		Role:               database.ChatMessageRoleAssistant,
		ProviderResponseID: sql.NullString{String: "resp-123", Valid: true},
		ModelConfigID:      uuid.NullUUID{UUID: modelConfigID, Valid: true},
	}
	skillOnly := chainModeSkillOnlyUserMessage()
	latestUser := chattest.ChatMessageWithParts([]codersdk.ChatMessagePart{{
		Type: codersdk.ChatMessagePartTypeText,
		Text: "latest user message",
	}})
	latestUser.Role = database.ChatMessageRoleUser

	chainInfo := chatopenai.ResolveChainMode([]database.ChatMessage{
		priorUser,
		assistant,
		skillOnly,
		latestUser,
	})
	require.Equal(t, 1, chainInfo.ContributingTrailingUserCount())

	prompt := []fantasy.Message{
		{
			Role: fantasy.MessageRoleSystem,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "system instruction"},
			},
		},
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "prior user message"},
			},
		},
		{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "assistant reply"},
			},
		},
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "latest user message"},
			},
		},
	}

	got := chatopenai.FilterPromptForChainMode(prompt, chainInfo)
	require.Len(t, got, 2)
	require.Equal(t, fantasy.MessageRoleSystem, got[0].Role)
	require.Equal(t, fantasy.MessageRoleUser, got[1].Role)

	part, ok := fantasy.AsMessagePart[fantasy.TextPart](got[1].Content[0])
	require.True(t, ok)
	require.Equal(t, "latest user message", part.Text)
}

func chainModeProviderOptions(store bool) fantasy.ProviderOptions {
	return fantasy.ProviderOptions{
		fantasyopenai.Name: &fantasyopenai.ResponsesProviderOptions{
			Store: &store,
		},
	}
}

func chainModeSystemMessage() database.ChatMessage {
	return database.ChatMessage{Role: database.ChatMessageRoleSystem}
}

func chainModeUserMessage(text string) database.ChatMessage {
	msg := chattest.ChatMessageWithParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText(text),
	})
	msg.Role = database.ChatMessageRoleUser
	return msg
}

func chainModeSkillOnlyUserMessage() database.ChatMessage {
	msg := chattest.ChatMessageWithParts([]codersdk.ChatMessagePart{
		{
			Type: codersdk.ChatMessagePartTypeContextFile,
			// Keep this in sync with chatd.AgentChatContextSentinelPath.
			ContextFilePath: ".coder/agent-chat-context-sentinel",
			ContextFileAgentID: uuid.NullUUID{
				UUID:  uuid.New(),
				Valid: true,
			},
		},
		{
			Type:      codersdk.ChatMessagePartTypeSkill,
			SkillName: "repo-helper",
			SkillDir:  "/skills/repo-helper",
		},
	})
	msg.Role = database.ChatMessageRoleUser
	return msg
}

func chainModeAssistantMessage(
	modelConfigID uuid.UUID,
	parts []codersdk.ChatMessagePart,
) database.ChatMessage {
	msg := chattest.ChatMessageWithParts(parts)
	msg.Role = database.ChatMessageRoleAssistant
	msg.ProviderResponseID = sql.NullString{String: "resp-123", Valid: true}
	msg.ModelConfigID = uuid.NullUUID{UUID: modelConfigID, Valid: true}
	return msg
}

func chainModeAssistantMessageWithoutResponse(
	modelConfigID uuid.UUID,
) database.ChatMessage {
	msg := chattest.ChatMessageWithParts(nil)
	msg.Role = database.ChatMessageRoleAssistant
	msg.ModelConfigID = uuid.NullUUID{UUID: modelConfigID, Valid: true}
	return msg
}

func chainModeCorruptAssistantMessage(modelConfigID uuid.UUID) database.ChatMessage {
	return database.ChatMessage{
		Role:               database.ChatMessageRoleAssistant,
		ProviderResponseID: sql.NullString{String: "resp-123", Valid: true},
		ModelConfigID:      uuid.NullUUID{UUID: modelConfigID, Valid: true},
		Content: pqtype.NullRawMessage{
			RawMessage: []byte("not json"),
			Valid:      true,
		},
		ContentVersion: chatprompt.CurrentContentVersion,
	}
}

func chainModeCorruptToolMessage() database.ChatMessage {
	return database.ChatMessage{
		Role: database.ChatMessageRoleTool,
		Content: pqtype.NullRawMessage{
			RawMessage: []byte("not json"),
			Valid:      true,
		},
		ContentVersion: chatprompt.CurrentContentVersion,
	}
}

func chainModeToolMessage(parts []codersdk.ChatMessagePart) database.ChatMessage {
	msg := chattest.ChatMessageWithParts(parts)
	msg.Role = database.ChatMessageRoleTool
	return msg
}
