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
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/quartz"
)

const (
	chatConfigProvidersTTL     = 10 * time.Second
	chatConfigModelConfigTTL   = 10 * time.Second
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

type cachedAdvisorConfig struct {
	config    codersdk.AdvisorConfig
	expiresAt time.Time
}

type cachedModelConfig struct {
	config    database.ChatModelConfig
	expiresAt time.Time
}

type modelConfigCacheKey struct {
	organizationID uuid.UUID
	modelConfigID  uuid.UUID
}

type modelConfigSnapshot struct {
	epoch      uint64
	generation uint64
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
	providerFetches    singleflight.Group[string, []database.AIProvider]

	// Model configs (keyed by ID).
	modelTopologyEpoch    uint64
	modelConfigs          map[modelConfigCacheKey]cachedModelConfig
	modelConfigGeneration map[modelConfigCacheKey]uint64
	modelConfigFetches    singleflight.Group[string, database.ChatModelConfig]

	// Default model config (keyed by organization ID).
	defaultModelConfigs          map[uuid.UUID]cachedModelConfig
	defaultModelConfigGeneration map[uuid.UUID]uint64
	defaultModelConfigFetches    singleflight.Group[string, database.ChatModelConfig]

	// User custom prompts (keyed by user ID).
	userPromptEpoch   uint64
	userPrompts       *tlru.Cache[uuid.UUID, string]
	userPromptFetches singleflight.Group[string, string]

	// Advisor configuration (keyed by organization ID).
	advisorConfigs          map[uuid.UUID]cachedAdvisorConfig
	advisorConfigGeneration map[uuid.UUID]uint64
	advisorConfigFetches    singleflight.Group[string, codersdk.AdvisorConfig]
}

func newChatConfigCache(ctx context.Context, db database.Store, clock quartz.Clock) *chatConfigCache {
	return &chatConfigCache{
		db:                           db,
		clock:                        clock,
		ctx:                          ctx,
		modelConfigs:                 make(map[modelConfigCacheKey]cachedModelConfig),
		modelConfigGeneration:        make(map[modelConfigCacheKey]uint64),
		defaultModelConfigs:          make(map[uuid.UUID]cachedModelConfig),
		defaultModelConfigGeneration: make(map[uuid.UUID]uint64),
		advisorConfigs:               make(map[uuid.UUID]cachedAdvisorConfig),
		advisorConfigGeneration:      make(map[uuid.UUID]uint64),
		userPrompts: tlru.New[uuid.UUID](
			tlru.ConstantCost[string],
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
	// Provider topology changed — model selections depend on
	// provider existence, so flush all model-config state.
	clear(c.modelConfigs)
	c.modelTopologyEpoch++
	clear(c.defaultModelConfigs)
	for organizationID := range c.defaultModelConfigGeneration {
		c.defaultModelConfigGeneration[organizationID]++
	}
	c.mu.Unlock()
}

func (c *chatConfigCache) ModelConfigByID(ctx context.Context, organizationID, id uuid.UUID) (database.ChatModelConfig, error) {
	if config, ok := c.cachedModelConfig(organizationID, id); ok {
		return config, nil
	}

	snap := c.modelConfigSnapshot(organizationID, id)
	config, err := singleflightDoChan(ctx, &c.modelConfigFetches, fmt.Sprintf("%d:%d:%s:%s", snap.epoch, snap.generation, organizationID, id), func() (database.ChatModelConfig, error) {
		if cached, ok := c.cachedModelConfig(organizationID, id); ok {
			return cached, nil
		}

		fetched, err := c.db.GetChatModelConfigByID(c.ctx, database.GetChatModelConfigByIDParams{
			OrganizationID: organizationID,
			ID:             id,
		})
		if err != nil {
			return database.ChatModelConfig{}, err
		}
		c.storeModelConfig(organizationID, snap, fetched)
		return cloneModelConfig(fetched), nil
	})
	if err != nil {
		return database.ChatModelConfig{}, err
	}

	return config, nil
}

func (c *chatConfigCache) cachedModelConfig(organizationID, id uuid.UUID) (database.ChatModelConfig, bool) {
	c.mu.RLock()
	key := modelConfigCacheKey{organizationID: organizationID, modelConfigID: id}
	entry, ok := c.modelConfigs[key]
	c.mu.RUnlock()
	if !ok {
		return database.ChatModelConfig{}, false
	}
	if c.clock.Now().Before(entry.expiresAt) {
		return cloneModelConfig(entry.config), true
	}

	c.mu.Lock()
	if current, ok := c.modelConfigs[key]; ok && !c.clock.Now().Before(current.expiresAt) {
		delete(c.modelConfigs, key)
	}
	c.mu.Unlock()

	return database.ChatModelConfig{}, false
}

func (c *chatConfigCache) modelConfigSnapshot(organizationID, modelConfigID uuid.UUID) modelConfigSnapshot {
	key := modelConfigCacheKey{organizationID: organizationID, modelConfigID: modelConfigID}
	c.mu.RLock()
	snap := modelConfigSnapshot{epoch: c.modelTopologyEpoch, generation: c.modelConfigGeneration[key]}
	c.mu.RUnlock()
	return snap
}

func (c *chatConfigCache) storeModelConfig(organizationID uuid.UUID, snap modelConfigSnapshot, config database.ChatModelConfig) {
	key := modelConfigCacheKey{organizationID: organizationID, modelConfigID: config.ID}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.modelTopologyEpoch != snap.epoch {
		return
	}
	if c.modelConfigGeneration[key] != snap.generation {
		return
	}

	c.modelConfigs[key] = cachedModelConfig{
		config:    cloneModelConfig(config),
		expiresAt: c.clock.Now().Add(chatConfigModelConfigTTL),
	}
}

func (c *chatConfigCache) DefaultModelConfig(ctx context.Context, organizationID uuid.UUID) (database.ChatModelConfig, error) {
	if config, ok := c.cachedDefaultModelConfig(organizationID); ok {
		return config, nil
	}

	snap := c.defaultModelConfigSnapshot(organizationID)
	config, err := singleflightDoChan(ctx, &c.defaultModelConfigFetches, fmt.Sprintf("%d:%d:%s:default", snap.epoch, snap.generation, organizationID), func() (database.ChatModelConfig, error) {
		if cached, ok := c.cachedDefaultModelConfig(organizationID); ok {
			return cached, nil
		}

		fetched, err := c.db.GetDefaultChatModelConfig(c.ctx, organizationID)
		if err != nil {
			return database.ChatModelConfig{}, err
		}
		c.storeDefaultModelConfig(organizationID, snap, fetched)
		return cloneModelConfig(fetched), nil
	})
	if err != nil {
		return database.ChatModelConfig{}, err
	}

	return config, nil
}

func (c *chatConfigCache) cachedDefaultModelConfig(organizationID uuid.UUID) (database.ChatModelConfig, bool) {
	c.mu.RLock()
	entry, ok := c.defaultModelConfigs[organizationID]
	c.mu.RUnlock()
	if !ok {
		return database.ChatModelConfig{}, false
	}
	if c.clock.Now().Before(entry.expiresAt) {
		return cloneModelConfig(entry.config), true
	}

	c.mu.Lock()
	if current, ok := c.defaultModelConfigs[organizationID]; ok && !c.clock.Now().Before(current.expiresAt) {
		delete(c.defaultModelConfigs, organizationID)
	}
	c.mu.Unlock()

	return database.ChatModelConfig{}, false
}

func (c *chatConfigCache) defaultModelConfigSnapshot(organizationID uuid.UUID) modelConfigSnapshot {
	c.mu.RLock()
	snap := modelConfigSnapshot{
		epoch:      c.modelTopologyEpoch,
		generation: c.defaultModelConfigGeneration[organizationID],
	}
	c.mu.RUnlock()
	return snap
}

func (c *chatConfigCache) storeDefaultModelConfig(organizationID uuid.UUID, snap modelConfigSnapshot, config database.ChatModelConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.modelTopologyEpoch != snap.epoch {
		return
	}
	if c.defaultModelConfigGeneration[organizationID] != snap.generation {
		return
	}

	c.defaultModelConfigs[organizationID] = cachedModelConfig{
		config:    cloneModelConfig(config),
		expiresAt: c.clock.Now().Add(chatConfigModelConfigTTL),
	}
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

func (c *chatConfigCache) InvalidateModelConfig(id uuid.UUID, organizationID uuid.UUID) {
	c.mu.Lock()
	key := modelConfigCacheKey{organizationID: organizationID, modelConfigID: id}
	delete(c.modelConfigs, key)
	c.modelConfigGeneration[key]++
	delete(c.defaultModelConfigs, organizationID)
	c.defaultModelConfigGeneration[organizationID]++
	c.mu.Unlock()
}

func (c *chatConfigCache) InvalidateUserPrompt(userID uuid.UUID) {
	c.mu.Lock()
	c.userPrompts.Delete(userID)
	c.userPromptEpoch++
	c.mu.Unlock()
}

// InvalidateAdvisorConfig drops the cached advisor configuration for one
// organization. Bumping its generation rejects a stale in-flight fill without
// invalidating other organizations' entries.
func (c *chatConfigCache) InvalidateAdvisorConfig(organizationID uuid.UUID) {
	c.mu.Lock()
	delete(c.advisorConfigs, organizationID)
	c.advisorConfigGeneration[organizationID]++
	c.mu.Unlock()
}

// AdvisorConfig returns the advisor configuration for organizationID.
func (c *chatConfigCache) AdvisorConfig(ctx context.Context, organizationID uuid.UUID) (codersdk.AdvisorConfig, error) {
	if config, ok := c.cachedAdvisorConfig(organizationID); ok {
		return config, nil
	}

	generation := c.advisorConfigGenerationSnapshot(organizationID)
	config, err := singleflightDoChan(
		ctx,
		&c.advisorConfigFetches,
		fmt.Sprintf("%d:%s:advisor", generation, organizationID),
		func() (codersdk.AdvisorConfig, error) {
			if cached, ok := c.cachedAdvisorConfig(organizationID); ok {
				return cached, nil
			}

			raw, err := c.db.GetChatAdvisorConfigForOrganization(c.ctx, organizationID)
			if err != nil {
				return codersdk.AdvisorConfig{}, err
			}
			var cfg codersdk.AdvisorConfig
			if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
				return codersdk.AdvisorConfig{}, err
			}
			c.storeAdvisorConfig(organizationID, generation, cfg)
			return cfg, nil
		},
	)
	if err != nil {
		return codersdk.AdvisorConfig{}, err
	}
	return config, nil
}

func (c *chatConfigCache) cachedAdvisorConfig(organizationID uuid.UUID) (codersdk.AdvisorConfig, bool) {
	c.mu.RLock()
	entry, ok := c.advisorConfigs[organizationID]
	c.mu.RUnlock()
	if !ok {
		return codersdk.AdvisorConfig{}, false
	}
	if c.clock.Now().Before(entry.expiresAt) {
		return entry.config, true
	}

	c.mu.Lock()
	if current, ok := c.advisorConfigs[organizationID]; ok && !c.clock.Now().Before(current.expiresAt) {
		delete(c.advisorConfigs, organizationID)
	}
	c.mu.Unlock()

	return codersdk.AdvisorConfig{}, false
}

func (c *chatConfigCache) advisorConfigGenerationSnapshot(organizationID uuid.UUID) uint64 {
	c.mu.RLock()
	generation := c.advisorConfigGeneration[organizationID]
	c.mu.RUnlock()
	return generation
}

func (c *chatConfigCache) storeAdvisorConfig(organizationID uuid.UUID, generation uint64, config codersdk.AdvisorConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.advisorConfigGeneration[organizationID] != generation {
		return
	}

	c.advisorConfigs[organizationID] = cachedAdvisorConfig{
		config:    config,
		expiresAt: c.clock.Now().Add(chatConfigAdvisorConfigTTL),
	}
}
