package chatd

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk/agenthooks"
)

const (
	// hookDecisionCacheMaxEntries bounds process memory; overflow evicts
	// the oldest entries. A miss only widens the duplicate-dispatch
	// window, so the bound is a resource guard, not a correctness limit.
	hookDecisionCacheMaxEntries = 4096
	hookDecisionCacheTTL        = 2 * time.Hour
)

// hookDecisionCache banks successful pre_tool_use decisions so
// same-process turn re-drives (interruptions, transient errors,
// requires-action recovery, retry after a hook-caused chat error) reuse
// the consumer's decision instead of re-consulting it. The cache is
// best-effort only: correctness and the documented at-least-once
// contract never depend on it. A miss (process loss, failover,
// eviction) dispatches fresh and the consumer's latest decision wins.
type hookDecisionCache struct {
	mu      sync.Mutex
	entries map[hookDecisionKey]*hookDecisionEntry
}

type hookDecisionKey struct {
	chatID    uuid.UUID
	toolUseID string
}

type hookDecisionEntry struct {
	toolName  string
	toolInput string
	response  agenthooks.Response
	// effectsApplied is set once transcript effects commit; replay
	// then applies only the permission decision.
	effectsApplied bool
	addedAt        time.Time
}

func newHookDecisionCache() *hookDecisionCache {
	return &hookDecisionCache{entries: make(map[hookDecisionKey]*hookDecisionEntry)}
}

// Replayed lookups derive input from jsonb-round-tripped content whose
// whitespace and key order differ from the streamed original.
func canonicalHookInput(input string) string {
	var value any
	if err := json.Unmarshal([]byte(input), &value); err != nil {
		return input
	}
	out, err := json.Marshal(value)
	if err != nil {
		return input
	}
	return string(out)
}

func (c *hookDecisionCache) put(chatID uuid.UUID, toolUseID, toolName, toolInput string, response agenthooks.Response) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneLocked()
	c.entries[hookDecisionKey{chatID: chatID, toolUseID: toolUseID}] = &hookDecisionEntry{
		toolName:  toolName,
		toolInput: canonicalHookInput(toolInput),
		response:  response,
		addedAt:   time.Now(),
	}
}

func (c *hookDecisionCache) lookup(chatID uuid.UUID, toolUseID, toolName, toolInput string) (response agenthooks.Response, effectsApplied bool, ok bool) {
	if c == nil {
		return agenthooks.Response{}, false, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[hookDecisionKey{chatID: chatID, toolUseID: toolUseID}]
	if !ok || entry.toolName != toolName || time.Since(entry.addedAt) > hookDecisionCacheTTL {
		return agenthooks.Response{}, false, false
	}
	input := canonicalHookInput(toolInput)
	if entry.toolInput != input && !entry.matchesOverride(input) {
		return agenthooks.Response{}, false, false
	}
	return entry.response, entry.effectsApplied, true
}

func (e *hookDecisionEntry) matchesOverride(canonicalInput string) bool {
	permission := e.response.Permission
	return permission != nil &&
		permission.Decision == agenthooks.PermissionAllow &&
		len(permission.InputOverride) > 0 &&
		canonicalHookInput(string(permission.InputOverride)) == canonicalInput
}

func (c *hookDecisionCache) markEffectsApplied(chatID uuid.UUID, toolUseIDs []string) {
	if c == nil || len(toolUseIDs) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, toolUseID := range toolUseIDs {
		if entry, ok := c.entries[hookDecisionKey{chatID: chatID, toolUseID: toolUseID}]; ok {
			entry.effectsApplied = true
		}
	}
}

func (c *hookDecisionCache) evict(chatID uuid.UUID, toolUseIDs []string) {
	if c == nil || len(toolUseIDs) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, toolUseID := range toolUseIDs {
		delete(c.entries, hookDecisionKey{chatID: chatID, toolUseID: toolUseID})
	}
}

// Entries deliberately survive chat error state so a retry reuses
// decisions banked for the step's other tool calls.
func (c *hookDecisionCache) evictChat(chatID uuid.UUID) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.entries {
		if key.chatID == chatID {
			delete(c.entries, key)
		}
	}
}

func (c *hookDecisionCache) pruneLocked() {
	now := time.Now()
	for key, entry := range c.entries {
		if now.Sub(entry.addedAt) > hookDecisionCacheTTL {
			delete(c.entries, key)
		}
	}
	for len(c.entries) >= hookDecisionCacheMaxEntries {
		var oldestKey hookDecisionKey
		var oldest time.Time
		first := true
		for key, entry := range c.entries {
			if first || entry.addedAt.Before(oldest) {
				oldestKey, oldest, first = key, entry.addedAt, false
			}
		}
		delete(c.entries, oldestKey)
	}
}
