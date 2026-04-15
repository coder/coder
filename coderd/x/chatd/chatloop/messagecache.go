package chatloop

import (
	"encoding/json"

	"charm.land/fantasy"
)

const (
	// MessageCacheProviderOptionsKey is the provider-options
	// key used to pass a MessageCache through fantasy.Call.
	MessageCacheProviderOptionsKey = "anthropic.message_cache"
)

// MessageCache caches serialized message JSON bytes keyed by
// output message index. Implementations need not be safe for
// concurrent use — the chat step loop is sequential.
type MessageCache interface {
	Get(index int) (json.RawMessage, bool)
	Set(index int, data json.RawMessage)
	Clear()
}

// mapMessageCache is a simple in-memory MessageCache backed by a map.
type mapMessageCache struct {
	entries map[int]json.RawMessage
}

func newMapMessageCache() *mapMessageCache {
	return &mapMessageCache{entries: make(map[int]json.RawMessage)}
}

func (c *mapMessageCache) Get(index int) (json.RawMessage, bool) {
	data, ok := c.entries[index]
	return data, ok
}

func (c *mapMessageCache) Set(index int, data json.RawMessage) {
	c.entries[index] = data
}

func (c *mapMessageCache) Clear() {
	clear(c.entries)
}

// ProviderOptionsData interface methods allow mapMessageCache to
// be stored in fantasy.ProviderOptions. The cache is never
// serialized over the wire — these are stubs.

func (*mapMessageCache) Options() {}

func (*mapMessageCache) MarshalJSON() ([]byte, error) {
	return []byte("null"), nil
}

func (*mapMessageCache) UnmarshalJSON([]byte) error {
	return nil
}

// cloneProviderOptionsWithCache returns a shallow copy of base
// with the message cache entry added. If base is nil a new map
// is created.
func cloneProviderOptionsWithCache(base fantasy.ProviderOptions, cache MessageCache) fantasy.ProviderOptions {
	result := make(fantasy.ProviderOptions, len(base)+1)
	for k, v := range base {
		result[k] = v
	}
	mmc, ok := cache.(*mapMessageCache)
	if !ok {
		return base
	}
	result[MessageCacheProviderOptionsKey] = mmc
	return result
}
