package chatd

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"tailscale.com/util/singleflight"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/quartz"
)

const (
	chatConfigProvidersTTL   = 10 * time.Second
	chatConfigModelConfigTTL = 10 * time.Second
	chatConfigUserPromptTTL  = 5 * time.Second
)

type cachedProviders struct {
	providers []database.ChatProvider
	expiresAt time.Time
}

type cachedModelConfig struct {
	config    database.ChatModelConfig
	expiresAt time.Time
}

type cachedUserPrompt struct {
	prompt    string
	expiresAt time.Time
}

// cloneModelConfig returns a shallow copy of cfg with Options
// deep-cloned so the cache owns its own backing array.
func cloneModelConfig(cfg database.ChatModelConfig) database.ChatModelConfig {
	cfg.Options = slices.Clone(cfg.Options)
	return cfg
}

type chatConfigCache struct {
	db    database.Store
	clock quartz.Clock
	// ctx is the server-scoped context used for all DB fills.
	// Cache fills run inside singleflight.Do where one caller
	// becomes the leader for all coalesced waiters. Using a
	// per-request context would mean the leader's cancellation
	// (timeout, user disconnect) fans the error to every waiter.
	// Storing the server context here makes that impossible by
	// construction — callers cannot pass a request context into
	// the shared fill path.
	ctx context.Context

	mu sync.RWMutex

	// Providers (singleton).
	providers          *cachedProviders
	providerGeneration uint64
	providerFetches    singleflight.Group[string, []database.ChatProvider]

	// Model configs (keyed by ID).
	modelConfigs           map[uuid.UUID]cachedModelConfig
	modelConfigGenerations map[uuid.UUID]uint64
	modelConfigFetches     singleflight.Group[string, database.ChatModelConfig]

	// Default model config (singleton).
	defaultModelConfig           *cachedModelConfig
	defaultModelConfigGeneration uint64
	defaultModelConfigFetches    singleflight.Group[string, database.ChatModelConfig]

	// User custom prompts (keyed by user ID).
	userPrompts           map[uuid.UUID]cachedUserPrompt
	userPromptGenerations map[uuid.UUID]uint64
	userPromptFetches     singleflight.Group[string, string]
}

func newChatConfigCache(ctx context.Context, db database.Store, clock quartz.Clock) *chatConfigCache {
	return &chatConfigCache{
		db:                     db,
		clock:                  clock,
		ctx:                    ctx,
		modelConfigs:           make(map[uuid.UUID]cachedModelConfig),
		modelConfigGenerations: make(map[uuid.UUID]uint64),
		userPrompts:            make(map[uuid.UUID]cachedUserPrompt),
		userPromptGenerations:  make(map[uuid.UUID]uint64),
	}
}

// singleflightDoChan wraps a singleflight group's DoChan method,
// allowing the caller to abandon the wait if their context is
// canceled while the shared fill continues running to completion.
// This separates two lifetimes: the fill runs under the server-scoped
// context, while each caller waits under its own request-scoped context.
func singleflightDoChan[K comparable, V any](
	ctx context.Context,
	group *singleflight.Group[K, V],
	key K,
	fn func() (V, error),
) (V, error) {
	ch := group.DoChan(key, fn)
	select {
	case <-ctx.Done():
		var zero V
		return zero, ctx.Err()
	case res := <-ch:
		return res.Val, res.Err
	}
}

func (c *chatConfigCache) EnabledProviders(ctx context.Context) ([]database.ChatProvider, error) {
	if providers, ok := c.cachedProviders(); ok {
		return providers, nil
	}

	providers, err := singleflightDoChan(ctx, &c.providerFetches, "providers", func() ([]database.ChatProvider, error) {
		if cached, ok := c.cachedProviders(); ok {
			return cached, nil
		}

		generation := c.providersGeneration()
		fetched, err := c.db.GetEnabledChatProviders(c.ctx)
		if err != nil {
			return nil, err
		}
		c.storeProviders(generation, fetched)
		return slices.Clone(fetched), nil
	})
	if err != nil {
		return nil, err
	}

	return slices.Clone(providers), nil
}

func (c *chatConfigCache) cachedProviders() ([]database.ChatProvider, bool) {
	c.mu.RLock()
	entry := c.providers
	c.mu.RUnlock()
	if entry == nil {
		return nil, false
	}
	if c.clock.Now().Before(entry.expiresAt) {
		return slices.Clone(entry.providers), true
	}

	c.mu.Lock()
	if current := c.providers; current != nil && !c.clock.Now().Before(current.expiresAt) {
		c.providers = nil
	}
	c.mu.Unlock()

	return nil, false
}

func (c *chatConfigCache) providersGeneration() uint64 {
	c.mu.RLock()
	generation := c.providerGeneration
	c.mu.RUnlock()
	return generation
}

func (c *chatConfigCache) storeProviders(generation uint64, providers []database.ChatProvider) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.providerGeneration != generation {
		return
	}

	c.providers = &cachedProviders{
		providers: slices.Clone(providers),
		expiresAt: c.clock.Now().Add(chatConfigProvidersTTL),
	}
}

func (c *chatConfigCache) InvalidateProviders() {
	c.mu.Lock()
	c.providers = nil
	c.providerGeneration++
	c.mu.Unlock()
	c.providerFetches.Forget("providers")
}

func (c *chatConfigCache) ModelConfigByID(ctx context.Context, id uuid.UUID) (database.ChatModelConfig, error) {
	if config, ok := c.cachedModelConfig(id); ok {
		return config, nil
	}

	config, err := singleflightDoChan(ctx, &c.modelConfigFetches, id.String(), func() (database.ChatModelConfig, error) {
		if cached, ok := c.cachedModelConfig(id); ok {
			return cached, nil
		}

		generation := c.modelConfigGeneration(id)
		fetched, err := c.db.GetChatModelConfigByID(c.ctx, id)
		if err != nil {
			return database.ChatModelConfig{}, err
		}
		c.storeModelConfig(id, generation, fetched)
		return cloneModelConfig(fetched), nil
	})
	if err != nil {
		return database.ChatModelConfig{}, err
	}

	return config, nil
}

func (c *chatConfigCache) cachedModelConfig(id uuid.UUID) (database.ChatModelConfig, bool) {
	c.mu.RLock()
	entry, ok := c.modelConfigs[id]
	c.mu.RUnlock()
	if !ok {
		return database.ChatModelConfig{}, false
	}
	if c.clock.Now().Before(entry.expiresAt) {
		return cloneModelConfig(entry.config), true
	}

	c.mu.Lock()
	if current, ok := c.modelConfigs[id]; ok && !c.clock.Now().Before(current.expiresAt) {
		delete(c.modelConfigs, id)
	}
	c.mu.Unlock()

	return database.ChatModelConfig{}, false
}

func (c *chatConfigCache) modelConfigGeneration(id uuid.UUID) uint64 {
	c.mu.RLock()
	generation := c.modelConfigGenerations[id]
	c.mu.RUnlock()
	return generation
}

func (c *chatConfigCache) storeModelConfig(id uuid.UUID, generation uint64, config database.ChatModelConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.modelConfigGenerations[id] != generation {
		return
	}

	c.modelConfigs[id] = cachedModelConfig{
		config:    cloneModelConfig(config),
		expiresAt: c.clock.Now().Add(chatConfigModelConfigTTL),
	}
}

func (c *chatConfigCache) DefaultModelConfig(ctx context.Context) (database.ChatModelConfig, error) {
	if config, ok := c.cachedDefaultModelConfig(); ok {
		return config, nil
	}

	config, err := singleflightDoChan(ctx, &c.defaultModelConfigFetches, "default", func() (database.ChatModelConfig, error) {
		if cached, ok := c.cachedDefaultModelConfig(); ok {
			return cached, nil
		}

		generation := c.defaultConfigGeneration()
		fetched, err := c.db.GetDefaultChatModelConfig(c.ctx)
		if err != nil {
			return database.ChatModelConfig{}, err
		}
		c.storeDefaultModelConfig(generation, fetched)
		return cloneModelConfig(fetched), nil
	})
	if err != nil {
		return database.ChatModelConfig{}, err
	}

	return config, nil
}

func (c *chatConfigCache) cachedDefaultModelConfig() (database.ChatModelConfig, bool) {
	c.mu.RLock()
	entry := c.defaultModelConfig
	c.mu.RUnlock()
	if entry == nil {
		return database.ChatModelConfig{}, false
	}
	if c.clock.Now().Before(entry.expiresAt) {
		return cloneModelConfig(entry.config), true
	}

	c.mu.Lock()
	if current := c.defaultModelConfig; current != nil && !c.clock.Now().Before(current.expiresAt) {
		c.defaultModelConfig = nil
	}
	c.mu.Unlock()

	return database.ChatModelConfig{}, false
}

func (c *chatConfigCache) defaultConfigGeneration() uint64 {
	c.mu.RLock()
	generation := c.defaultModelConfigGeneration
	c.mu.RUnlock()
	return generation
}

func (c *chatConfigCache) storeDefaultModelConfig(generation uint64, config database.ChatModelConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.defaultModelConfigGeneration != generation {
		return
	}

	c.defaultModelConfig = &cachedModelConfig{
		config:    cloneModelConfig(config),
		expiresAt: c.clock.Now().Add(chatConfigModelConfigTTL),
	}
}

func (c *chatConfigCache) UserPrompt(ctx context.Context, userID uuid.UUID) (string, error) {
	if prompt, ok := c.cachedUserPrompt(userID); ok {
		return prompt, nil
	}

	prompt, err := singleflightDoChan(ctx, &c.userPromptFetches, userID.String(), func() (string, error) {
		if cached, ok := c.cachedUserPrompt(userID); ok {
			return cached, nil
		}

		generation := c.userPromptGeneration(userID)
		fetched, err := c.db.GetUserChatCustomPrompt(c.ctx, userID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.storeUserPrompt(userID, generation, "")
				return "", nil
			}
			return "", err
		}
		c.storeUserPrompt(userID, generation, fetched)
		return fetched, nil
	})
	if err != nil {
		return "", err
	}

	return prompt, nil
}

func (c *chatConfigCache) cachedUserPrompt(userID uuid.UUID) (string, bool) {
	c.mu.RLock()
	entry, ok := c.userPrompts[userID]
	c.mu.RUnlock()
	if !ok {
		return "", false
	}
	if c.clock.Now().Before(entry.expiresAt) {
		return entry.prompt, true
	}

	c.mu.Lock()
	if current, ok := c.userPrompts[userID]; ok && !c.clock.Now().Before(current.expiresAt) {
		delete(c.userPrompts, userID)
	}
	c.mu.Unlock()

	return "", false
}

func (c *chatConfigCache) userPromptGeneration(userID uuid.UUID) uint64 {
	c.mu.RLock()
	generation := c.userPromptGenerations[userID]
	c.mu.RUnlock()
	return generation
}

func (c *chatConfigCache) storeUserPrompt(userID uuid.UUID, generation uint64, prompt string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.userPromptGenerations[userID] != generation {
		return
	}

	c.userPrompts[userID] = cachedUserPrompt{
		prompt:    prompt,
		expiresAt: c.clock.Now().Add(chatConfigUserPromptTTL),
	}
}

func (c *chatConfigCache) InvalidateModelConfig(id uuid.UUID) {
	c.mu.Lock()
	delete(c.modelConfigs, id)
	c.modelConfigGenerations[id]++
	c.defaultModelConfig = nil
	c.defaultModelConfigGeneration++
	c.mu.Unlock()
	c.modelConfigFetches.Forget(id.String())
	c.defaultModelConfigFetches.Forget("default")
}

func (c *chatConfigCache) InvalidateUserPrompt(userID uuid.UUID) {
	c.mu.Lock()
	delete(c.userPrompts, userID)
	c.userPromptGenerations[userID]++
	c.mu.Unlock()
	c.userPromptFetches.Forget(userID.String())
}
