package chatloop

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"iter"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatsanitize"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func validWebSearchProviderMetadataForTest() fantasy.ProviderMetadata {
	return fantasy.ProviderMetadata{
		fantasyanthropic.Name: &fantasyanthropic.WebSearchResultMetadata{
			Results: []fantasyanthropic.WebSearchResultItem{
				{
					URL:              "https://example.com",
					Title:            "Example",
					EncryptedContent: "encrypted",
				},
			},
		},
	}
}

func safeToolCallContent(block fantasy.Content) (fantasy.ToolCallContent, bool) {
	var zero fantasy.ToolCallContent
	switch value := block.(type) {
	case fantasy.ToolCallContent:
		return value, true
	case *fantasy.ToolCallContent:
		if value == nil {
			return zero, false
		}
		return *value, true
	default:
		return zero, false
	}
}

func safeToolResultContent(block fantasy.Content) (fantasy.ToolResultContent, bool) {
	var zero fantasy.ToolResultContent
	switch value := block.(type) {
	case fantasy.ToolResultContent:
		return value, true
	case *fantasy.ToolResultContent:
		if value == nil {
			return zero, false
		}
		return *value, true
	default:
		return zero, false
	}
}

func toolCallContentToPart(toolCall fantasy.ToolCallContent) fantasy.ToolCallPart {
	return fantasy.ToolCallPart{
		ToolCallID:       toolCall.ToolCallID,
		ToolName:         toolCall.ToolName,
		Input:            toolCall.Input,
		ProviderExecuted: toolCall.ProviderExecuted,
		ProviderOptions:  fantasy.ProviderOptions(toolCall.ProviderMetadata),
	}
}

func toolResultContentToPart(toolResult fantasy.ToolResultContent) fantasy.ToolResultPart {
	return fantasy.ToolResultPart{
		ToolCallID:       toolResult.ToolCallID,
		Output:           toolResult.Result,
		ProviderExecuted: toolResult.ProviderExecuted,
		ProviderOptions:  fantasy.ProviderOptions(toolResult.ProviderMetadata),
	}
}

func TestStreamSilenceGuard_DisarmAndFireRace(t *testing.T) {
	t.Parallel()

	for range 128 {
		var cancels atomic.Int32
		guard := newStreamSilenceGuard(quartz.NewReal(), time.Hour, func(err error) {
			if errors.Is(err, errStreamSilenceTimeout) {
				cancels.Add(1)
			}
		})

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			<-start
			guard.onTimeout()
		}()

		go func() {
			defer wg.Done()
			<-start
			guard.Disarm()
		}()

		close(start)
		wg.Wait()

		guard.onTimeout()
		guard.Disarm()

		require.LessOrEqual(t, cancels.Load(), int32(1))
	}
}

func TestStreamSilenceGuard_DisarmPreservesPermanentError(t *testing.T) {
	t.Parallel()

	attemptCtx, cancelAttempt := context.WithCancelCause(context.Background())
	defer cancelAttempt(nil)

	guard := newStreamSilenceGuard(quartz.NewReal(), time.Hour, cancelAttempt)
	guard.Disarm()
	guard.onTimeout()

	classified := chaterror.Classify(classifyStreamSilenceTimeout(
		attemptCtx,
		"openai",
		xerrors.New("invalid model"),
	))
	require.Equal(t, codersdk.ChatErrorKindConfig, classified.Kind)
	require.False(t, classified.Retryable)
	require.Nil(t, context.Cause(attemptCtx))
}

func TestGenerateAssistant_ProviderContextSurvivesStreamError(t *testing.T) {
	t.Parallel()

	model := &chattest.FakeModel{
		ProviderName: "openai",
		ModelName:    "test-model",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return nil, xerrors.New("upstream returned status 400")
		},
	}

	_, err := GenerateAssistant(context.Background(), GenerateAssistantOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "hello"),
		},
	})
	require.Error(t, err)
	classified := chaterror.Classify(err)
	require.Equal(t, "openai", classified.Provider)
	require.Equal(t, "OpenAI returned an unexpected error.", classified.Message)
}

func TestGenerateAssistant_ErrorProviderOverridesTransportLabel(t *testing.T) {
	t.Parallel()

	// Bedrock routed through aibridge uses the Anthropic transport, so
	// Model.Provider() reports "anthropic". The user-facing error must use
	// the configured provider from ErrorProvider instead.
	model := &chattest.FakeModel{
		ProviderName: "anthropic",
		ModelName:    "qwen3-coder-next",
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return nil, xerrors.New("upstream returned status 400")
		},
	}

	_, err := GenerateAssistant(context.Background(), GenerateAssistantOptions{
		Model:         model,
		ErrorProvider: "bedrock",
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "hello"),
		},
	})
	require.Error(t, err)
	classified := chaterror.Classify(err)
	require.Equal(t, "bedrock", classified.Provider)
	require.Equal(t, "AWS Bedrock returned an unexpected error.", classified.Message)
}

func TestGenerateAssistant_HTTP2TransportErrorClassifiedAsRetryableTimeout(t *testing.T) {
	t.Parallel()

	for _, provider := range []string{"anthropic", "openai"} {
		provider := provider
		t.Run(provider, func(t *testing.T) {
			t.Parallel()

			model := &chattest.FakeModel{
				ProviderName: provider,
				ModelName:    "test-model",
				StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
					return nil, xerrors.New("http2: client connection force closed via ClientConn.Close")
				},
			}

			_, err := GenerateAssistant(context.Background(), GenerateAssistantOptions{
				Model: model,
			})
			require.Error(t, err)
			classified := chaterror.Classify(err)
			require.Equal(t, codersdk.ChatErrorKindTimeout, classified.Kind)
			require.Equal(t, provider, classified.Provider)
			require.True(t, classified.Retryable)
		})
	}
}

func TestGenerateAssistant_StreamSilenceTimeoutRetryClassification(t *testing.T) {
	t.Parallel()

	t.Run("timeout while opening stream", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		const silenceTimeout = 5 * time.Millisecond
		clock := quartz.NewMock(t)
		trap := clock.Trap().AfterFunc(streamSilenceGuardTimerTag)
		defer trap.Close()
		var calls atomic.Int32
		model := &chattest.FakeModel{
			ProviderName: "openai",
			ModelName:    "test-model",
			StreamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				if calls.Add(1) == 1 {
					<-ctx.Done()
					return nil, ctx.Err()
				}
				return streamFromParts([]fantasy.StreamPart{{
					Type:         fantasy.StreamPartTypeFinish,
					FinishReason: fantasy.FinishReasonStop,
				}}), nil
			},
		}
		done := make(chan error, 1)
		go func() {
			_, err := GenerateAssistant(context.Background(), GenerateAssistantOptions{
				Model:                model,
				Clock:                clock,
				StreamSilenceTimeout: silenceTimeout,
			})
			done <- err
		}()

		trap.MustWait(ctx).MustRelease(ctx)
		_, waiter := clock.AdvanceNext()
		waiter.MustWait(ctx)
		require.Error(t, <-done)
		require.Equal(t, int32(1), calls.Load())
	})

	t.Run("timeout before first part", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		const silenceTimeout = 5 * time.Millisecond
		clock := quartz.NewMock(t)
		trap := clock.Trap().AfterFunc(streamSilenceGuardTimerTag)
		defer trap.Close()
		var calls atomic.Int32
		model := &chattest.FakeModel{
			ProviderName: "openai",
			ModelName:    "test-model",
			StreamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				calls.Add(1)
				return func(yield func(fantasy.StreamPart) bool) {
					<-ctx.Done()
					yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeError, Error: ctx.Err()})
				}, nil
			},
		}
		done := make(chan error, 1)
		go func() {
			_, err := GenerateAssistant(context.Background(), GenerateAssistantOptions{
				Model:                model,
				Clock:                clock,
				StreamSilenceTimeout: silenceTimeout,
			})
			done <- err
		}()

		trap.MustWait(ctx).MustRelease(ctx)
		_, waiter := clock.AdvanceNext()
		waiter.MustWait(ctx)
		err := <-done
		require.Error(t, err)
		classified := chaterror.Classify(err)
		require.Equal(t, codersdk.ChatErrorKindStreamSilenceTimeout, classified.Kind)
		require.Equal(t, "openai", classified.Provider)
		require.True(t, classified.Retryable)
		require.Equal(t, int32(1), calls.Load())
	})

	t.Run("first part disarms timeout", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		const silenceTimeout = 5 * time.Millisecond
		clock := quartz.NewMock(t)
		trap := clock.Trap().AfterFunc(streamSilenceGuardTimerTag)
		defer trap.Close()
		var calls atomic.Int32
		continueStream := make(chan struct{})
		model := &chattest.FakeModel{
			ProviderName: "openai",
			ModelName:    "test-model",
			StreamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				calls.Add(1)
				return func(yield func(fantasy.StreamPart) bool) {
					if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"}) {
						return
					}
					select {
					case <-continueStream:
					case <-ctx.Done():
						yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeError, Error: ctx.Err()})
						return
					}
					yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "done"})
					yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"})
					yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop})
				}, nil
			},
		}
		done := make(chan error, 1)
		go func() {
			_, err := GenerateAssistant(context.Background(), GenerateAssistantOptions{
				Model:                model,
				Clock:                clock,
				StreamSilenceTimeout: silenceTimeout,
			})
			done <- err
		}()

		trap.MustWait(ctx).MustRelease(ctx)
		close(continueStream)
		require.NoError(t, <-done)
		require.Equal(t, int32(1), calls.Load())
	})

	t.Run("silent stream close after timeout", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		const silenceTimeout = 5 * time.Millisecond
		clock := quartz.NewMock(t)
		trap := clock.Trap().AfterFunc(streamSilenceGuardTimerTag)
		defer trap.Close()
		var calls atomic.Int32
		model := &chattest.FakeModel{
			ProviderName: "openai",
			ModelName:    "test-model",
			StreamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				calls.Add(1)
				return func(func(fantasy.StreamPart) bool) {
					<-ctx.Done()
				}, nil
			},
		}
		done := make(chan error, 1)
		go func() {
			_, err := GenerateAssistant(context.Background(), GenerateAssistantOptions{
				Model:                model,
				Clock:                clock,
				StreamSilenceTimeout: silenceTimeout,
			})
			done <- err
		}()

		trap.MustWait(ctx).MustRelease(ctx)
		_, waiter := clock.AdvanceNext()
		waiter.MustWait(ctx)
		err := <-done
		require.Error(t, err)
		classified := chaterror.Classify(err)
		require.Equal(t, codersdk.ChatErrorKindStreamSilenceTimeout, classified.Kind)
		require.Equal(t, int32(1), calls.Load())
	})
}

func TestGenerateAssistant_PanicInPublishMessagePartReleasesAttempt(t *testing.T) {
	t.Parallel()

	attemptReleased := make(chan struct{})
	model := &chattest.FakeModel{
		ProviderName: "openai",
		ModelName:    "test-model",
		StreamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			go func() {
				<-ctx.Done()
				close(attemptReleased)
			}()
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "boom"},
			}), nil
		},
	}

	defer func() {
		r := recover()
		require.NotNil(t, r)
		select {
		case <-attemptReleased:
		case <-time.After(time.Second):
			t.Fatal("attempt context was not released after panic")
		}
	}()

	_, _ = GenerateAssistant(context.Background(), GenerateAssistantOptions{
		Model: model,
		PublishMessagePart: func(codersdk.ChatMessageRole, codersdk.ChatMessagePart) {
			panic("publish panic")
		},
	})

	t.Fatal("expected GenerateAssistant to panic")
}

func requireToolResultErrorMessage(
	t *testing.T,
	result fantasy.ToolResultContent,
	expected string,
) {
	t.Helper()

	output, ok := result.Result.(fantasy.ToolResultOutputContentError)
	require.Truef(t, ok, "expected error tool result, got %T", result.Result)
	require.Error(t, output.Error)
	require.Equal(t, expected, output.Error.Error())
}

func streamFromParts(parts []fantasy.StreamPart) fantasy.StreamResponse {
	return iter.Seq[fantasy.StreamPart](func(yield func(fantasy.StreamPart) bool) {
		for _, part := range parts {
			if !yield(part) {
				return
			}
		}
	})
}

func textMessage(role fantasy.MessageRole, text string) fantasy.Message {
	return fantasy.Message{
		Role: role,
		Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: text},
		},
	}
}

func requireTextContent(t *testing.T, content []fantasy.Content, text string) fantasy.TextContent {
	t.Helper()

	for _, block := range content {
		textContent, ok := fantasy.AsContentType[fantasy.TextContent](block)
		if ok && textContent.Text == text {
			return textContent
		}
	}
	t.Fatalf("missing text content %q", text)
	return fantasy.TextContent{}
}

func TestExclusiveToolPolicy_MixedBatchErrors(t *testing.T) {
	t.Parallel()

	results, violated := applyExclusiveToolPolicy(
		[]fantasy.ToolCallContent{
			{ToolCallID: "advisor-1", ToolName: "advisor", Input: `{}`},
			{ToolCallID: "read-1", ToolName: "read_file", Input: `{"path":"main.go"}`},
		},
		map[string]bool{"advisor": true},
		NopMetrics(),
		"fake",
		"",
	)

	require.True(t, violated)
	require.Len(t, results, 2)
	require.Equal(t, "advisor-1", results[0].ToolCallID)
	require.Equal(t, "read-1", results[1].ToolCallID)
	requireToolResultErrorMessage(
		t,
		results[0],
		"advisor must be called alone, without other tools in the same batch. Retry with only the advisor call.",
	)
	requireToolResultErrorMessage(
		t,
		results[1],
		"this tool was skipped because advisor must run alone in its batch. Retry your tool calls without advisor, or call advisor separately first.",
	)
}

func TestApplyExclusiveToolPolicy_RecordsErrorMetrics(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewPedanticRegistry()
	m := NewMetrics(reg)

	_, violated := applyExclusiveToolPolicy(
		[]fantasy.ToolCallContent{
			{ToolCallID: "advisor-1", ToolName: "advisor", Input: `{}`},
			{ToolCallID: "read-1", ToolName: "read_file", Input: `{"path":"main.go"}`},
		},
		map[string]bool{"advisor": true},
		m,
		"fake",
		"claude-test",
	)
	require.True(t, violated)

	require.Equal(t, 1.0, promtestutil.ToFloat64(
		m.ToolErrorsTotal.WithLabelValues("fake", "claude-test", "advisor"),
	))
	require.Equal(t, 1.0, promtestutil.ToFloat64(
		m.ToolErrorsTotal.WithLabelValues("fake", "claude-test", "read_file"),
	))
}

func TestExclusiveToolPolicy_MultipleExclusive(t *testing.T) {
	t.Parallel()

	results, violated := applyExclusiveToolPolicy(
		[]fantasy.ToolCallContent{
			{ToolCallID: "advisor-1", ToolName: "advisor", Input: `{}`},
			{ToolCallID: "advisor-2", ToolName: "advisor", Input: `{"mode":"second-opinion"}`},
		},
		map[string]bool{"advisor": true},
		NopMetrics(),
		"fake",
		"",
	)

	require.True(t, violated)
	require.Len(t, results, 2)
	requireToolResultErrorMessage(
		t,
		results[0],
		"advisor must be called alone, without other tools in the same batch. Retry with only the advisor call.",
	)
	requireToolResultErrorMessage(
		t,
		results[1],
		"advisor must be called alone, without other tools in the same batch. Retry with only the advisor call.",
	)
}

func TestSanitizeAnthropicProviderToolContent(t *testing.T) {
	t.Parallel()

	providerCall := func(id, name, input string) fantasy.ToolCallContent {
		return fantasy.ToolCallContent{
			ToolCallID:       id,
			ToolName:         name,
			Input:            input,
			ProviderExecuted: true,
		}
	}
	providerResult := func(id, name string) fantasy.ToolResultContent {
		return fantasy.ToolResultContent{
			ToolCallID:       id,
			ToolName:         name,
			ProviderExecuted: true,
			ProviderMetadata: validWebSearchProviderMetadataForTest(),
			Result:           fantasy.ToolResultOutputContentText{Text: "ok"},
		}
	}
	localCall := func(id, name string) fantasy.ToolCallContent {
		return fantasy.ToolCallContent{
			ToolCallID: id,
			ToolName:   name,
			Input:      `{}`,
		}
	}
	localResult := func(id, name string) fantasy.ToolResultContent {
		return fantasy.ToolResultContent{
			ToolCallID: id,
			ToolName:   name,
			Result:     fantasy.ToolResultOutputContentText{Text: "ok"},
		}
	}
	type contentSummary struct {
		providerCalls   []string
		providerResults []string
		localCalls      []string
		localResults    []string
	}
	summarizeContent := func(content []fantasy.Content) contentSummary {
		var summary contentSummary
		for _, block := range content {
			if toolCall, ok := safeToolCallContent(block); ok {
				if toolCall.ProviderExecuted {
					summary.providerCalls = append(summary.providerCalls, toolCall.ToolCallID)
				} else {
					summary.localCalls = append(summary.localCalls, toolCall.ToolCallID)
				}
				continue
			}
			if toolResult, ok := safeToolResultContent(block); ok {
				if toolResult.ProviderExecuted {
					summary.providerResults = append(summary.providerResults, toolResult.ToolCallID)
				} else {
					summary.localResults = append(summary.localResults, toolResult.ToolCallID)
				}
			}
		}
		return summary
	}
	assertProviderHistoryValid := func(t *testing.T, content []fantasy.Content) {
		t.Helper()

		parts := make([]fantasy.MessagePart, 0)
		for _, block := range content {
			if toolCall, ok := safeToolCallContent(block); ok && toolCall.ProviderExecuted {
				parts = append(parts, toolCallContentToPart(toolCall))
				continue
			}
			if toolResult, ok := safeToolResultContent(block); ok && toolResult.ProviderExecuted {
				parts = append(parts, toolResultContentToPart(toolResult))
			}
		}
		if len(parts) == 0 {
			return
		}
		require.Empty(t, chatsanitize.ValidateAnthropicProviderToolHistory([]fantasy.Message{
			{
				Role:    fantasy.MessageRoleAssistant,
				Content: parts,
			},
		}))
	}

	metadataCall := providerCall("ws-meta", "web_search", `{"query":"coder"}`)
	metadataCall.ProviderMetadata = fantasy.ProviderMetadata{fantasyanthropic.Name: nil}
	metadataResult := providerResult("ws-meta", "web_search")
	metadataResult.ProviderMetadata = fantasy.ProviderMetadata{fantasyanthropic.Name: nil}
	pointerCall := providerCall("ws-pointer", "web_search", `{"query":"coder"}`)
	var nilToolCall *fantasy.ToolCallContent

	testCases := []struct {
		name               string
		provider           string
		content            []fantasy.Content
		wantSummary        contentSummary
		wantRemovedCalls   int
		wantRemovedResults int
		wantTexts          []string
		validateAnthropic  bool
	}{
		{
			name:     "orphan provider result textified",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				fantasy.TextContent{Text: "keep"},
				providerResult("ws-1", "web_search"),
			},
			wantRemovedResults: 1,
			wantTexts:          []string{"keep", "ok"},
			validateAnthropic:  true,
		},
		{
			name:     "result before call removes both provider blocks",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				providerResult("ws-1", "web_search"),
				providerCall("ws-1", "web_search", `{"query":"coder"}`),
			},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
			wantTexts:          []string{"ok"},
			validateAnthropic:  true,
		},
		{
			name:     "valid web search pair preserved",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				providerCall("ws-1", "web_search", `{"query":"coder"}`),
				providerResult("ws-1", "web_search"),
				fantasy.TextContent{Text: "search done"},
			},
			wantSummary: contentSummary{
				providerCalls:   []string{"ws-1"},
				providerResults: []string{"ws-1"},
			},
			wantTexts:         []string{"search done"},
			validateAnthropic: true,
		},
		{
			name:     "invalid JSON provider call drops pair",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				providerCall("ws-1", "web_search", `{`),
				providerResult("ws-1", "web_search"),
			},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
			wantTexts:          []string{"ok"},
			validateAnthropic:  true,
		},
		{
			name:     "empty ID provider call drops pair",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				providerCall("", "web_search", `{"query":"coder"}`),
				providerResult("", "web_search"),
			},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
			wantTexts:          []string{"ok"},
			validateAnthropic:  true,
		},
		{
			name:     "empty tool name provider call drops pair",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				providerCall("ws-empty", "", `{"query":"coder"}`),
				providerResult("ws-empty", ""),
			},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
			wantTexts:          []string{"ok"},
			validateAnthropic:  true,
		},
		{
			name:     "non web search provider pair drops through serializable helper",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				providerCall("code-1", "code_execution", `{"code":"print(1)"}`),
				providerResult("code-1", "code_execution"),
			},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
			wantTexts:          []string{"ok"},
			validateAnthropic:  true,
		},
		{
			name:     "mismatched provider result tool name drops pair",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				providerCall("ws-mismatch", "web_search", `{"query":"coder"}`),
				providerResult("ws-mismatch", "code_execution"),
			},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
			wantTexts:          []string{"ok"},
			validateAnthropic:  true,
		},
		{
			name:     "duplicate provider IDs drop all provider content for ID",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				providerCall("dup-1", "web_search", `{"query":"coder"}`),
				providerResult("dup-1", "web_search"),
				providerCall("dup-1", "web_search", `{"query":"coder"}`),
			},
			wantRemovedCalls:   2,
			wantRemovedResults: 1,
			wantTexts:          []string{"ok"},
			validateAnthropic:  true,
		},
		{
			name:     "mismatched provider flags remove only provider side",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				providerCall("mix-1", "web_search", `{"query":"coder"}`),
				localResult("mix-1", "web_search"),
				localCall("mix-2", "read_file"),
				providerResult("mix-2", "web_search"),
			},
			wantSummary: contentSummary{
				localCalls:   []string{"mix-2"},
				localResults: []string{"mix-1"},
			},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
			wantTexts:          []string{"ok"},
			validateAnthropic:  true,
		},
		{
			name:     "malformed provider metadata textifies result",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				metadataCall,
				metadataResult,
			},
			wantRemovedCalls:   1,
			wantRemovedResults: 1,
			wantTexts:          []string{"ok"},
			validateAnthropic:  true,
		},
		{
			name:     "pointer and nil pointer variants are handled safely",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				nilToolCall,
				&pointerCall,
				providerResult("ws-pointer", "web_search"),
			},
			wantSummary: contentSummary{
				providerCalls:   []string{"ws-pointer"},
				providerResults: []string{"ws-pointer"},
			},
			validateAnthropic: true,
		},
		{
			name:     "local tool content is unchanged",
			provider: fantasyanthropic.Name,
			content: []fantasy.Content{
				localCall("tc-1", "read_file"),
				localResult("tc-1", "read_file"),
			},
			wantSummary: contentSummary{
				localCalls:   []string{"tc-1"},
				localResults: []string{"tc-1"},
			},
			validateAnthropic: true,
		},
		{
			name:     "non Anthropic provider content is unchanged",
			provider: "fake",
			content: []fantasy.Content{
				providerCall("ws-1", "web_search", `{"query":"coder"}`),
			},
			wantSummary: contentSummary{
				providerCalls: []string{"ws-1"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sanitized, stats := chatsanitize.SanitizeAnthropicProviderToolContent(tc.provider, tc.content)
			require.Equal(t, tc.wantRemovedCalls, stats.RemovedToolCalls)
			require.Equal(t, tc.wantRemovedResults, stats.RemovedToolResults)
			require.Zero(t, stats.DroppedMessages)

			summary := summarizeContent(sanitized)
			assert.ElementsMatch(t, tc.wantSummary.providerCalls, summary.providerCalls)
			assert.ElementsMatch(t, tc.wantSummary.providerResults, summary.providerResults)
			assert.ElementsMatch(t, tc.wantSummary.localCalls, summary.localCalls)
			assert.ElementsMatch(t, tc.wantSummary.localResults, summary.localResults)
			for _, text := range tc.wantTexts {
				requireTextContent(t, sanitized, text)
			}
			if tc.validateAnthropic {
				assertProviderHistoryValid(t, sanitized)
			}
		})
	}
}

type serialMarkerTool struct{ fantasy.AgentTool }

func (serialMarkerTool) SerialToolCalls() bool { return true }

type observerMarkerTool struct {
	fantasy.AgentTool
	observed func(names []string)
}

func (t observerMarkerTool) ObserveStepToolCalls(names []string) { t.observed(names) }

func TestExecuteToolsNotifiesStepToolCallObservers(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var observedNames []string
	observedBeforeRun := false
	observer := observerMarkerTool{
		AgentTool: fantasy.NewAgentTool(
			"observer_tool",
			"records sibling calls",
			func(_ context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				mu.Lock()
				observedBeforeRun = observedNames != nil
				mu.Unlock()
				return fantasy.NewTextResponse("ok"), nil
			},
		),
		observed: func(names []string) {
			mu.Lock()
			defer mu.Unlock()
			observedNames = append([]string{}, names...)
		},
	}
	var uncalledObserved atomic.Bool
	uncalledObserver := observerMarkerTool{
		AgentTool: fantasy.NewAgentTool(
			"uncalled_observer",
			"never called this step",
			func(_ context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return fantasy.NewTextResponse("ok"), nil
			},
		),
		observed: func([]string) { uncalledObserved.Store(true) },
	}
	other := fantasy.NewAgentTool(
		"other_tool",
		"plain tool",
		func(_ context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.NewTextResponse("ok"), nil
		},
	)

	executeTools(
		context.Background(),
		quartz.NewReal(),
		[]fantasy.AgentTool{observer, uncalledObserver, other},
		nil,
		nil,
		nil,
		[]fantasy.ToolCallContent{
			{ToolCallID: "1", ToolName: "observer_alias", Input: "{}"},
			{ToolCallID: "2", ToolName: "other_tool", Input: "{}"},
		},
		[]fantasy.ToolCallContent{
			{ToolCallID: "1", ToolName: "observer_alias", Input: "{}"},
			{ToolCallID: "2", ToolName: "other_tool", Input: "{}"},
			{ToolCallID: "3", ToolName: "denied_tool", Input: "{}"},
		},
		NewMetrics(prometheus.NewRegistry()),
		slog.Make(),
		"fake", "fake-model",
		map[string]bool{},
		defaultToolResultBytes,
		map[string]string{"observer_alias": "observer_tool"},
		time.Time{},
		nil,
	)

	require.Equal(t, []string{"observer_tool", "other_tool", "denied_tool"}, observedNames,
		"a called observer sees every observed tool-call name, including calls denied before execution")
	require.True(t, observedBeforeRun, "observers are notified before any tool call executes")
	require.False(t, uncalledObserved.Load(), "tools not called this step are not notified")
}

type resultObserverMarkerTool struct {
	fantasy.AgentTool
	observedResults func(names []string, errored []bool)
}

func (t resultObserverMarkerTool) ObserveStepToolResults(names []string, errored []bool) {
	t.observedResults(names, errored)
}

func TestExecuteToolsNotifiesStepToolResultObservers(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var gotNames []string
	var gotErrored []bool
	notifications := 0
	observer := resultObserverMarkerTool{
		AgentTool: fantasy.NewAgentTool(
			"observer_tool",
			"records sibling outcomes",
			func(_ context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return fantasy.NewTextResponse("ok"), nil
			},
		),
		observedResults: func(names []string, errored []bool) {
			mu.Lock()
			defer mu.Unlock()
			notifications++
			gotNames = append([]string{}, names...)
			gotErrored = append([]bool{}, errored...)
		},
	}
	failing := fantasy.NewAgentTool(
		"failing_tool",
		"returns an error result",
		func(_ context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.NewTextErrorResponse("remote error"), nil
		},
	)

	executed := []fantasy.ToolCallContent{
		{ToolCallID: "1", ToolName: "observer_alias", Input: "{}"},
		{ToolCallID: "2", ToolName: "failing_tool", Input: "{}"},
		{ToolCallID: "3", ToolName: "missing_tool", Input: "{}"},
	}
	executeTools(
		context.Background(),
		quartz.NewReal(),
		[]fantasy.AgentTool{observer, failing},
		nil,
		nil,
		nil,
		executed,
		append(slices.Clone(executed), fantasy.ToolCallContent{
			ToolCallID: "4", ToolName: "rejected_tool", Input: "{not json",
		}),
		NewMetrics(prometheus.NewRegistry()),
		slog.Make(),
		"fake", "fake-model",
		map[string]bool{},
		defaultToolResultBytes,
		map[string]string{"observer_alias": "observer_tool"},
		time.Time{},
		nil,
	)

	require.Equal(t, 1, notifications, "each called observer is notified once per step")
	require.Equal(t, []string{"observer_tool", "failing_tool", "missing_tool", "rejected_tool"}, gotNames,
		"outcomes are reported per call in observed order with aliases resolved")
	require.Equal(t, []bool{false, true, true, true}, gotErrored,
		"error results, unresolvable tools, and observed calls rejected before execution all settle as errored outcomes")
}

type serialResultObserverTool struct {
	fantasy.AgentTool
	observedResults func(names []string, errored []bool)
}

func (serialResultObserverTool) SerialToolCalls() bool { return true }

func (t serialResultObserverTool) ObserveStepToolResults(names []string, errored []bool) {
	t.observedResults(names, errored)
}

func TestExecuteToolsReconcilesResultsBeforeSerialCalls(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var erroredAtNotify []string
	var erroredAtRun []string
	notified := false
	serial := serialResultObserverTool{
		AgentTool: fantasy.NewAgentTool(
			"serial_observer",
			"observes sibling outcomes before running",
			func(_ context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				mu.Lock()
				erroredAtRun = append([]string{}, erroredAtNotify...)
				mu.Unlock()
				return fantasy.NewTextResponse("ok"), nil
			},
		),
		observedResults: func(names []string, errored []bool) {
			mu.Lock()
			defer mu.Unlock()
			notified = true
			erroredAtNotify = nil
			for i, name := range names {
				if errored[i] {
					erroredAtNotify = append(erroredAtNotify, name)
				}
			}
		},
	}
	failing := fantasy.NewAgentTool(
		"failing_tool",
		"returns an error result",
		func(_ context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.NewTextErrorResponse("remote error"), nil
		},
	)

	results := executeTools(
		context.Background(),
		quartz.NewReal(),
		[]fantasy.AgentTool{serial, failing},
		nil,
		nil,
		nil,
		[]fantasy.ToolCallContent{
			{ToolCallID: "1", ToolName: "serial_observer", Input: "{}"},
			{ToolCallID: "2", ToolName: "failing_tool", Input: "{}"},
		},
		nil,
		NewMetrics(prometheus.NewRegistry()),
		slog.Make(),
		"fake", "fake-model",
		map[string]bool{},
		defaultToolResultBytes,
		nil,
		time.Time{},
		nil,
	)

	require.True(t, notified)
	require.Equal(t, []string{"failing_tool"}, erroredAtRun,
		"a serial tool must see settled sibling outcomes before it executes")
	require.Len(t, results, 2)
	require.Equal(t, "1", results[0].content.ToolCallID, "results keep original call order")
}

func TestExecuteToolsSerialToolCallOrder(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var events []string
	inFlight := 0
	maxInFlight := 0
	record := func(event string, delta int) {
		mu.Lock()
		defer mu.Unlock()
		inFlight += delta
		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}
		events = append(events, event)
	}
	serial := serialMarkerTool{AgentTool: fantasy.NewAgentTool(
		"serial_tool",
		"records call order",
		func(_ context.Context, _ struct{}, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			record(call.ID+":start", 1)
			runtime.Gosched()
			record(call.ID+":end", -1)
			return fantasy.NewTextResponse("ok"), nil
		},
	)}
	parallelRan := make(chan struct{})
	parallel := fantasy.NewAgentTool(
		"parallel_tool",
		"plain tool",
		func(_ context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			close(parallelRan)
			return fantasy.NewTextResponse("ok"), nil
		},
	)

	calls := []fantasy.ToolCallContent{
		{ToolCallID: "a", ToolName: "serial_tool", Input: "{}"},
		{ToolCallID: "p", ToolName: "parallel_tool", Input: "{}"},
		{ToolCallID: "b", ToolName: "serial_tool", Input: "{}"},
		{ToolCallID: "c", ToolName: "serial_tool", Input: "{}"},
	}
	results := executeTools(
		context.Background(),
		quartz.NewReal(),
		[]fantasy.AgentTool{serial, parallel},
		nil,
		nil,
		nil,
		calls,
		nil,
		NewMetrics(prometheus.NewRegistry()),
		slog.Make(),
		"fake", "fake-model",
		map[string]bool{},
		defaultToolResultBytes,
		nil,
		time.Time{},
		nil,
	)

	require.Equal(t, []string{"a:start", "a:end", "b:start", "b:end", "c:start", "c:end"}, events,
		"serial tool calls must run one at a time in tool-call order")
	require.Equal(t, 1, maxInFlight)
	select {
	case <-parallelRan:
	default:
		t.Fatal("parallel tool call did not run")
	}
	require.Len(t, results, len(calls))
	for i, tc := range calls {
		require.Equal(t, tc.ToolCallID, results[i].content.ToolCallID, "results keep original call order")
	}
}

type timingEvent struct {
	dispatchIndex int
	at            time.Time
}

// liveToolBillingRecorder is a test ToolBillingRecorder that publishes
// start and completion timestamps as they happen.
type liveToolBillingRecorder struct {
	started   chan timingEvent
	completed chan timingEvent
}

func (r liveToolBillingRecorder) RecordStart(dispatchIndex int, startedAt time.Time) {
	r.started <- timingEvent{dispatchIndex: dispatchIndex, at: startedAt}
}

func (r liveToolBillingRecorder) RecordComplete(dispatchIndex int, completedAt time.Time) {
	r.completed <- timingEvent{dispatchIndex: dispatchIndex, at: completedAt}
}

func TestExecuteToolsReturnsExecutionIntervals(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	started := make(chan timingEvent, 3)
	completed := make(chan timingEvent, 3)

	slowGo := make(chan struct{})
	fastGo := make(chan struct{})
	blocking := func(name string, release <-chan struct{}) fantasy.AgentTool {
		return fantasy.NewAgentTool(
			name,
			"waits for the test to release it",
			func(_ context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				<-release
				return fantasy.NewTextResponse("ok"), nil
			},
		)
	}
	slow := blocking("slow_tool", slowGo)
	fast := blocking("fast_tool", fastGo)
	serial := serialMarkerTool{AgentTool: fantasy.NewAgentTool(
		"serial_tool",
		"runs after concurrent tools",
		func(_ context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.NewTextResponse("ok"), nil
		},
	)}
	calls := []fantasy.ToolCallContent{
		{ToolCallID: "slow", ToolName: "slow_tool", Input: "{}"},
		{ToolCallID: "fast", ToolName: "fast_tool", Input: "{}"},
		{ToolCallID: "serial", ToolName: "serial_tool", Input: "{}"},
	}
	batchStart := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	resultsCh := make(chan []toolExecutionResult, 1)
	go func() {
		resultsCh <- executeTools(
			ctx,
			quartz.NewReal(),
			[]fantasy.AgentTool{slow, fast, serial},
			nil,
			nil,
			nil,
			calls,
			nil,
			NewMetrics(prometheus.NewRegistry()),
			slog.Make(),
			"fake", "fake-model",
			map[string]bool{},
			defaultToolResultBytes,
			nil,
			batchStart,
			liveToolBillingRecorder{started: started, completed: completed},
		)
	}()

	startByIndex := make([]time.Time, len(calls))
	for range 2 {
		event := testutil.RequireReceive(ctx, t, started)
		startByIndex[event.dispatchIndex] = event.at
	}

	close(fastGo)
	fastCompleted := testutil.RequireReceive(ctx, t, completed)
	require.Equal(t, 1, fastCompleted.dispatchIndex)

	close(slowGo)
	slowCompleted := testutil.RequireReceive(ctx, t, completed)
	require.Equal(t, 0, slowCompleted.dispatchIndex)

	serialStarted := testutil.RequireReceive(ctx, t, started)
	require.Equal(t, 2, serialStarted.dispatchIndex)
	startByIndex[serialStarted.dispatchIndex] = serialStarted.at
	serialCompleted := testutil.RequireReceive(ctx, t, completed)
	require.Equal(t, 2, serialCompleted.dispatchIndex)

	endByIndex := []time.Time{slowCompleted.at, fastCompleted.at, serialCompleted.at}
	results := testutil.RequireReceive(ctx, t, resultsCh)
	require.Len(t, results, len(calls))
	for i, call := range calls {
		require.Equal(t, call.ToolCallID, results[i].content.ToolCallID,
			"results keep dispatch order when calls complete out of order")
		require.Equal(t, startByIndex[i], results[i].interval.Start)
		require.Equal(t, endByIndex[i], results[i].interval.End)
	}
	require.Equal(t, batchStart, results[0].interval.Start)
	require.Equal(t, batchStart, results[1].interval.Start)
	require.NotEqual(t, batchStart, results[2].interval.Start)
	require.False(t, results[2].interval.Start.Before(slowCompleted.at),
		"serial execution starts only after concurrent calls settle")
}

func TestExecuteSingleTool_MediaBase64Encoding(t *testing.T) {
	t.Parallel()

	originalBytes := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}
	metrics := NewMetrics(prometheus.NewRegistry())
	logger := slog.Make()

	t.Run("EncodesRawBytesToBase64", func(t *testing.T) {
		t.Parallel()

		tool := fantasy.NewAgentTool(
			"screenshot",
			"takes a screenshot",
			func(_ context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return fantasy.ToolResponse{
					Type:      "image",
					Data:      originalBytes,
					MediaType: "image/jpeg",
				}, nil
			},
		)

		toolMap := map[string]fantasy.AgentTool{
			"screenshot": tool,
		}
		tc := fantasy.ToolCallContent{
			ToolCallID: "call-1",
			ToolName:   "screenshot",
			Input:      "{}",
		}

		result := executeSingleTool(
			context.Background(),
			toolMap,
			tc,
			metrics,
			logger,
			"fake", "fake-model",
			map[string]bool{},
			[]string{"screenshot"},
			nil,
			map[string]struct{}{},
			nil,
			defaultToolResultBytes,
			nil,
		)

		media, ok := result.Result.(fantasy.ToolResultOutputContentMedia)
		require.True(t, ok, "expected ToolResultOutputContentMedia")
		require.Equal(t, "image/jpeg", media.MediaType)

		decoded, err := base64.StdEncoding.DecodeString(media.Data)
		require.NoError(t, err, "Data should be valid base64")
		require.Equal(t, originalBytes, decoded)
	})

	t.Run("SanitizesInvalidUTF8InContent", func(t *testing.T) {
		t.Parallel()

		tool := fantasy.NewAgentTool(
			"screenshot",
			"takes a screenshot",
			func(_ context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return fantasy.ToolResponse{
					Type:      "image",
					Data:      originalBytes,
					MediaType: "image/png",
					Content:   "hello\xffworld",
				}, nil
			},
		)

		toolMap := map[string]fantasy.AgentTool{
			"screenshot": tool,
		}
		tc := fantasy.ToolCallContent{
			ToolCallID: "call-2",
			ToolName:   "screenshot",
			Input:      "{}",
		}

		result := executeSingleTool(
			context.Background(),
			toolMap,
			tc,
			metrics,
			logger,
			"fake", "fake-model",
			map[string]bool{},
			[]string{"screenshot"},
			nil,
			map[string]struct{}{},
			nil,
			defaultToolResultBytes,
			nil,
		)

		media, ok := result.Result.(fantasy.ToolResultOutputContentMedia)
		require.True(t, ok, "expected ToolResultOutputContentMedia")
		require.True(t, utf8.ValidString(media.Text), "Text should be valid UTF-8")
		require.Contains(t, media.Text, "hello")
		require.Contains(t, media.Text, "world")
	})

	t.Run("SanitizesInvalidUTF8InTextResult", func(t *testing.T) {
		t.Parallel()

		tool := fantasy.NewAgentTool(
			"echo",
			"echoes input",
			func(_ context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return fantasy.ToolResponse{
					Content: "hello\xffworld",
				}, nil
			},
		)

		toolMap := map[string]fantasy.AgentTool{
			"echo": tool,
		}
		tc := fantasy.ToolCallContent{
			ToolCallID: "call-3",
			ToolName:   "echo",
			Input:      "{}",
		}

		result := executeSingleTool(
			context.Background(),
			toolMap,
			tc,
			metrics,
			logger,
			"fake", "fake-model",
			map[string]bool{},
			[]string{"echo"},
			nil,
			map[string]struct{}{},
			nil,
			defaultToolResultBytes,
			nil,
		)

		textOutput, ok := result.Result.(fantasy.ToolResultOutputContentText)
		require.True(t, ok, "expected ToolResultOutputContentText, got %T", result.Result)
		require.True(t, utf8.ValidString(textOutput.Text), "Text should be valid UTF-8")
		require.Contains(t, textOutput.Text, "hello")
		require.Contains(t, textOutput.Text, "world")
	})
}

func TestExecuteSingleTool_ResolvesToolNameAlias(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics(prometheus.NewRegistry())
	logger := slog.Make()

	var gotName string
	tool := fantasy.NewAgentTool(
		"interrupt_agent",
		"interrupts an agent",
		func(_ context.Context, _ struct{}, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			gotName = call.Name
			return fantasy.ToolResponse{Content: `{"interrupted":true}`}, nil
		},
	)
	toolMap := map[string]fantasy.AgentTool{"interrupt_agent": tool}

	// The model emits the deprecated name from old history; only the
	// canonical name is advertised/active.
	tc := fantasy.ToolCallContent{
		ToolCallID: "call-alias",
		ToolName:   "close_agent",
		Input:      "{}",
	}

	result := executeSingleTool(
		context.Background(),
		toolMap,
		tc,
		metrics,
		logger,
		"fake", "fake-model",
		map[string]bool{},
		[]string{"interrupt_agent"},
		nil,
		map[string]struct{}{},
		nil,
		defaultToolResultBytes,
		map[string]string{"close_agent": "interrupt_agent"},
	)

	textOutput, ok := result.Result.(fantasy.ToolResultOutputContentText)
	require.True(t, ok, "expected text output, got %T", result.Result)
	require.Contains(t, textOutput.Text, "interrupted")
	// The handler receives the resolved canonical name.
	require.Equal(t, "interrupt_agent", gotName)
	// The persisted result keeps the original alias so existing history
	// renders consistently.
	require.Equal(t, "close_agent", result.ToolName)
}

func TestExecuteSingleTool_UnknownAliasFallsThrough(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics(prometheus.NewRegistry())
	logger := slog.Make()

	tc := fantasy.ToolCallContent{
		ToolCallID: "call-missing",
		ToolName:   "close_agent",
		Input:      "{}",
	}

	// No alias provided: the deprecated name is neither active nor in the
	// tool map, so it surfaces a clear not-active error and the model can
	// self-correct to the advertised name.
	result := executeSingleTool(
		context.Background(),
		map[string]fantasy.AgentTool{},
		tc,
		metrics,
		logger,
		"fake", "fake-model",
		map[string]bool{},
		[]string{"interrupt_agent"},
		nil,
		map[string]struct{}{},
		nil,
		defaultToolResultBytes,
		nil,
	)

	errOutput, ok := result.Result.(fantasy.ToolResultOutputContentError)
	require.True(t, ok, "expected error output, got %T", result.Result)
	require.Contains(t, errOutput.Error.Error(), "close_agent")
}

func TestExecuteSingleTool_AllowsDeferredDirectCall(t *testing.T) {
	t.Parallel()
	tool := fantasy.NewAgentTool(
		"server__direct",
		"direct",
		func(_ context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return fantasy.NewTextResponse("ok"), nil
		},
	)
	result := executeSingleTool(
		context.Background(),
		map[string]fantasy.AgentTool{"server__direct": tool},
		fantasy.ToolCallContent{ToolCallID: "call-direct", ToolName: "server__direct", Input: "{}"},
		NewMetrics(prometheus.NewRegistry()),
		slog.Make(),
		"fake", "fake-model",
		map[string]bool{},
		[]string{"find_tools"},
		map[string]bool{"server__direct": true},
		map[string]struct{}{},
		nil,
		defaultToolResultBytes,
		nil,
	)
	text, ok := result.Result.(fantasy.ToolResultOutputContentText)
	require.True(t, ok)
	require.Equal(t, "ok", text.Text)
}

// fullSchemaTestTool models a tool that reports its complete input
// schema alongside a flattened Info() view.
type fullSchemaTestTool struct {
	fantasy.AgentTool
	schema map[string]any
}

func (fullSchemaTestTool) Info() fantasy.ToolInfo {
	return fantasy.ToolInfo{
		Name:        "full_schema_tool",
		Description: "keeps schemas whole",
		Parameters:  map[string]any{"fallback": map[string]any{"type": "string"}},
	}
}

func (t fullSchemaTestTool) FullInputSchema() map[string]any { return t.schema }

func (fullSchemaTestTool) ProviderOptions() fantasy.ProviderOptions { return nil }

// TestBuildToolDefinitionsFullSchema verifies that a tool exposing a
// full input schema reaches the wire whole, with "required" coerced
// from the []any shape JSON unmarshaling produces to []string so
// provider drivers that assert []string do not drop it.
func TestBuildToolDefinitionsFullSchema(t *testing.T) {
	t.Parallel()

	source := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"filter": map[string]any{"$ref": "#/$defs/Filter"},
		},
		"required":             []any{"filter"},
		"additionalProperties": false,
		"$defs": map[string]any{
			"Filter": map[string]any{"type": "string"},
		},
	}
	defs := buildToolDefinitions([]fantasy.AgentTool{fullSchemaTestTool{schema: source}}, nil, nil)
	require.Len(t, defs, 1)
	ft, ok := defs[0].(fantasy.FunctionTool)
	require.True(t, ok)
	require.Equal(t, []string{"filter"}, ft.InputSchema["required"])
	raw, err := json.Marshal(ft.InputSchema)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"type": "object",
		"properties": {"filter": {"$ref": "#/$defs/Filter"}},
		"required": ["filter"],
		"additionalProperties": false,
		"$defs": {"Filter": {"type": "string"}}
	}`, string(raw))
	// The tool's own schema map is not mutated by the coercion.
	require.Equal(t, []any{"filter"}, source["required"])
}

// TestBuildToolDefinitionsFullSchemaNormalization verifies the
// defensive guards on the full-schema path: a plain object root gets
// "type" defaulted and a non-nil properties object, and an empty
// "required" is dropped so it never serializes to null.
func TestBuildToolDefinitionsFullSchemaNormalization(t *testing.T) {
	t.Parallel()

	source := map[string]any{
		"description": "no explicit type",
		"required":    []any{},
	}
	defs := buildToolDefinitions([]fantasy.AgentTool{fullSchemaTestTool{schema: source}}, nil, nil)
	require.Len(t, defs, 1)
	ft, ok := defs[0].(fantasy.FunctionTool)
	require.True(t, ok)
	raw, err := json.Marshal(ft.InputSchema)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"object","properties":{},"description":"no explicit type"}`, string(raw))
}

// TestBuildToolDefinitionsFullSchemaNilFallsBack verifies that a tool
// whose FullInputSchema returns nil produces the same reconstruction
// from Info() as tools without one.
func TestBuildToolDefinitionsFullSchemaNilFallsBack(t *testing.T) {
	t.Parallel()

	defs := buildToolDefinitions([]fantasy.AgentTool{fullSchemaTestTool{schema: nil}}, nil, nil)
	require.Len(t, defs, 1)
	ft, ok := defs[0].(fantasy.FunctionTool)
	require.True(t, ok)
	raw, err := json.Marshal(ft.InputSchema)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"object","properties":{"fallback":{"type":"string"}}}`, string(raw))
}

// TestBuildToolDefinitionsFullSchemaBannedRootKeywords verifies that
// schema keywords OpenAI rejects at the top level of a tool schema are
// stripped from the root, with an object root enforced, dangling
// "required" entries filtered, and every other key (nested
// combinators, $defs, additionalProperties, description) preserved.
func TestBuildToolDefinitionsFullSchemaBannedRootKeywords(t *testing.T) {
	t.Parallel()

	source := map[string]any{
		"$ref":  "#/$defs/Root",
		"oneOf": []any{map[string]any{"required": []any{"kept"}}},
		"anyOf": []any{map[string]any{"required": []any{"lost"}}},
		"allOf": []any{map[string]any{"properties": map[string]any{"lost": map[string]any{"type": "string"}}}},
		"enum":  []any{"a", "b"},
		"const": "a",
		"not":   map[string]any{"type": "string"},
		"properties": map[string]any{
			"kept": map[string]any{
				"oneOf": []any{
					map[string]any{"$ref": "#/$defs/Kept"},
					map[string]any{"type": "integer"},
				},
			},
		},
		"required":             []any{"kept", "lost"},
		"$defs":                map[string]any{"Kept": map[string]any{"type": "string"}},
		"additionalProperties": false,
		"description":          "root description",
	}
	defs := buildToolDefinitions([]fantasy.AgentTool{fullSchemaTestTool{schema: source}}, nil, nil)
	require.Len(t, defs, 1)
	ft, ok := defs[0].(fantasy.FunctionTool)
	require.True(t, ok)
	raw, err := json.Marshal(ft.InputSchema)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"type": "object",
		"properties": {
			"kept": {"oneOf": [{"$ref": "#/$defs/Kept"}, {"type": "integer"}]}
		},
		"required": ["kept"],
		"$defs": {"Kept": {"type": "string"}},
		"additionalProperties": false,
		"description": "root description"
	}`, string(raw))
	// The tool's own schema map keeps its top-level keys.
	require.Contains(t, source, "oneOf")
	require.Contains(t, source, "$ref")
	require.Equal(t, []any{"kept", "lost"}, source["required"])
}

// TestBuildToolDefinitionsFullSchemaRootTypeArray verifies that a root
// type array, which schema.Normalize rewrites into a root "anyOf",
// still reaches the wire as a plain object root instead of the
// top-level combinator OpenAI rejects.
func TestBuildToolDefinitionsFullSchemaRootTypeArray(t *testing.T) {
	t.Parallel()

	source := map[string]any{
		"type": []any{"object", "null"},
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
		"required": []any{"name"},
	}
	defs := buildToolDefinitions([]fantasy.AgentTool{fullSchemaTestTool{schema: source}}, nil, nil)
	require.Len(t, defs, 1)
	ft, ok := defs[0].(fantasy.FunctionTool)
	require.True(t, ok)
	raw, err := json.Marshal(ft.InputSchema)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"type": "object",
		"properties": {"name": {"type": "string"}},
		"required": ["name"]
	}`, string(raw))
}
