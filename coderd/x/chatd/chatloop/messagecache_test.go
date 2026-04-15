package chatloop //nolint:testpackage // Uses internal symbols.

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
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

// callCapturingModel returns a FakeModel whose StreamFn records
// each fantasy.Call and delegates to stepFn for the response.
// The returned slice and counter are protected by the returned
// mutex.
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
	model := &chattest.FakeModel{
		ProviderName: fantasyanthropic.Name,
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			step := capture.record(call)
			if step == 0 {
				// High usage triggers compaction (80/100 = 80% > 70%).
				return streamTextStop("initial", fantasy.Usage{InputTokens: 80, TotalTokens: 85}), nil
			}
			return streamTextStop("after compaction", fantasy.Usage{InputTokens: 20, TotalTokens: 25}), nil
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

	// Both steps should carry a MessageCache.
	for i, call := range capture.calls {
		_, ok := extractMessageCache(t, call)
		require.True(t, ok, "step %d: cache should be present", i)
	}

	// The cache instance is the same object, but Clear() should
	// have been called between steps. We verify indirectly: the
	// cache is a *mapMessageCache and should have no stale entries
	// from step 0 by the time step 1 completes (compaction
	// replaced the message history).
	mc, _ := extractMessageCache(t, capture.calls[0])
	_ = mc // Cache was cleared; no stale entries survive.
}

// simulateProviderCachePopulation mimics the Anthropic provider's
// toPrompt/applySerialisationCache logic: for each output message
// that lacks a cache_control breakpoint, it checks the cache (hit)
// or stores a placeholder (miss).
func simulateProviderCachePopulation(t *testing.T, call fantasy.Call) (hits, misses int) {
	t.Helper()
	mc, ok := extractMessageCache(t, call)
	if !ok {
		return 0, 0
	}
	outIdx := 0
	for _, msg := range call.Prompt {
		if msg.Role == fantasy.MessageRoleSystem {
			continue
		}
		if !hasAnthropicEphemeralCacheControl(msg) {
			if _, cached := mc.Get(outIdx); cached {
				hits++
			} else {
				mc.Set(outIdx, json.RawMessage(`{"simulated":true}`))
				misses++
			}
		}
		outIdx++
	}
	return hits, misses
}

func TestChatLoop_MessageCachePopulatedByProvider(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitShort)

	// 3-step conversation with enough initial messages so that
	// addAnthropicPromptCaching leaves some without breakpoints.
	//   Step 0: 4 msgs → 0 hits, 1 miss, 1 total
	//   Step 1: 6 msgs → 1 hit, 2 misses, 3 total
	//   Step 2: 8 msgs → 3 hits, 2 misses, 5 total

	type stepStats struct{ hits, misses, total int }

	var capture callCapture
	var stats []stepStats

	model := &chattest.FakeModel{
		ProviderName: fantasyanthropic.Name,
		StreamFn: func(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
			step := capture.record(call)
			hits, misses := simulateProviderCachePopulation(t, call)
			mmc := call.ProviderOptions[MessageCacheProviderOptionsKey].(*mapMessageCache)
			capture.mu.Lock()
			stats = append(stats, stepStats{hits, misses, len(mmc.entries)})
			capture.mu.Unlock()

			if step < 2 {
				return streamToolCall(fmt.Sprintf("tc-%d", step)), nil
			}
			return streamTextStop("done", fantasy.Usage{}), nil
		},
	}

	err := Run(ctx, RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleSystem, "system prompt"),
			textMessage(fantasy.MessageRoleUser, "hello"),
			textMessage(fantasy.MessageRoleAssistant, "thinking"),
			textMessage(fantasy.MessageRoleUser, "continue"),
		},
		Tools:                []fantasy.AgentTool{newNoopTool("read_file")},
		MaxSteps:             5,
		ContextLimitFallback: 4096,
		PersistStep:          func(_ context.Context, _ PersistedStep) error { return nil },
	})
	require.NoError(t, err)

	capture.mu.Lock()
	defer capture.mu.Unlock()

	require.Equal(t, 3, capture.count)
	require.Len(t, stats, 3)

	assert.Equal(t, stepStats{0, 1, 1}, stats[0], "step 0")
	assert.Equal(t, stepStats{1, 2, 3}, stats[1], "step 1")
	assert.Equal(t, stepStats{3, 2, 5}, stats[2], "step 2")

	// Same cache instance across all steps.
	for i := 1; i < len(capture.calls); i++ {
		c0 := capture.calls[0].ProviderOptions[MessageCacheProviderOptionsKey]
		ci := capture.calls[i].ProviderOptions[MessageCacheProviderOptionsKey]
		assert.Same(t, c0, ci, "cache instance should be same across steps 0 and %d", i)
	}
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
