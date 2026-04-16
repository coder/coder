package chatexec_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentchat/chatexec"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

func TestMessagesFromSDK(t *testing.T) {
	t.Parallel()

	logger := slog.Make()
	metadata := mustMarshalProviderMetadata(t, fantasy.ProviderMetadata{
		"anthropic": &fantasyanthropic.ProviderCacheControlOptions{
			CacheControl: fantasyanthropic.CacheControl{Type: "ephemeral"},
		},
	})

	msgs := []agentsdk.ChatRunnerMessage{
		{
			Role: string(codersdk.ChatMessageRoleSystem),
			Text: "be concise",
		},
		{
			Role: string(codersdk.ChatMessageRoleUser),
			Content: []codersdk.ChatMessagePart{
				{
					Type:             codersdk.ChatMessagePartTypeText,
					Text:             "question",
					ProviderMetadata: metadata,
				},
				{
					Type: codersdk.ChatMessagePartTypeReasoning,
					Text: "thinking",
				},
			},
		},
		{
			Role: string(codersdk.ChatMessageRoleAssistant),
			Content: []codersdk.ChatMessagePart{{
				Type:       codersdk.ChatMessagePartTypeToolCall,
				ToolCallID: "call-1",
				ToolName:   "search",
				Args:       json.RawMessage(`{"query":"mux"}`),
			}},
		},
		{
			Role: string(codersdk.ChatMessageRoleTool),
			Content: []codersdk.ChatMessagePart{{
				Type:       codersdk.ChatMessagePartTypeToolResult,
				ToolCallID: "call-1",
				ToolName:   "search",
				Result:     json.RawMessage(`{"answer":"done"}`),
			}},
		},
	}

	got, err := chatexec.MessagesFromSDK(logger, msgs)
	require.NoError(t, err)
	require.Len(t, got, 4)

	require.Equal(t, fantasy.MessageRoleSystem, got[0].Role)
	require.Len(t, got[0].Content, 1)
	systemText, ok := fantasy.AsMessagePart[fantasy.TextPart](got[0].Content[0])
	require.True(t, ok)
	require.Equal(t, "be concise", systemText.Text)

	require.Equal(t, fantasy.MessageRoleUser, got[1].Role)
	require.Len(t, got[1].Content, 2)
	userText, ok := fantasy.AsMessagePart[fantasy.TextPart](got[1].Content[0])
	require.True(t, ok)
	require.Equal(t, "question", userText.Text)
	cacheControl := fantasyanthropic.GetCacheControl(userText.ProviderOptions)
	require.NotNil(t, cacheControl)
	require.Equal(t, "ephemeral", cacheControl.Type)
	reasoning, ok := fantasy.AsMessagePart[fantasy.ReasoningPart](got[1].Content[1])
	require.True(t, ok)
	require.Equal(t, "thinking", reasoning.Text)

	require.Equal(t, fantasy.MessageRoleAssistant, got[2].Role)
	toolCall, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](got[2].Content[0])
	require.True(t, ok)
	require.Equal(t, "call-1", toolCall.ToolCallID)
	require.Equal(t, "search", toolCall.ToolName)
	require.Equal(t, `{"query":"mux"}`, toolCall.Input)

	require.Equal(t, fantasy.MessageRoleTool, got[3].Role)
	toolResult, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](got[3].Content[0])
	require.True(t, ok)
	require.Equal(t, "call-1", toolResult.ToolCallID)
	textOutput, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](toolResult.Output)
	require.True(t, ok)
	require.Equal(t, `{"answer":"done"}`, textOutput.Text)
}

func TestMessagesFromSDKNilInput(t *testing.T) {
	t.Parallel()

	got, err := chatexec.MessagesFromSDK(slog.Make(), nil)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestMessagesFromSDKUnknownRole(t *testing.T) {
	t.Parallel()

	_, err := chatexec.MessagesFromSDK(slog.Make(), []agentsdk.ChatRunnerMessage{{
		Role: "mystery",
		Text: "hello",
	}})
	require.Error(t, err)
	require.ErrorContains(t, err, `unsupported chat runner message role "mystery"`)
}

func TestContentFromParts(t *testing.T) {
	t.Parallel()

	logger := slog.Make()
	metadata := mustMarshalProviderMetadata(t, fantasy.ProviderMetadata{
		"anthropic": &fantasyanthropic.ProviderCacheControlOptions{
			CacheControl: fantasyanthropic.CacheControl{Type: "ephemeral"},
		},
	})

	parts := []codersdk.ChatMessagePart{
		{
			Type:             codersdk.ChatMessagePartTypeText,
			Text:             "hello",
			ProviderMetadata: metadata,
		},
		{
			Type: codersdk.ChatMessagePartTypeReasoning,
			Text: "thinking",
		},
		{
			Type:             codersdk.ChatMessagePartTypeToolCall,
			ToolCallID:       "call-1",
			ToolName:         "search",
			Args:             json.RawMessage(`{"query":"mux"}`),
			ProviderExecuted: true,
		},
		{
			Type:       codersdk.ChatMessagePartTypeToolResult,
			ToolCallID: "call-err",
			ToolName:   "search",
			Result:     json.RawMessage(`{"error":"boom"}`),
			IsError:    true,
		},
		{
			Type:       codersdk.ChatMessagePartTypeToolResult,
			ToolCallID: "call-media",
			ToolName:   "screenshot",
			Result:     json.RawMessage(`{"data":"YWJj","mime_type":"image/png","text":"preview"}`),
			IsMedia:    true,
		},
		{
			Type:       codersdk.ChatMessagePartTypeToolResult,
			ToolCallID: "call-empty",
			ToolName:   "search",
		},
	}

	got, err := chatexec.ContentFromParts(logger, parts)
	require.NoError(t, err)
	require.Len(t, got, len(parts))

	text, ok := fantasy.AsContentType[fantasy.TextContent](got[0])
	require.True(t, ok)
	require.Equal(t, "hello", text.Text)
	cacheControl, ok := text.ProviderMetadata["anthropic"].(*fantasyanthropic.ProviderCacheControlOptions)
	require.True(t, ok)
	require.Equal(t, "ephemeral", cacheControl.CacheControl.Type)

	reasoning, ok := fantasy.AsContentType[fantasy.ReasoningContent](got[1])
	require.True(t, ok)
	require.Equal(t, "thinking", reasoning.Text)

	toolCall, ok := fantasy.AsContentType[fantasy.ToolCallContent](got[2])
	require.True(t, ok)
	require.Equal(t, "call-1", toolCall.ToolCallID)
	require.Equal(t, "search", toolCall.ToolName)
	require.Equal(t, `{"query":"mux"}`, toolCall.Input)
	require.True(t, toolCall.ProviderExecuted)

	errResult, ok := fantasy.AsContentType[fantasy.ToolResultContent](got[3])
	require.True(t, ok)
	errOutput, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentError](errResult.Result)
	require.True(t, ok)
	require.EqualError(t, errOutput.Error, "boom")

	mediaResult, ok := fantasy.AsContentType[fantasy.ToolResultContent](got[4])
	require.True(t, ok)
	mediaOutput, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentMedia](mediaResult.Result)
	require.True(t, ok)
	require.Equal(t, "YWJj", mediaOutput.Data)
	require.Equal(t, "image/png", mediaOutput.MediaType)
	require.Equal(t, "preview", mediaOutput.Text)

	emptyResult, ok := fantasy.AsContentType[fantasy.ToolResultContent](got[5])
	require.True(t, ok)
	emptyOutput, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](emptyResult.Result)
	require.True(t, ok)
	require.Equal(t, "{}", emptyOutput.Text)
}

func TestContentFromPartsWarnsOnBadMetadata(t *testing.T) {
	t.Parallel()

	logger, sink := newCaptureLogger()
	got, err := chatexec.ContentFromParts(logger, []codersdk.ChatMessagePart{{
		Type:             codersdk.ChatMessagePartTypeText,
		Text:             "hello",
		ProviderMetadata: json.RawMessage(`{"anthropic":`),
	}})
	require.NoError(t, err)
	require.Len(t, got, 1)

	text, ok := fantasy.AsContentType[fantasy.TextContent](got[0])
	require.True(t, ok)
	require.Nil(t, text.ProviderMetadata)

	entries := sink.entriesSnapshot()
	require.Len(t, entries, 1)
	require.Equal(t, slog.LevelWarn, entries[0].Level)
	require.Equal(t, "failed to unmarshal provider metadata", entries[0].Message)
}

func TestContentFromPartsNilInput(t *testing.T) {
	t.Parallel()

	got, err := chatexec.ContentFromParts(slog.Make(), nil)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestContentFromPartsUnknownPartType(t *testing.T) {
	t.Parallel()

	_, err := chatexec.ContentFromParts(slog.Make(), []codersdk.ChatMessagePart{{
		Type: codersdk.ChatMessagePartType("mystery"),
	}})
	require.Error(t, err)
	require.ErrorContains(t, err, `unsupported chat message part "mystery"`)
}

func TestSplitPersistedContent(t *testing.T) {
	t.Parallel()

	assistantParts, toolResults := chatexec.SplitPersistedContent([]fantasy.Content{
		fantasy.TextContent{Text: "hello"},
		fantasy.ToolCallContent{
			ToolCallID: "call-1",
			ToolName:   "search",
			Input:      `{"query":"mux"}`,
		},
		fantasy.ToolResultContent{
			ToolCallID:       "call-provider",
			ToolName:         "web_search",
			Result:           fantasy.ToolResultOutputContentText{Text: `{"cached":true}`},
			ProviderExecuted: true,
		},
		fantasy.ReasoningContent{Text: "thinking"},
		fantasy.ToolResultContent{
			ToolCallID: "call-1",
			ToolName:   "search",
			Result:     fantasy.ToolResultOutputContentText{Text: `{"answer":"done"}`},
		},
	})

	require.Len(t, assistantParts, 4)
	require.Len(t, toolResults, 1)

	require.Equal(t, codersdk.ChatMessagePartTypeText, assistantParts[0].Type)
	require.Equal(t, codersdk.ChatMessagePartTypeToolCall, assistantParts[1].Type)
	require.Equal(t, codersdk.ChatMessagePartTypeToolResult, assistantParts[2].Type)
	require.True(t, assistantParts[2].ProviderExecuted)
	require.Equal(t, codersdk.ChatMessagePartTypeReasoning, assistantParts[3].Type)

	require.Equal(t, codersdk.ChatMessagePartTypeToolResult, toolResults[0].Type)
	require.Equal(t, "call-1", toolResults[0].ToolCallID)
	require.False(t, toolResults[0].ProviderExecuted)
}

func TestSplitPersistedContentEmpty(t *testing.T) {
	t.Parallel()

	assistantParts, toolResults := chatexec.SplitPersistedContent(nil)
	require.Nil(t, assistantParts)
	require.Nil(t, toolResults)
}

func TestUsageToSDK(t *testing.T) {
	t.Parallel()

	got := chatexec.UsageToSDK(fantasy.Usage{
		InputTokens:         11,
		OutputTokens:        12,
		TotalTokens:         23,
		ReasoningTokens:     7,
		CacheCreationTokens: 5,
		CacheReadTokens:     3,
	})
	require.NotNil(t, got)
	require.EqualValues(t, 11, got.InputTokens)
	require.EqualValues(t, 12, got.OutputTokens)
	require.EqualValues(t, 23, got.TotalTokens)
	require.EqualValues(t, 7, got.ReasoningTokens)
	require.EqualValues(t, 5, got.CacheCreationTokens)
	require.EqualValues(t, 3, got.CacheReadTokens)
}

func TestUsageToSDKZeroUsageReturnsNil(t *testing.T) {
	t.Parallel()

	require.Nil(t, chatexec.UsageToSDK(fantasy.Usage{}))
}

type captureSink struct {
	mu      sync.Mutex
	entries []slog.SinkEntry
}

func (s *captureSink) LogEntry(_ context.Context, entry slog.SinkEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
}

func (*captureSink) Sync() {}

func (s *captureSink) entriesSnapshot() []slog.SinkEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]slog.SinkEntry(nil), s.entries...)
}

func newCaptureLogger() (slog.Logger, *captureSink) {
	sink := &captureSink{}
	return slog.Make(sink), sink
}

func mustMarshalProviderMetadata(
	t testing.TB,
	metadata fantasy.ProviderMetadata,
) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(metadata)
	require.NoError(t, err)
	return data
}
