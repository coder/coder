package chatd //nolint:testpackage // Uses internal cache state.

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type stubChatConfigStore struct {
	database.Store

	getEnabledChatProviders   func(context.Context) ([]database.ChatProvider, error)
	getChatModelConfigByID    func(context.Context, uuid.UUID) (database.ChatModelConfig, error)
	getDefaultChatModelConfig func(context.Context) (database.ChatModelConfig, error)
	getModelProviderConfigs   func(context.Context, uuid.UUID) ([]database.GetModelProviderConfigsRow, error)
	getUserChatCustomPrompt   func(context.Context, uuid.UUID) (string, error)

	enabledProvidersCalls     atomic.Int32
	modelConfigByIDCalls      atomic.Int32
	defaultModelConfigCall    atomic.Int32
	modelProviderConfigsCalls atomic.Int32
	userPromptCalls           atomic.Int32
}

func (s *stubChatConfigStore) GetEnabledChatProviders(ctx context.Context) ([]database.ChatProvider, error) {
	s.enabledProvidersCalls.Add(1)
	if s.getEnabledChatProviders == nil {
		panic("unexpected GetEnabledChatProviders call")
	}
	return s.getEnabledChatProviders(ctx)
}

func (s *stubChatConfigStore) GetChatModelConfigByID(ctx context.Context, id uuid.UUID) (database.ChatModelConfig, error) {
	s.modelConfigByIDCalls.Add(1)
	if s.getChatModelConfigByID == nil {
		panic("unexpected GetChatModelConfigByID call")
	}
	return s.getChatModelConfigByID(ctx, id)
}

func (s *stubChatConfigStore) GetDefaultChatModelConfig(ctx context.Context) (database.ChatModelConfig, error) {
	s.defaultModelConfigCall.Add(1)
	if s.getDefaultChatModelConfig == nil {
		panic("unexpected GetDefaultChatModelConfig call")
	}
	return s.getDefaultChatModelConfig(ctx)
}

func (s *stubChatConfigStore) GetModelProviderConfigs(ctx context.Context, modelConfigID uuid.UUID) ([]database.GetModelProviderConfigsRow, error) {
	s.modelProviderConfigsCalls.Add(1)
	if s.getModelProviderConfigs == nil {
		panic("unexpected GetModelProviderConfigs call")
	}
	return s.getModelProviderConfigs(ctx, modelConfigID)
}

func (s *stubChatConfigStore) GetUserChatCustomPrompt(ctx context.Context, userID uuid.UUID) (string, error) {
	s.userPromptCalls.Add(1)
	if s.getUserChatCustomPrompt == nil {
		panic("unexpected GetUserChatCustomPrompt call")
	}
	return s.getUserChatCustomPrompt(ctx, userID)
}

func setupCache(t *testing.T, wait time.Duration) (context.Context, *quartz.Mock, *stubChatConfigStore, *chatConfigCache) {
	t.Helper()
	if wait == 0 {
		wait = testutil.WaitShort
	}
	ctx := testutil.Context(t, wait)
	clock := quartz.NewMock(t)
	store := &stubChatConfigStore{}
	return ctx, clock, store, newChatConfigCache(ctx, store, clock)
}

func TestConfigCache_EnabledProviders(t *testing.T) {
	t.Parallel()

	t.Run("CacheHit", func(t *testing.T) {
		t.Parallel()

		ctx, _, store, cache := setupCache(t, 0)
		providers := []database.ChatProvider{testChatProvider("provider-a")}
		store.getEnabledChatProviders = func(context.Context) ([]database.ChatProvider, error) {
			return providers, nil
		}

		first, err := cache.EnabledProviders(ctx)
		require.NoError(t, err)
		second, err := cache.EnabledProviders(ctx)
		require.NoError(t, err)

		require.Equal(t, providers, first)
		require.Equal(t, providers, second)
		require.Equal(t, int32(1), store.enabledProvidersCalls.Load())
	})

	t.Run("PreservesAllProviders", func(t *testing.T) {
		t.Parallel()

		ctx, _, store, cache := setupCache(t, 0)
		anthropic := testChatProvider("anthropic")
		anthropic.ID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
		anthropic.CreatedAt = time.Unix(0, 0).UTC()
		anthropic.UpdatedAt = anthropic.CreatedAt
		olderOpenAI := testChatProvider("openai")
		olderOpenAI.ID = uuid.MustParse("00000000-0000-0000-0000-000000000002")
		olderOpenAI.CreatedAt = time.Unix(1, 0).UTC()
		olderOpenAI.UpdatedAt = olderOpenAI.CreatedAt
		newerOpenAI := testChatProvider("openai")
		newerOpenAI.ID = uuid.MustParse("00000000-0000-0000-0000-000000000003")
		newerOpenAI.CreatedAt = time.Unix(2, 0).UTC()
		newerOpenAI.UpdatedAt = newerOpenAI.CreatedAt
		store.getEnabledChatProviders = func(context.Context) ([]database.ChatProvider, error) {
			return []database.ChatProvider{anthropic, olderOpenAI, newerOpenAI}, nil
		}

		providers, err := cache.EnabledProviders(ctx)
		require.NoError(t, err)
		require.Equal(t, []database.ChatProvider{anthropic, olderOpenAI, newerOpenAI}, providers)
	})

	t.Run("TTLExpiry", func(t *testing.T) {
		t.Parallel()

		ctx, clock, store, cache := setupCache(t, 0)
		store.getEnabledChatProviders = func(context.Context) ([]database.ChatProvider, error) {
			call := store.enabledProvidersCalls.Load()
			return []database.ChatProvider{testChatProvider(fmt.Sprintf("provider-%d", call))}, nil
		}

		first, err := cache.EnabledProviders(ctx)
		require.NoError(t, err)
		clock.Advance(chatConfigProvidersTTL).MustWait(ctx)
		second, err := cache.EnabledProviders(ctx)
		require.NoError(t, err)

		require.NotEqual(t, first, second)
		require.Equal(t, int32(2), store.enabledProvidersCalls.Load())
	})

	t.Run("Invalidation", func(t *testing.T) {
		t.Parallel()

		ctx, _, store, cache := setupCache(t, 0)
		store.getEnabledChatProviders = func(context.Context) ([]database.ChatProvider, error) {
			call := store.enabledProvidersCalls.Load()
			return []database.ChatProvider{testChatProvider(fmt.Sprintf("provider-%d", call))}, nil
		}

		first, err := cache.EnabledProviders(ctx)
		require.NoError(t, err)
		cache.InvalidateProviders()
		second, err := cache.EnabledProviders(ctx)
		require.NoError(t, err)

		require.NotEqual(t, first, second)
		require.Equal(t, int32(2), store.enabledProvidersCalls.Load())
	})
}

func TestConfigCache_EnabledProviderByID(t *testing.T) {
	t.Parallel()

	t.Run("WarmCacheHit", func(t *testing.T) {
		t.Parallel()

		ctx, _, store, cache := setupCache(t, 0)
		providerA := testChatProvider("provider-a")
		providerA.ID = uuid.MustParse("00000000-0000-0000-0000-000000000011")
		providerB := testChatProvider("provider-b")
		providerB.ID = uuid.MustParse("00000000-0000-0000-0000-000000000012")
		store.getEnabledChatProviders = func(context.Context) ([]database.ChatProvider, error) {
			return []database.ChatProvider{providerA, providerB}, nil
		}

		_, err := cache.EnabledProviders(ctx)
		require.NoError(t, err)

		provider, ok, err := cache.EnabledProviderByID(ctx, providerB.ID)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, providerB, provider)
		require.Equal(t, int32(1), store.enabledProvidersCalls.Load())
	})

	t.Run("TTLExpiryRefreshesSliceAndIndex", func(t *testing.T) {
		t.Parallel()

		ctx, clock, store, cache := setupCache(t, 0)
		providerID := uuid.MustParse("00000000-0000-0000-0000-000000000021")
		store.getEnabledChatProviders = func(context.Context) ([]database.ChatProvider, error) {
			call := store.enabledProvidersCalls.Load()
			provider := testChatProvider(fmt.Sprintf("provider-%d", call))
			provider.ID = providerID
			return []database.ChatProvider{provider}, nil
		}

		firstProviders, err := cache.EnabledProviders(ctx)
		require.NoError(t, err)

		clock.Advance(chatConfigProvidersTTL).MustWait(ctx)

		provider, ok, err := cache.EnabledProviderByID(ctx, providerID)
		require.NoError(t, err)
		require.True(t, ok)

		secondProviders, err := cache.EnabledProviders(ctx)
		require.NoError(t, err)

		require.NotEqual(t, firstProviders[0], provider)
		require.Equal(t, provider, secondProviders[0])
		require.Equal(t, int32(2), store.enabledProvidersCalls.Load())
	})

	t.Run("InvalidationClearsSliceAndIndex", func(t *testing.T) {
		t.Parallel()

		ctx, _, store, cache := setupCache(t, 0)
		providerID := uuid.MustParse("00000000-0000-0000-0000-000000000031")
		store.getEnabledChatProviders = func(context.Context) ([]database.ChatProvider, error) {
			call := store.enabledProvidersCalls.Load()
			provider := testChatProvider(fmt.Sprintf("provider-%d", call))
			provider.ID = providerID
			return []database.ChatProvider{provider}, nil
		}

		firstProviders, err := cache.EnabledProviders(ctx)
		require.NoError(t, err)

		cache.InvalidateProviders()
		require.Nil(t, cache.providers)

		provider, ok, err := cache.EnabledProviderByID(ctx, providerID)
		require.NoError(t, err)
		require.True(t, ok)

		secondProviders, err := cache.EnabledProviders(ctx)
		require.NoError(t, err)

		require.NotEqual(t, firstProviders[0], provider)
		require.Equal(t, provider, secondProviders[0])
		require.Equal(t, int32(2), store.enabledProvidersCalls.Load())
	})

	t.Run("MissingID", func(t *testing.T) {
		t.Parallel()

		ctx, _, store, cache := setupCache(t, 0)
		provider := testChatProvider("provider-a")
		store.getEnabledChatProviders = func(context.Context) ([]database.ChatProvider, error) {
			return []database.ChatProvider{provider}, nil
		}

		_, err := cache.EnabledProviders(ctx)
		require.NoError(t, err)
		cachedProviders := cache.providers

		missingID := uuid.MustParse("00000000-0000-0000-0000-000000000041")
		lookup, ok, err := cache.EnabledProviderByID(ctx, missingID)
		require.NoError(t, err)
		require.False(t, ok)
		require.Equal(t, database.ChatProvider{}, lookup)
		require.Same(t, cachedProviders, cache.providers)
		require.Equal(t, int32(1), store.enabledProvidersCalls.Load())
	})
}

func TestConfigCache_ModelConfigByID(t *testing.T) {
	t.Parallel()

	t.Run("CacheHit", func(t *testing.T) {
		t.Parallel()

		ctx, _, store, cache := setupCache(t, 0)
		configID := uuid.New()
		config := testChatModelConfig(configID, "model-a")
		store.getChatModelConfigByID = func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
			return config, nil
		}

		first, err := cache.ModelConfigByID(ctx, configID)
		require.NoError(t, err)
		second, err := cache.ModelConfigByID(ctx, configID)
		require.NoError(t, err)

		require.Equal(t, config, first)
		require.Equal(t, config, second)
		require.Equal(t, int32(1), store.modelConfigByIDCalls.Load())
	})

	t.Run("ClonesOptionsForCache", func(t *testing.T) {
		t.Parallel()

		ctx, _, store, cache := setupCache(t, 0)
		configID := uuid.New()
		const options = `{"temperature":0.1}`
		config := testChatModelConfig(configID, "model-a")
		config.Options = []byte(options)
		store.getChatModelConfigByID = func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
			return config, nil
		}

		// First call populates cache via singleflight.
		first, err := cache.ModelConfigByID(ctx, configID)
		require.NoError(t, err)
		first.Options[0] = 'x' // mutate singleflight return

		// Second call is a cache hit.
		second, err := cache.ModelConfigByID(ctx, configID)
		require.NoError(t, err)
		require.Equal(t, options, string(second.Options))
		second.Options[0] = 'y' // mutate cache-hit return

		// Third call is another cache hit. It must be unaffected.
		third, err := cache.ModelConfigByID(ctx, configID)
		require.NoError(t, err)
		require.Equal(t, options, string(third.Options))
	})

	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()

		ctx, _, store, cache := setupCache(t, 0)
		configID := uuid.New()
		store.getChatModelConfigByID = func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
			return database.ChatModelConfig{}, sql.ErrNoRows
		}

		_, err := cache.ModelConfigByID(ctx, configID)
		require.ErrorIs(t, err, sql.ErrNoRows)
		_, err = cache.ModelConfigByID(ctx, configID)
		require.ErrorIs(t, err, sql.ErrNoRows)

		require.Equal(t, int32(2), store.modelConfigByIDCalls.Load())
		_, ok := cache.modelConfigs[configID]
		require.False(t, ok)
	})
}

type modelAttachmentSpec struct {
	id       string
	provider string
	priority int32
}

type modelAttachmentsTestScenario struct {
	ctx         context.Context
	clock       *quartz.Mock
	modelID     uuid.UUID
	store       *stubChatConfigStore
	cache       *chatConfigCache
	attachments [][]database.GetModelProviderConfigsRow
	blocked     []blockedCall[[]database.GetModelProviderConfigsRow]
}

func singleModelAttachment(id, provider string) []modelAttachmentSpec {
	return []modelAttachmentSpec{{id: id, provider: provider}}
}

func newModelAttachmentsTestScenario(t *testing.T, wait time.Duration, prepare func(*modelAttachmentsTestScenario), sets ...[]modelAttachmentSpec) modelAttachmentsTestScenario {
	t.Helper()
	if wait == 0 {
		wait = testutil.WaitShort
	}
	scenario := modelAttachmentsTestScenario{ctx: testutil.Context(t, wait), clock: quartz.NewMock(t), modelID: uuid.New()}
	for _, specs := range sets {
		attachments := make([]database.GetModelProviderConfigsRow, 0, len(specs))
		for _, spec := range specs {
			providerConfigID := uuid.New()
			if spec.id != "" {
				providerConfigID = uuid.MustParse(spec.id)
			}
			attachments = append(attachments, testModelAttachment(scenario.modelID, providerConfigID, spec.provider, spec.priority))
		}
		scenario.attachments = append(scenario.attachments, attachments)
	}
	require.NotEmpty(t, scenario.attachments)
	if prepare != nil {
		prepare(&scenario)
	}
	scenario.store = &stubChatConfigStore{}
	scenario.store.getModelProviderConfigs = func(context.Context, uuid.UUID) ([]database.GetModelProviderConfigsRow, error) {
		call := int(scenario.store.modelProviderConfigsCalls.Load()) - 1
		if len(scenario.blocked) > 0 && call > 0 {
			if call > len(scenario.blocked) {
				return nil, xerrors.Errorf("unexpected model attachment call %d", call+1)
			}
			fetch := scenario.blocked[call-1]
			close(fetch.started)
			<-fetch.release
			return fetch.value, nil
		}
		if call >= len(scenario.attachments) {
			call = len(scenario.attachments) - 1
		}
		return scenario.attachments[call], nil
	}
	scenario.cache = newChatConfigCache(scenario.ctx, scenario.store, scenario.clock)
	return scenario
}

func TestConfigCache_ModelAttachments(t *testing.T) {
	t.Parallel()
	assertInvalidated := func(t *testing.T, scenario modelAttachmentsTestScenario) {
		_, ok := scenario.cache.modelAttachments[scenario.modelID]
		require.False(t, ok)
		require.Equal(t, uint64(1), scenario.cache.modelAttachmentEpoch)
	}
	tests := []struct {
		name      string
		sets      [][]modelAttachmentSpec
		before    func(*testing.T, modelAttachmentsTestScenario)
		wantCalls int32
	}{
		{name: "CacheHit", sets: [][]modelAttachmentSpec{{{provider: "openai"}, {provider: "anthropic", priority: 1}}}, wantCalls: 1},
		{
			name: "TTLExpiry", sets: [][]modelAttachmentSpec{singleModelAttachment("00000000-0000-0000-0000-000000000051", "openai"), singleModelAttachment("00000000-0000-0000-0000-000000000052", "anthropic")}, wantCalls: 2,
			before: func(t *testing.T, scenario modelAttachmentsTestScenario) {
				scenario.clock.Advance(chatConfigModelConfigTTL).MustWait(scenario.ctx)
			},
		},
		{
			name: "InvalidateModelConfig_ClearsAttachments", sets: [][]modelAttachmentSpec{singleModelAttachment("00000000-0000-0000-0000-000000000061", "openai"), singleModelAttachment("00000000-0000-0000-0000-000000000062", "openrouter")}, wantCalls: 2,
			before: func(t *testing.T, scenario modelAttachmentsTestScenario) {
				scenario.cache.InvalidateModelConfig(scenario.modelID)
				assertInvalidated(t, scenario)
			},
		},
		{
			name: "InvalidateProviders_ClearsAttachments", sets: [][]modelAttachmentSpec{singleModelAttachment("00000000-0000-0000-0000-000000000071", "openai"), singleModelAttachment("00000000-0000-0000-0000-000000000072", "google")}, wantCalls: 2,
			before: func(t *testing.T, scenario modelAttachmentsTestScenario) {
				scenario.cache.InvalidateProviders()
				assertInvalidated(t, scenario)
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			scenario := newModelAttachmentsTestScenario(t, 0, nil, tt.sets...)
			first, err := scenario.cache.ModelAttachments(scenario.ctx, scenario.modelID)
			require.NoError(t, err)
			if tt.before != nil {
				tt.before(t, scenario)
			}
			second, err := scenario.cache.ModelAttachments(scenario.ctx, scenario.modelID)
			require.NoError(t, err)
			require.Equal(t, scenario.attachments[0], first)
			require.Equal(t, scenario.attachments[len(scenario.attachments)-1], second)
			require.Equal(t, tt.wantCalls, scenario.store.modelProviderConfigsCalls.Load())
		})
	}
}

func TestConfigCache_ModelAttachments_StaleWriteDiscarded(t *testing.T) {
	t.Parallel()
	scenario := newModelAttachmentsTestScenario(t, testutil.WaitMedium, func(scenario *modelAttachmentsTestScenario) {
		for _, attachments := range scenario.attachments[1:] {
			scenario.blocked = append(scenario.blocked, newBlockedCall(attachments))
		}
	},
		singleModelAttachment("00000000-0000-0000-0000-000000000081", "openai"),
		singleModelAttachment("00000000-0000-0000-0000-000000000082", "azure"),
		singleModelAttachment("00000000-0000-0000-0000-000000000083", "anthropic"),
	)
	warm, err := scenario.cache.ModelAttachments(scenario.ctx, scenario.modelID)
	require.NoError(t, err)
	require.Equal(t, scenario.attachments[0], warm)
	scenario.cache.InvalidateProviders()
	runBlockedStaleWriteTest(
		t,
		scenario.blocked,
		func() ([]database.GetModelProviderConfigsRow, error) {
			return scenario.cache.ModelAttachments(scenario.ctx, scenario.modelID)
		},
		scenario.cache.InvalidateProviders,
		func(t *testing.T) { _, ok := scenario.cache.modelAttachments[scenario.modelID]; require.False(t, ok) },
		func() int32 { return scenario.store.modelProviderConfigsCalls.Load() },
		3,
	)
}

func TestConfigCache_InvalidateModelConfig_CascadesToDefault(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	configID := uuid.New()
	config := testChatModelConfig(configID, "model-a")
	store := &stubChatConfigStore{}
	store.getChatModelConfigByID = func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
		return config, nil
	}
	store.getDefaultChatModelConfig = func(context.Context) (database.ChatModelConfig, error) {
		call := store.defaultModelConfigCall.Load()
		return testChatModelConfig(uuid.New(), fmt.Sprintf("default-model-%d", call)), nil
	}
	cache := newChatConfigCache(ctx, store, clock)

	_, err := cache.ModelConfigByID(ctx, configID)
	require.NoError(t, err)
	firstDefault, err := cache.DefaultModelConfig(ctx)
	require.NoError(t, err)

	cache.InvalidateModelConfig(configID)
	require.Nil(t, cache.defaultModelConfig)

	secondDefault, err := cache.DefaultModelConfig(ctx)
	require.NoError(t, err)

	require.NotEqual(t, firstDefault, secondDefault)
	require.Equal(t, int32(2), store.defaultModelConfigCall.Load())
}

func TestConfigCache_UserPrompt_NegativeCaching(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	userID := uuid.New()
	store := &stubChatConfigStore{
		getUserChatCustomPrompt: func(context.Context, uuid.UUID) (string, error) {
			return "", sql.ErrNoRows
		},
	}
	cache := newChatConfigCache(ctx, store, clock)

	first, err := cache.UserPrompt(ctx, userID)
	require.NoError(t, err)
	second, err := cache.UserPrompt(ctx, userID)
	require.NoError(t, err)

	require.Empty(t, first)
	require.Empty(t, second)
	require.Equal(t, int32(1), store.userPromptCalls.Load())
}

func TestConfigCache_UserPrompt_ExpiredEntryRefetches(t *testing.T) {
	t.Parallel()

	ctx, _, store, cache := setupCache(t, 0)
	userID := uuid.New()
	store.getUserChatCustomPrompt = func(context.Context, uuid.UUID) (string, error) {
		call := store.userPromptCalls.Load()
		return fmt.Sprintf("prompt-%d", call), nil
	}
	cache.userPrompts.Set(userID, "stale", -time.Second)

	first, err := cache.UserPrompt(ctx, userID)
	require.NoError(t, err)
	second, err := cache.UserPrompt(ctx, userID)
	require.NoError(t, err)

	require.Equal(t, "prompt-1", first)
	require.Equal(t, first, second)
	require.Equal(t, int32(1), store.userPromptCalls.Load())
}

func TestConfigCache_InvalidateUserPrompt(t *testing.T) {
	t.Parallel()

	ctx, _, store, cache := setupCache(t, 0)
	userID := uuid.New()
	store.getUserChatCustomPrompt = func(context.Context, uuid.UUID) (string, error) {
		call := store.userPromptCalls.Load()
		return fmt.Sprintf("prompt-%d", call), nil
	}

	first, err := cache.UserPrompt(ctx, userID)
	require.NoError(t, err)
	cache.InvalidateUserPrompt(userID)
	second, err := cache.UserPrompt(ctx, userID)
	require.NoError(t, err)

	require.NotEqual(t, first, second)
	require.Equal(t, int32(2), store.userPromptCalls.Load())
}

func TestConfigCache_InvalidateUserPrompt_BlocksStaleInFlightPrompt(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	clock := quartz.NewMock(t)
	userID := uuid.New()
	blocked := []blockedCall[string]{newBlockedCall("stale prompt"), newBlockedCall("fresh prompt")}
	store := &stubChatConfigStore{}
	store.getUserChatCustomPrompt = func(context.Context, uuid.UUID) (string, error) {
		switch call := store.userPromptCalls.Load(); call {
		case 1, 2:
			fetch := blocked[call-1]
			close(fetch.started)
			<-fetch.release
			return fetch.value, nil
		default:
			return "", xerrors.Errorf("unexpected user prompt call %d", call)
		}
	}
	cache := newChatConfigCache(ctx, store, clock)

	runBlockedStaleWriteTest(
		t,
		blocked,
		func() (string, error) { return cache.UserPrompt(ctx, userID) },
		func() { cache.InvalidateUserPrompt(userID) },
		func(t *testing.T) {
			_, _, ok := cache.userPrompts.Get(userID)
			require.False(t, ok)
		},
		func() int32 { return store.userPromptCalls.Load() },
		2,
	)
}

func TestConfigCache_Singleflight(t *testing.T) {
	t.Parallel()

	ctx, _, store, cache := setupCache(t, testutil.WaitMedium)
	providers := []database.ChatProvider{testChatProvider("provider-a")}
	fetchStarted := make(chan struct{})
	releaseFetch := make(chan struct{})
	var startedOnce sync.Once
	store.getEnabledChatProviders = func(context.Context) ([]database.ChatProvider, error) {
		startedOnce.Do(func() { close(fetchStarted) })
		<-releaseFetch
		return providers, nil
	}

	const callers = 8
	results := make([][]database.ChatProvider, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = cache.EnabledProviders(ctx)
		}(i)
	}

	close(start)
	waitForSignal(t, fetchStarted)
	close(releaseFetch)
	wg.Wait()

	for i := 0; i < callers; i++ {
		require.NoError(t, errs[i])
		require.Equal(t, providers, results[i])
	}
	require.Equal(t, int32(1), store.enabledProvidersCalls.Load())
}

func TestConfigCache_GenerationPreventsStaleWrite(t *testing.T) {
	t.Parallel()

	ctx, _, store, cache := setupCache(t, testutil.WaitMedium)
	firstProviders := []database.ChatProvider{testChatProvider("provider-a")}
	secondProviders := []database.ChatProvider{testChatProvider("provider-b")}
	fetchStarted := make(chan struct{})
	releaseFetch := make(chan struct{})
	var startedOnce sync.Once
	store.getEnabledChatProviders = func(context.Context) ([]database.ChatProvider, error) {
		call := store.enabledProvidersCalls.Load()
		if call == 1 {
			startedOnce.Do(func() { close(fetchStarted) })
			<-releaseFetch
			return firstProviders, nil
		}
		return secondProviders, nil
	}

	resultCh := make(chan []database.ChatProvider, 1)
	errCh := make(chan error, 1)
	go func() {
		providers, err := cache.EnabledProviders(ctx)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- providers
	}()

	waitForSignal(t, fetchStarted)
	cache.InvalidateProviders()
	close(releaseFetch)

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case providers := <-resultCh:
		require.Equal(t, firstProviders, providers)
	case <-time.After(testutil.WaitShort):
		t.Fatal("timed out waiting for in-flight fetch")
	}

	require.Nil(t, cache.providers)
	second, err := cache.EnabledProviders(ctx)
	require.NoError(t, err)
	require.Equal(t, secondProviders, second)
	require.Equal(t, int32(2), store.enabledProvidersCalls.Load())
}

func TestConfigCache_InvalidateProviders(t *testing.T) {
	t.Parallel()

	t.Run("BlocksStaleInFlightProviders", func(t *testing.T) {
		t.Parallel()

		ctx, _, store, cache := setupCache(t, testutil.WaitMedium)
		blocked := []blockedCall[[]database.ChatProvider]{
			newBlockedCall([]database.ChatProvider{testChatProvider("provider-stale")}),
			newBlockedCall([]database.ChatProvider{testChatProvider("provider-fresh")}),
		}
		store.getEnabledChatProviders = func(context.Context) ([]database.ChatProvider, error) {
			switch call := store.enabledProvidersCalls.Load(); call {
			case 1, 2:
				fetch := blocked[call-1]
				close(fetch.started)
				<-fetch.release
				return fetch.value, nil
			default:
				return nil, xerrors.Errorf("unexpected provider call %d", call)
			}
		}

		runBlockedStaleWriteTest(
			t,
			blocked,
			func() ([]database.ChatProvider, error) { return cache.EnabledProviders(ctx) },
			cache.InvalidateProviders,
			func(t *testing.T) { require.Nil(t, cache.providers) },
			func() int32 { return store.enabledProvidersCalls.Load() },
			2,
		)
	})

	t.Run("CascadesToModelConfigs", func(t *testing.T) {
		t.Parallel()

		ctx, _, store, cache := setupCache(t, 0)
		configID := uuid.New()
		store.getChatModelConfigByID = func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
			call := store.modelConfigByIDCalls.Load()
			return testChatModelConfig(configID, fmt.Sprintf("model-%d", call)), nil
		}

		first, err := cache.ModelConfigByID(ctx, configID)
		require.NoError(t, err)
		cache.InvalidateProviders()
		second, err := cache.ModelConfigByID(ctx, configID)
		require.NoError(t, err)

		require.NotEqual(t, first, second)
		require.Equal(t, int32(2), store.modelConfigByIDCalls.Load())
	})

	t.Run("CascadesToDefaultModelConfig", func(t *testing.T) {
		t.Parallel()

		ctx, _, store, cache := setupCache(t, 0)
		store.getDefaultChatModelConfig = func(context.Context) (database.ChatModelConfig, error) {
			call := store.defaultModelConfigCall.Load()
			return testChatModelConfig(uuid.New(), fmt.Sprintf("default-model-%d", call)), nil
		}

		first, err := cache.DefaultModelConfig(ctx)
		require.NoError(t, err)
		cache.InvalidateProviders()
		second, err := cache.DefaultModelConfig(ctx)
		require.NoError(t, err)

		require.NotEqual(t, first, second)
		require.Equal(t, int32(2), store.defaultModelConfigCall.Load())
	})

	t.Run("BlocksStaleInFlightModelConfig", func(t *testing.T) {
		t.Parallel()

		ctx, _, store, cache := setupCache(t, testutil.WaitMedium)
		configID := uuid.New()
		blocked := []blockedCall[database.ChatModelConfig]{
			newBlockedCall(testChatModelConfig(configID, "stale-model")),
			newBlockedCall(testChatModelConfig(configID, "fresh-model")),
		}
		store.getChatModelConfigByID = func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
			switch call := store.modelConfigByIDCalls.Load(); call {
			case 1, 2:
				fetch := blocked[call-1]
				close(fetch.started)
				<-fetch.release
				return fetch.value, nil
			default:
				return database.ChatModelConfig{}, xerrors.Errorf("unexpected model config call %d", call)
			}
		}

		runBlockedStaleWriteTest(
			t,
			blocked,
			func() (database.ChatModelConfig, error) { return cache.ModelConfigByID(ctx, configID) },
			cache.InvalidateProviders,
			func(t *testing.T) {
				_, ok := cache.modelConfigs[configID]
				require.False(t, ok)
			},
			func() int32 { return store.modelConfigByIDCalls.Load() },
			2,
		)
	})
}

func testModelAttachment(modelID, providerConfigID uuid.UUID, provider string, priority int32) database.GetModelProviderConfigsRow {
	return database.GetModelProviderConfigsRow{
		ID:                  uuid.New(),
		ModelConfigID:       modelID,
		ProviderConfigID:    providerConfigID,
		Priority:            priority,
		CreatedAt:           time.Unix(0, 0).UTC(),
		UpdatedAt:           time.Unix(0, 0).UTC(),
		Provider:            provider,
		ProviderDisplayName: provider,
		ProviderEnabled:     true,
	}
}

func testChatProvider(name string) database.ChatProvider {
	return database.ChatProvider{
		ID:          uuid.New(),
		Provider:    name,
		DisplayName: name,
		Enabled:     true,
		CreatedAt:   time.Unix(0, 0).UTC(),
		UpdatedAt:   time.Unix(0, 0).UTC(),
	}
}

func testChatModelConfig(id uuid.UUID, model string) database.ChatModelConfig {
	return database.ChatModelConfig{
		ID:                   id,
		Provider:             "openai",
		Model:                model,
		DisplayName:          model,
		Enabled:              true,
		CreatedAt:            time.Unix(0, 0).UTC(),
		UpdatedAt:            time.Unix(0, 0).UTC(),
		ContextLimit:         128000,
		CompressionThreshold: 64000,
	}
}

func waitForSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(testutil.WaitShort):
		t.Fatal("timed out waiting for signal")
	}
}

type blockedCall[T any] struct {
	started chan struct{}
	release chan struct{}
	value   T
}

func newBlockedCall[T any](value T) blockedCall[T] {
	return blockedCall[T]{
		started: make(chan struct{}),
		release: make(chan struct{}),
		value:   value,
	}
}

type asyncCall[T any] struct {
	value T
	err   error
	done  chan struct{}
}

func startAsyncCall[T any](wg *sync.WaitGroup, call func() (T, error)) *asyncCall[T] {
	result := &asyncCall[T]{done: make(chan struct{})}
	wg.Go(func() {
		defer close(result.done)
		result.value, result.err = call()
	})
	return result
}

func awaitAsyncCall[T any](t *testing.T, result *asyncCall[T]) T {
	t.Helper()
	<-result.done
	require.NoError(t, result.err)
	return result.value
}

func runBlockedStaleWriteTest[T any](
	t *testing.T,
	blocked []blockedCall[T],
	call func() (T, error),
	betweenCalls func(),
	assertCleared func(t *testing.T),
	storeCalls func() int32,
	expectedCalls int32,
) {
	t.Helper()

	var wg sync.WaitGroup
	first := startAsyncCall(&wg, call)
	waitForSignal(t, blocked[0].started)
	betweenCalls()

	second := startAsyncCall(&wg, call)
	waitForSignal(t, blocked[1].started)
	close(blocked[0].release)
	require.Equal(t, blocked[0].value, awaitAsyncCall(t, first))
	assertCleared(t)

	close(blocked[1].release)
	require.Equal(t, blocked[1].value, awaitAsyncCall(t, second))
	wg.Wait()
	require.Equal(t, expectedCalls, storeCalls())

	third, err := call()
	require.NoError(t, err)
	require.Equal(t, blocked[1].value, third)
	require.Equal(t, expectedCalls, storeCalls())
}

// TestConfigCache_CallerCancellation verifies the DoChan-based
// cancellation semantics across all four cache methods:
//   - A canceled caller returns immediately without waiting for the
//     shared fill to complete.
//   - One canceled waiter does not poison other coalesced waiters.
//   - Server context cancellation propagates through the fill.
func TestConfigCache_CallerCancellation(t *testing.T) {
	t.Parallel()

	type cacheMethod struct {
		name string
		// setupBlocked configures the store to block on release.
		// The started channel is closed when the fill enters the
		// store. The release channel unblocks the store.
		setupBlocked func(store *stubChatConfigStore, started, release chan struct{})
		// setupCtxSensitive configures the store to block until
		// its context is canceled (for server-shutdown testing).
		setupCtxSensitive func(store *stubChatConfigStore, started chan struct{})
		// call invokes the cache method under test.
		call func(ctx context.Context, cache *chatConfigCache) error
		// storeCalls returns the number of underlying store calls.
		storeCalls func(store *stubChatConfigStore) int32
	}

	configID := uuid.New()
	userID := uuid.New()

	methods := []cacheMethod{
		{
			name: "EnabledProviders",
			setupBlocked: func(store *stubChatConfigStore, started, release chan struct{}) {
				var once sync.Once
				store.getEnabledChatProviders = func(ctx context.Context) ([]database.ChatProvider, error) {
					once.Do(func() { close(started) })
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-release:
						return []database.ChatProvider{testChatProvider("p")}, nil
					}
				}
			},
			setupCtxSensitive: func(store *stubChatConfigStore, started chan struct{}) {
				var once sync.Once
				store.getEnabledChatProviders = func(ctx context.Context) ([]database.ChatProvider, error) {
					once.Do(func() { close(started) })
					<-ctx.Done()
					return nil, ctx.Err()
				}
			},
			call: func(ctx context.Context, cache *chatConfigCache) error {
				_, err := cache.EnabledProviders(ctx)
				return err
			},
			storeCalls: func(store *stubChatConfigStore) int32 {
				return store.enabledProvidersCalls.Load()
			},
		},
		{
			name: "ModelConfigByID",
			setupBlocked: func(store *stubChatConfigStore, started, release chan struct{}) {
				var once sync.Once
				store.getChatModelConfigByID = func(ctx context.Context, id uuid.UUID) (database.ChatModelConfig, error) {
					once.Do(func() { close(started) })
					select {
					case <-ctx.Done():
						return database.ChatModelConfig{}, ctx.Err()
					case <-release:
						return testChatModelConfig(id, "model"), nil
					}
				}
			},
			setupCtxSensitive: func(store *stubChatConfigStore, started chan struct{}) {
				var once sync.Once
				store.getChatModelConfigByID = func(ctx context.Context, _ uuid.UUID) (database.ChatModelConfig, error) {
					once.Do(func() { close(started) })
					<-ctx.Done()
					return database.ChatModelConfig{}, ctx.Err()
				}
			},
			call: func(ctx context.Context, cache *chatConfigCache) error {
				_, err := cache.ModelConfigByID(ctx, configID)
				return err
			},
			storeCalls: func(store *stubChatConfigStore) int32 {
				return store.modelConfigByIDCalls.Load()
			},
		},
		{
			name: "DefaultModelConfig",
			setupBlocked: func(store *stubChatConfigStore, started, release chan struct{}) {
				var once sync.Once
				store.getDefaultChatModelConfig = func(ctx context.Context) (database.ChatModelConfig, error) {
					once.Do(func() { close(started) })
					select {
					case <-ctx.Done():
						return database.ChatModelConfig{}, ctx.Err()
					case <-release:
						return testChatModelConfig(uuid.New(), "default"), nil
					}
				}
			},
			setupCtxSensitive: func(store *stubChatConfigStore, started chan struct{}) {
				var once sync.Once
				store.getDefaultChatModelConfig = func(ctx context.Context) (database.ChatModelConfig, error) {
					once.Do(func() { close(started) })
					<-ctx.Done()
					return database.ChatModelConfig{}, ctx.Err()
				}
			},
			call: func(ctx context.Context, cache *chatConfigCache) error {
				_, err := cache.DefaultModelConfig(ctx)
				return err
			},
			storeCalls: func(store *stubChatConfigStore) int32 {
				return store.defaultModelConfigCall.Load()
			},
		},
		{
			name: "UserPrompt",
			setupBlocked: func(store *stubChatConfigStore, started, release chan struct{}) {
				var once sync.Once
				store.getUserChatCustomPrompt = func(ctx context.Context, _ uuid.UUID) (string, error) {
					once.Do(func() { close(started) })
					select {
					case <-ctx.Done():
						return "", ctx.Err()
					case <-release:
						return "custom prompt", nil
					}
				}
			},
			setupCtxSensitive: func(store *stubChatConfigStore, started chan struct{}) {
				var once sync.Once
				store.getUserChatCustomPrompt = func(ctx context.Context, _ uuid.UUID) (string, error) {
					once.Do(func() { close(started) })
					<-ctx.Done()
					return "", ctx.Err()
				}
			},
			call: func(ctx context.Context, cache *chatConfigCache) error {
				_, err := cache.UserPrompt(ctx, userID)
				return err
			},
			storeCalls: func(store *stubChatConfigStore) int32 {
				return store.userPromptCalls.Load()
			},
		},
	}

	// Test A: A canceled caller stops waiting immediately; the
	// shared fill still completes and populates the cache.
	t.Run("CanceledCallerStopsWaiting", func(t *testing.T) {
		t.Parallel()
		for _, m := range methods {
			t.Run(m.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitMedium)
				clock := quartz.NewMock(t)
				store := &stubChatConfigStore{}
				started := make(chan struct{})
				release := make(chan struct{})
				m.setupBlocked(store, started, release)
				cache := newChatConfigCache(ctx, store, clock)

				callerCtx, callerCancel := context.WithCancel(ctx)
				errCh := make(chan error, 1)
				go func() {
					errCh <- m.call(callerCtx, cache)
				}()

				// Wait for the fill to enter the store, then
				// cancel the caller's context.
				waitForSignal(t, started)
				callerCancel()

				select {
				case err := <-errCh:
					require.ErrorIs(t, err, context.Canceled)
				case <-time.After(testutil.WaitShort):
					t.Fatal("canceled caller did not return promptly")
				}

				// Release the store so the fill can complete.
				close(release)

				// A fresh call must succeed — either a cache
				// hit or by joining the still-in-flight fill.
				// Only one store call should have occurred.
				require.NoError(t, m.call(ctx, cache))
				require.Equal(t, int32(1), m.storeCalls(store))
			})
		}
	})

	// Test B: One canceled waiter does not poison other coalesced
	// waiters sharing the same singleflight entry.
	t.Run("CanceledWaiterDoesNotPoisonOthers", func(t *testing.T) {
		t.Parallel()
		for _, m := range methods {
			t.Run(m.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitMedium)
				clock := quartz.NewMock(t)
				store := &stubChatConfigStore{}
				started := make(chan struct{})
				release := make(chan struct{})
				m.setupBlocked(store, started, release)
				cache := newChatConfigCache(ctx, store, clock)

				cancelCtx, cancel := context.WithCancel(ctx)
				cancelErrCh := make(chan error, 1)
				survivorErrCh := make(chan error, 1)

				go func() {
					cancelErrCh <- m.call(cancelCtx, cache)
				}()
				go func() {
					survivorErrCh <- m.call(ctx, cache)
				}()

				waitForSignal(t, started)
				cancel()

				select {
				case err := <-cancelErrCh:
					require.ErrorIs(t, err, context.Canceled)
				case <-time.After(testutil.WaitShort):
					t.Fatal("canceled caller did not return promptly")
				}

				// Release the store; the surviving waiter
				// must receive the successful result.
				close(release)

				select {
				case err := <-survivorErrCh:
					require.NoError(t, err)
				case <-time.After(testutil.WaitShort):
					t.Fatal("survivor caller did not return")
				}

				require.Equal(t, int32(1), m.storeCalls(store))
			})
		}
	})

	// Test C: Server context cancellation propagates through the
	// fill, ensuring graceful shutdown behavior is preserved.
	t.Run("ServerCancellation", func(t *testing.T) {
		t.Parallel()
		for _, m := range methods {
			t.Run(m.name, func(t *testing.T) {
				t.Parallel()
				clock := quartz.NewMock(t)
				store := &stubChatConfigStore{}
				started := make(chan struct{})
				m.setupCtxSensitive(store, started)

				serverCtx, serverCancel := context.WithCancel(context.Background())
				defer serverCancel()
				cache := newChatConfigCache(serverCtx, store, clock)

				callerCtx := testutil.Context(t, testutil.WaitMedium)
				errCh := make(chan error, 1)
				go func() {
					errCh <- m.call(callerCtx, cache)
				}()

				waitForSignal(t, started)
				serverCancel()

				select {
				case err := <-errCh:
					require.ErrorIs(t, err, context.Canceled)
				case <-time.After(testutil.WaitShort):
					t.Fatal("caller did not return after server cancel")
				}
			})
		}
	})
}
