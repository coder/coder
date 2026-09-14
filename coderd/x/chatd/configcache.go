package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/ammario/tlru"
	"github.com/google/uuid"
	"tailscale.com/util/singleflight"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/quartz"
)

const (
	chatConfigProvidersTTL     = 10 * time.Second
	chatConfigUserPromptTTL    = 5 * time.Second
	chatConfigAdvisorConfigTTL = 10 * time.Second
	// Bound user-prompt cache cardinality so one-shot users do not
	// accumulate forever in long-lived chatd processes.
	chatConfigUserPromptEntryLimit = 64 * 1024
)

type cachedProviders struct {
	providers []database.AIProvider
	expiresAt time.Time
}

type advisorRuntimeConfig struct {
	Enabled         bool  `json:"enabled"`
	MaxUsesPerRun   int   `json:"max_uses_per_run"`
	MaxOutputTokens int64 `json:"max_output_tokens"`
}

type cachedAdvisorConfig struct {
	config    advisorRuntimeConfig
	expiresAt time.Time
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
	providerFetches    singleflight.Group[string, []database.AIProvider]

	// User custom prompts (keyed by user ID).
	userPromptEpoch   uint64
	userPrompts       *tlru.Cache[uuid.UUID, string]
	userPromptFetches singleflight.Group[string, string]

	// User personal-memory settings (keyed by user ID).
	userPersonalMemoryEpoch   uint64
	userPersonalMemoryEnabled *tlru.Cache[uuid.UUID, bool]
	userPersonalMemoryFetches singleflight.Group[string, bool]

	// Advisor configuration (singleton).
	advisorConfig           *cachedAdvisorConfig
	advisorConfigGeneration uint64
	advisorConfigFetches    singleflight.Group[string, advisorRuntimeConfig]
}

func newChatConfigCache(ctx context.Context, db database.Store, clock quartz.Clock) *chatConfigCache {
	return &chatConfigCache{
		db:    db,
		clock: clock,
		ctx:   ctx,
		userPrompts: tlru.New[uuid.UUID](
			tlru.ConstantCost[string],
			chatConfigUserPromptEntryLimit,
		),
		userPersonalMemoryEnabled: tlru.New[uuid.UUID](
			tlru.ConstantCost[bool],
			chatConfigUserPromptEntryLimit,
		),
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

func (c *chatConfigCache) EnabledProviders(ctx context.Context) ([]database.AIProvider, error) {
	if providers, ok := c.cachedProviders(); ok {
		return providers, nil
	}

	generation := c.providersGeneration()
	providers, err := singleflightDoChan(
		ctx,
		&c.providerFetches,
		fmt.Sprintf("%d:providers", generation),
		func() ([]database.AIProvider, error) {
			if cached, ok := c.cachedProviders(); ok {
				return cached, nil
			}

			fetched, err := c.db.GetAIProviders(c.ctx, database.GetAIProvidersParams{})
			if err != nil {
				return nil, err
			}
			c.storeProviders(generation, fetched)
			return slices.Clone(fetched), nil
		},
	)
	if err != nil {
		return nil, err
	}

	return slices.Clone(providers), nil
}

func (c *chatConfigCache) cachedProviders() ([]database.AIProvider, bool) {
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

func (c *chatConfigCache) storeProviders(generation uint64, providers []database.AIProvider) {
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
}

func (c *chatConfigCache) UserPrompt(ctx context.Context, userID uuid.UUID) (string, error) {
	if prompt, ok := c.cachedUserPrompt(userID); ok {
		return prompt, nil
	}

	epoch := c.currentUserPromptEpoch()
	prompt, err := singleflightDoChan(ctx, &c.userPromptFetches, fmt.Sprintf("%d:%s", epoch, userID), func() (string, error) {
		if cached, ok := c.cachedUserPrompt(userID); ok {
			return cached, nil
		}

		fetched, err := c.db.GetUserChatCustomPrompt(c.ctx, userID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.storeUserPrompt(epoch, userID, "")
				return "", nil
			}
			return "", err
		}
		c.storeUserPrompt(epoch, userID, fetched)
		return fetched, nil
	})
	if err != nil {
		return "", err
	}

	return prompt, nil
}

func (c *chatConfigCache) cachedUserPrompt(userID uuid.UUID) (string, bool) {
	prompt, _, ok := c.userPrompts.Get(userID)
	if !ok {
		return "", false
	}
	return prompt, true
}

func (c *chatConfigCache) currentUserPromptEpoch() uint64 {
	c.mu.RLock()
	epoch := c.userPromptEpoch
	c.mu.RUnlock()
	return epoch
}

func (c *chatConfigCache) storeUserPrompt(epoch uint64, userID uuid.UUID, prompt string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.userPromptEpoch != epoch {
		return
	}

	c.userPrompts.Set(userID, prompt, chatConfigUserPromptTTL)
}

func (c *chatConfigCache) InvalidateUserPrompt(userID uuid.UUID) {
	c.mu.Lock()
	c.userPrompts.Delete(userID)
	c.userPromptEpoch++
	c.mu.Unlock()
}

func (c *chatConfigCache) GetUserChatPersonalMemoryEnabled(ctx context.Context, userID uuid.UUID) (bool, error) {
	if enabled, ok := c.cachedUserPersonalMemoryEnabled(userID); ok {
		return enabled, nil
	}

	epoch := c.currentUserPersonalMemoryEpoch()
	enabled, err := singleflightDoChan(ctx, &c.userPersonalMemoryFetches, fmt.Sprintf("%d:%s", epoch, userID), func() (bool, error) {
		if cached, ok := c.cachedUserPersonalMemoryEnabled(userID); ok {
			return cached, nil
		}

		fetched, err := c.db.GetUserChatPersonalMemoryEnabled(c.ctx, userID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.storeUserPersonalMemoryEnabled(epoch, userID, true)
				return true, nil
			}
			return false, err
		}
		enabled := fetched == "true"
		c.storeUserPersonalMemoryEnabled(epoch, userID, enabled)
		return enabled, nil
	})
	if err != nil {
		return false, err
	}
	return enabled, nil
}

func (c *chatConfigCache) cachedUserPersonalMemoryEnabled(userID uuid.UUID) (enabled bool, ok bool) {
	enabled, _, ok = c.userPersonalMemoryEnabled.Get(userID)
	return enabled, ok
}

func (c *chatConfigCache) currentUserPersonalMemoryEpoch() uint64 {
	c.mu.RLock()
	epoch := c.userPersonalMemoryEpoch
	c.mu.RUnlock()
	return epoch
}

func (c *chatConfigCache) storeUserPersonalMemoryEnabled(epoch uint64, userID uuid.UUID, enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.userPersonalMemoryEpoch != epoch {
		return
	}
	c.userPersonalMemoryEnabled.Set(userID, enabled, chatConfigUserPromptTTL)
}

func (c *chatConfigCache) InvalidateUserPersonalMemoryEnabled(userID uuid.UUID) {
	c.mu.Lock()
	c.userPersonalMemoryEnabled.Delete(userID)
	c.userPersonalMemoryEpoch++
	c.mu.Unlock()
}

// InvalidateAdvisorConfig drops the cached advisor configuration so the
// next AdvisorConfig call re-fetches from the database. Called from the
// ChatConfigEvent subscriber after an admin writes
// PUT /api/experimental/chats/config/advisor; without this the cache
// could serve stale enabled/model/limits for up to
// chatConfigAdvisorConfigTTL. Bumping the generation counter also
// discards any in-flight fill started before the invalidation, so a
// stale DB read cannot re-cache the pre-update value.
func (c *chatConfigCache) InvalidateAdvisorConfig() {
	c.mu.Lock()
	c.advisorConfig = nil
	c.advisorConfigGeneration++
	c.mu.Unlock()
}

// AdvisorConfig returns the deployment-wide advisor configuration. The
// underlying site-config row changes on the order of hours or days, so
// this cache saves a per-turn DB round trip on chats that reference the
// advisor. Parse errors and lookup errors are surfaced to the caller;
// callers that prefer silent fallback handle that at the call site.
func (c *chatConfigCache) AdvisorConfig(ctx context.Context) (advisorRuntimeConfig, error) {
	if config, ok := c.cachedAdvisorConfig(); ok {
		return config, nil
	}

	generation := c.advisorConfigGenerationSnapshot()
	config, err := singleflightDoChan(
		ctx,
		&c.advisorConfigFetches,
		fmt.Sprintf("%d:advisor", generation),
		func() (advisorRuntimeConfig, error) {
			if cached, ok := c.cachedAdvisorConfig(); ok {
				return cached, nil
			}

			raw, err := c.db.GetChatAdvisorConfig(c.ctx)
			if err != nil {
				return advisorRuntimeConfig{}, err
			}
			var cfg advisorRuntimeConfig
			if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
				return advisorRuntimeConfig{}, err
			}
			c.storeAdvisorConfig(generation, cfg)
			return cfg, nil
		},
	)
	if err != nil {
		return advisorRuntimeConfig{}, err
	}
	return config, nil
}

func (c *chatConfigCache) cachedAdvisorConfig() (advisorRuntimeConfig, bool) {
	c.mu.RLock()
	entry := c.advisorConfig
	c.mu.RUnlock()
	if entry == nil {
		return advisorRuntimeConfig{}, false
	}
	if c.clock.Now().Before(entry.expiresAt) {
		return entry.config, true
	}

	c.mu.Lock()
	if current := c.advisorConfig; current != nil && !c.clock.Now().Before(current.expiresAt) {
		c.advisorConfig = nil
	}
	c.mu.Unlock()

	return advisorRuntimeConfig{}, false
}

func (c *chatConfigCache) advisorConfigGenerationSnapshot() uint64 {
	c.mu.RLock()
	generation := c.advisorConfigGeneration
	c.mu.RUnlock()
	return generation
}

func (c *chatConfigCache) storeAdvisorConfig(generation uint64, config advisorRuntimeConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.advisorConfigGeneration != generation {
		return
	}

	c.advisorConfig = &cachedAdvisorConfig{
		config:    config,
		expiresAt: c.clock.Now().Add(chatConfigAdvisorConfigTTL),
	}
}
