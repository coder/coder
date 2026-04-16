package chatloop //nolint:testpackage // Uses internal symbols.

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/testutil"
)

// extractMessageCache is a test helper that extracts the
// MessageCache from a captured fantasy.Call's ProviderOptions.
// Returns (cache, true) when the key is present and the value
// satisfies the MessageCache interface.
func extractMessageCache(t *testing.T, call fantasy.Call) (MessageCache, bool) {
	t.Helper()
	if call.ProviderOptions == nil {
		return nil, false
	}
	v, ok := call.ProviderOptions[MessageCacheProviderOptionsKey]
	if !ok {
		return nil, false
	}
	mc, ok := v.(MessageCache)
	require.True(t, ok, "cache entry should implement MessageCache")
	return mc, true
}

// streamToolCall returns a StreamResponse that emits a single tool
// call and finishes with FinishReasonToolCalls, causing the step
// loop to continue.
func streamToolCall(id string) fantasy.StreamResponse {
	return streamFromParts([]fantasy.StreamPart{
		{Type: fantasy.StreamPartTypeToolInputStart, ID: id, ToolCallName: "read_file"},
		{Type: fantasy.StreamPartTypeToolInputDelta, ID: id, Delta: `{"path":"f.go"}`},
		{Type: fantasy.StreamPartTypeToolInputEnd, ID: id},
		{
			Type:          fantasy.StreamPartTypeToolCall,
			ID:            id,
			ToolCallName:  "read_file",
			ToolCallInput: `{"path":"f.go"}`,
		},
		{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls},
	})
}

// streamTextStop returns a StreamResponse that emits a short text
// block and finishes with FinishReasonStop, causing the step loop
// to end.
func streamTextStop(text string, usage fantasy.Usage) fantasy.StreamResponse {
	return streamFromParts([]fantasy.StreamPart{
		{Type: fantasy.StreamPartTypeTextStart, ID: "text"},
		{Type: fantasy.StreamPartTypeTextDelta, ID: "text", Delta: text},
		{Type: fantasy.StreamPartTypeTextEnd, ID: "text"},
		{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop, Usage: usage},
	})
}

// callCapture records fantasy.Calls made to a FakeModel's StreamFn.
type callCapture struct {
	mu    sync.Mutex
	calls []fantasy.Call
	count int
}

func (c *callCapture) record(call fantasy.Call) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	step := c.count
	c.count++
	c.calls = append(c.calls, call)
	return step
}

func TestChatLoop_MessageCachePassedToProvider(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	var capture callCapture
	model := &chattest.FakeModel{
		ProviderName: fantasyanthropic.Name,
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			step := capture.record(call)
			if step == 0 {
				return streamToolCall("tc-1"), nil
			}
			return streamTextStop("done", fantasy.Usage{}), nil
		},
	}

	err := Run(ctx, RunOptions{
		Model:                model,
		Messages:             []fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")},
		Tools:                []fantasy.AgentTool{newNoopTool("read_file")},
		MaxSteps:             3,
		ContextLimitFallback: 4096,
		PersistStep:          func(_ context.Context, _ PersistedStep) error { return nil },
	})
	require.NoError(t, err)

	capture.mu.Lock()
	defer capture.mu.Unlock()

	require.Equal(t, 2, capture.count, "expected 2 stream calls (tool-call + stop)")

	// Both calls should carry the same MessageCache instance.
	cache0, ok0 := extractMessageCache(t, capture.calls[0])
	cache1, ok1 := extractMessageCache(t, capture.calls[1])
	require.True(t, ok0, "step 0: cache should be present")
	require.True(t, ok1, "step 1: cache should be present")
	assert.Same(t, cache0, cache1, "same cache instance should be reused across steps")
}

func TestChatLoop_MessageCacheNotSetForNonAnthropic(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	var capturedCall fantasy.Call
	model := &chattest.FakeModel{
		ProviderName: "openai",
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			capturedCall = call
			return streamTextStop("done", fantasy.Usage{}), nil
		},
	}

	err := Run(ctx, RunOptions{
		Model:                model,
		Messages:             []fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")},
		MaxSteps:             1,
		ContextLimitFallback: 4096,
		PersistStep:          func(_ context.Context, _ PersistedStep) error { return nil },
	})
	require.NoError(t, err)

	_, ok := extractMessageCache(t, capturedCall)
	require.False(t, ok, "non-Anthropic provider should not have a MessageCache")
}

func TestChatLoop_MessageCacheClearedOnCompaction(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)
	var capture callCapture
	// sentinelPlanted is set once we inject a marker entry into
	// the cache during step 0. After compaction, Clear() should
	// remove it before step 1 runs.
	var sentinelPlanted atomic.Bool
	model := &chattest.FakeModel{
		ProviderName: fantasyanthropic.Name,
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			step := capture.record(call)
			mc, ok := extractMessageCache(t, call)
			require.True(t, ok, "step %d: cache should be present", step)
			mmc := mc.(*mapMessageCache)

			switch step {
			case 0:
				// Plant a sentinel so we can detect whether
				// Clear() runs before the next step.
				mmc.Set(9999, json.RawMessage(`{"sentinel":true}`))
				sentinelPlanted.Store(true)
				// High usage triggers compaction (80/100 = 80% > 70%).
				return streamTextStop("initial", fantasy.Usage{InputTokens: 80, TotalTokens: 85}), nil
			default:
				// After compaction + Clear(), the sentinel should
				// be gone and the cache should be empty.
				require.True(t, sentinelPlanted.Load(),
					"sentinel should have been planted in step 0")
				_, hasSentinel := mmc.Get(9999)
				assert.False(t, hasSentinel,
					"sentinel entry should have been cleared by compaction")
				assert.Empty(t, mmc.entries,
					"cache should be empty after compaction Clear()")
				return streamTextStop("after compaction", fantasy.Usage{InputTokens: 20, TotalTokens: 25}), nil
			}
		},
		GenerateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
			return &fantasy.Response{
				Content: []fantasy.Content{fantasy.TextContent{Text: "compacted summary"}},
			}, nil
		},
	}

	err := Run(ctx, RunOptions{
		Model:                model,
		Messages:             []fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")},
		MaxSteps:             3,
		ContextLimitFallback: 100,
		PersistStep:          func(_ context.Context, _ PersistedStep) error { return nil },
		Compaction: &CompactionOptions{
			ThresholdPercent: 70,
			SummaryPrompt:    "summarize now",
			Persist:          func(_ context.Context, _ CompactionResult) error { return nil },
		},
		ReloadMessages: func(_ context.Context) ([]fantasy.Message, error) {
			return []fantasy.Message{
				textMessage(fantasy.MessageRoleSystem, "compacted system"),
				textMessage(fantasy.MessageRoleUser, "compacted user"),
			}, nil
		},
	})
	require.NoError(t, err)

	capture.mu.Lock()
	defer capture.mu.Unlock()
	require.GreaterOrEqual(t, capture.count, 2, "expected at least 2 stream calls")
}

// TestChatLoop_MessageCachePopulatedByProvider uses a real fantasy
// Anthropic provider backed by a mock HTTP server to verify that
// the provider's toPrompt actually populates the cache. This test
// fails when the fantasy dependency lacks MessageSerializationCache
// support, since the cache entries will be empty.
func TestChatLoop_MessageCachePopulatedByProvider(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	var requestCount atomic.Int32
	serverURL := chattest.NewAnthropic(t, func(_ *chattest.AnthropicRequest) chattest.AnthropicResponse {
		requestCount.Add(1)
		return chattest.AnthropicStreamingResponse(
			chattest.AnthropicTextChunks("done")...,
		)
	})

	provider, err := fantasyanthropic.New(
		fantasyanthropic.WithAPIKey("test-key"),
		fantasyanthropic.WithBaseURL(serverURL),
	)
	require.NoError(t, err)

	model, err := provider.LanguageModel(ctx, "claude-sonnet-4-20250514")
	require.NoError(t, err)

	err = Run(ctx, RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleSystem, "system prompt"),
			textMessage(fantasy.MessageRoleUser, "hello"),
			textMessage(fantasy.MessageRoleAssistant, "thinking"),
			textMessage(fantasy.MessageRoleUser, "continue"),
		},
		MaxSteps:             1,
		ContextLimitFallback: 4096,
		PersistStep:          func(_ context.Context, _ PersistedStep) error { return nil },
	})
	require.NoError(t, err)
	require.Equal(t, int32(1), requestCount.Load(), "expected 1 API request")

	// Run doesn't expose the cache directly, but we know
	// applyAnthropicCaching is true (provider is "anthropic")
	// so a cache was created. We need to inspect it.
	//
	// Since the cache is local to Run and not returned, we
	// re-run with a wrapper model that captures the Call so we
	// can inspect the cache after the provider has touched it.
	var capturedCache *mapMessageCache
	wrappedModel := &chattest.FakeModel{
		ProviderName: fantasyanthropic.Name,
		StreamFn: func(streamCtx context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			// Grab the cache before the real provider runs.
			v := call.ProviderOptions[MessageCacheProviderOptionsKey]
			//nolint:forcetypeassert // Test-only, panics are fine.
			capturedCache = v.(*mapMessageCache)

			// Delegate to the real provider so toPrompt runs
			// and populates the cache.
			return model.Stream(streamCtx, call)
		},
		ModelName: "claude-sonnet-4-20250514",
	}

	err = Run(ctx, RunOptions{
		Model: wrappedModel,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleSystem, "system prompt"),
			textMessage(fantasy.MessageRoleUser, "hello"),
			textMessage(fantasy.MessageRoleAssistant, "thinking"),
			textMessage(fantasy.MessageRoleUser, "continue"),
		},
		MaxSteps:             1,
		ContextLimitFallback: 4096,
		PersistStep:          func(_ context.Context, _ PersistedStep) error { return nil },
	})
	require.NoError(t, err)

	require.NotNil(t, capturedCache, "cache should have been created")
	require.NotEmpty(t, capturedCache.entries,
		"cache should have been populated by the fantasy Anthropic "+
			"provider's toPrompt — if empty, the fantasy dependency "+
			"likely lacks MessageSerializationCache support")
}

func TestMapMessageCache_BasicOperations(t *testing.T) {
	t.Parallel()

	t.Run("SetAndGet", func(t *testing.T) {
		t.Parallel()
		cache := newMapMessageCache()
		data := json.RawMessage(`{"role":"user","content":"hello"}`)
		cache.Set(0, data)
		got, ok := cache.Get(0)
		require.True(t, ok)
		require.Equal(t, data, got)
	})

	t.Run("GetMissing", func(t *testing.T) {
		t.Parallel()
		cache := newMapMessageCache()
		_, ok := cache.Get(42)
		require.False(t, ok)
	})

	t.Run("ClearEmptiesAll", func(t *testing.T) {
		t.Parallel()
		cache := newMapMessageCache()
		cache.Set(0, json.RawMessage(`{"a":1}`))
		cache.Set(1, json.RawMessage(`{"b":2}`))
		cache.Set(2, json.RawMessage(`{"c":3}`))
		cache.Clear()
		for i := range 3 {
			_, ok := cache.Get(i)
			require.False(t, ok, "index %d should be cleared", i)
		}
	})

	t.Run("SetOverwrites", func(t *testing.T) {
		t.Parallel()
		cache := newMapMessageCache()
		cache.Set(0, json.RawMessage(`{"old":true}`))
		cache.Set(0, json.RawMessage(`{"new":true}`))
		got, ok := cache.Get(0)
		require.True(t, ok)
		require.Equal(t, json.RawMessage(`{"new":true}`), got)
	})
}
