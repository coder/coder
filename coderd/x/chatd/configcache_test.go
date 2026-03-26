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
	getUserChatCustomPrompt   func(context.Context, uuid.UUID) (string, error)

	enabledProvidersCalls  atomic.Int32
	modelConfigByIDCalls   atomic.Int32
	defaultModelConfigCall atomic.Int32
	userPromptCalls        atomic.Int32
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

func (s *stubChatConfigStore) GetUserChatCustomPrompt(ctx context.Context, userID uuid.UUID) (string, error) {
	s.userPromptCalls.Add(1)
	if s.getUserChatCustomPrompt == nil {
		panic("unexpected GetUserChatCustomPrompt call")
	}
	return s.getUserChatCustomPrompt(ctx, userID)
}

func TestConfigCache_EnabledProviders_CacheHit(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	providers := []database.ChatProvider{testChatProvider("provider-a")}
	store := &stubChatConfigStore{
		getEnabledChatProviders: func(context.Context) ([]database.ChatProvider, error) {
			return providers, nil
		},
	}
	cache := newChatConfigCache(ctx, store, clock)

	first, err := cache.EnabledProviders(ctx)
	require.NoError(t, err)
	second, err := cache.EnabledProviders(ctx)
	require.NoError(t, err)

	require.Equal(t, providers, first)
	require.Equal(t, providers, second)
	require.Equal(t, int32(1), store.enabledProvidersCalls.Load())
}

func TestConfigCache_EnabledProviders_TTLExpiry(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	store := &stubChatConfigStore{}
	store.getEnabledChatProviders = func(context.Context) ([]database.ChatProvider, error) {
		call := store.enabledProvidersCalls.Load()
		return []database.ChatProvider{testChatProvider(fmt.Sprintf("provider-%d", call))}, nil
	}
	cache := newChatConfigCache(ctx, store, clock)

	first, err := cache.EnabledProviders(ctx)
	require.NoError(t, err)
	clock.Advance(chatConfigProvidersTTL).MustWait(ctx)
	second, err := cache.EnabledProviders(ctx)
	require.NoError(t, err)

	require.NotEqual(t, first, second)
	require.Equal(t, int32(2), store.enabledProvidersCalls.Load())
}

func TestConfigCache_EnabledProviders_Invalidation(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	store := &stubChatConfigStore{}
	store.getEnabledChatProviders = func(context.Context) ([]database.ChatProvider, error) {
		call := store.enabledProvidersCalls.Load()
		return []database.ChatProvider{testChatProvider(fmt.Sprintf("provider-%d", call))}, nil
	}
	cache := newChatConfigCache(ctx, store, clock)

	first, err := cache.EnabledProviders(ctx)
	require.NoError(t, err)
	cache.InvalidateProviders()
	second, err := cache.EnabledProviders(ctx)
	require.NoError(t, err)

	require.NotEqual(t, first, second)
	require.Equal(t, int32(2), store.enabledProvidersCalls.Load())
}

func TestConfigCache_ModelConfigByID_CacheHit(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	configID := uuid.New()
	config := testChatModelConfig(configID, "model-a")
	store := &stubChatConfigStore{
		getChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
			return config, nil
		},
	}
	cache := newChatConfigCache(ctx, store, clock)

	first, err := cache.ModelConfigByID(ctx, configID)
	require.NoError(t, err)
	second, err := cache.ModelConfigByID(ctx, configID)
	require.NoError(t, err)

	require.Equal(t, config, first)
	require.Equal(t, config, second)
	require.Equal(t, int32(1), store.modelConfigByIDCalls.Load())
}

func TestConfigCache_ModelConfigByID_ClonesOptionsForCache(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	configID := uuid.New()
	const options = `{"temperature":0.1}`
	config := testChatModelConfig(configID, "model-a")
	config.Options = []byte(options)
	store := &stubChatConfigStore{
		getChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
			return config, nil
		},
	}
	cache := newChatConfigCache(ctx, store, clock)

	// First call populates cache via singleflight.
	first, err := cache.ModelConfigByID(ctx, configID)
	require.NoError(t, err)
	first.Options[0] = 'x' // mutate singleflight return

	// Second call is a cache hit.
	second, err := cache.ModelConfigByID(ctx, configID)
	require.NoError(t, err)
	require.Equal(t, options, string(second.Options))
	second.Options[0] = 'y' // mutate cache-hit return

	// Third call is another cache hit — must be unaffected.
	third, err := cache.ModelConfigByID(ctx, configID)
	require.NoError(t, err)
	require.Equal(t, options, string(third.Options))
}

func TestConfigCache_ModelConfigByID_NotFound(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	configID := uuid.New()
	store := &stubChatConfigStore{
		getChatModelConfigByID: func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
			return database.ChatModelConfig{}, sql.ErrNoRows
		},
	}
	cache := newChatConfigCache(ctx, store, clock)

	_, err := cache.ModelConfigByID(ctx, configID)
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = cache.ModelConfigByID(ctx, configID)
	require.ErrorIs(t, err, sql.ErrNoRows)

	require.Equal(t, int32(2), store.modelConfigByIDCalls.Load())
	_, ok := cache.modelConfigs[configID]
	require.False(t, ok)
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

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	userID := uuid.New()
	store := &stubChatConfigStore{}
	store.getUserChatCustomPrompt = func(context.Context, uuid.UUID) (string, error) {
		call := store.userPromptCalls.Load()
		return fmt.Sprintf("prompt-%d", call), nil
	}
	cache := newChatConfigCache(ctx, store, clock)
	cache.userPrompts.Set(userID, "stale", 0)

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

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	userID := uuid.New()
	store := &stubChatConfigStore{}
	store.getUserChatCustomPrompt = func(context.Context, uuid.UUID) (string, error) {
		call := store.userPromptCalls.Load()
		return fmt.Sprintf("prompt-%d", call), nil
	}
	cache := newChatConfigCache(ctx, store, clock)

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
	const stalePrompt = "stale prompt"
	const freshPrompt = "fresh prompt"
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	releaseSecond := make(chan struct{})
	store := &stubChatConfigStore{}
	store.getUserChatCustomPrompt = func(context.Context, uuid.UUID) (string, error) {
		switch call := store.userPromptCalls.Load(); call {
		case 1:
			close(firstStarted)
			<-releaseFirst
			return stalePrompt, nil
		case 2:
			close(secondStarted)
			<-releaseSecond
			return freshPrompt, nil
		default:
			return "", xerrors.Errorf("unexpected user prompt call %d", call)
		}
	}
	cache := newChatConfigCache(ctx, store, clock)

	type result struct {
		prompt string
		err    error
	}

	firstResult := make(chan result, 1)
	go func() {
		prompt, err := cache.UserPrompt(ctx, userID)
		firstResult <- result{prompt: prompt, err: err}
	}()

	waitForSignal(t, firstStarted)
	cache.InvalidateUserPrompt(userID)

	secondResult := make(chan result, 1)
	go func() {
		prompt, err := cache.UserPrompt(ctx, userID)
		secondResult <- result{prompt: prompt, err: err}
	}()

	waitForSignal(t, secondStarted)
	close(releaseFirst)
	first := <-firstResult
	require.NoError(t, first.err)
	require.Equal(t, stalePrompt, first.prompt)
	_, _, ok := cache.userPrompts.Get(userID)
	require.False(t, ok)

	close(releaseSecond)
	second := <-secondResult
	require.NoError(t, second.err)
	require.Equal(t, freshPrompt, second.prompt)
	require.Equal(t, int32(2), store.userPromptCalls.Load())

	third, err := cache.UserPrompt(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, freshPrompt, third)
	require.Equal(t, int32(2), store.userPromptCalls.Load())
}

func TestConfigCache_Singleflight(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	clock := quartz.NewMock(t)
	providers := []database.ChatProvider{testChatProvider("provider-a")}
	fetchStarted := make(chan struct{})
	releaseFetch := make(chan struct{})
	var startedOnce sync.Once
	store := &stubChatConfigStore{}
	store.getEnabledChatProviders = func(context.Context) ([]database.ChatProvider, error) {
		startedOnce.Do(func() { close(fetchStarted) })
		<-releaseFetch
		return providers, nil
	}
	cache := newChatConfigCache(ctx, store, clock)

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

	ctx := testutil.Context(t, testutil.WaitMedium)
	clock := quartz.NewMock(t)
	firstProviders := []database.ChatProvider{testChatProvider("provider-a")}
	secondProviders := []database.ChatProvider{testChatProvider("provider-b")}
	fetchStarted := make(chan struct{})
	releaseFetch := make(chan struct{})
	var startedOnce sync.Once
	store := &stubChatConfigStore{}
	store.getEnabledChatProviders = func(context.Context) ([]database.ChatProvider, error) {
		call := store.enabledProvidersCalls.Load()
		if call == 1 {
			startedOnce.Do(func() { close(fetchStarted) })
			<-releaseFetch
			return firstProviders, nil
		}
		return secondProviders, nil
	}
	cache := newChatConfigCache(ctx, store, clock)

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

func TestConfigCache_InvalidateProviders_BlocksStaleInFlightProviders(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	clock := quartz.NewMock(t)
	staleProviders := []database.ChatProvider{testChatProvider("provider-stale")}
	freshProviders := []database.ChatProvider{testChatProvider("provider-fresh")}
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	releaseSecond := make(chan struct{})
	store := &stubChatConfigStore{}
	store.getEnabledChatProviders = func(context.Context) ([]database.ChatProvider, error) {
		switch call := store.enabledProvidersCalls.Load(); call {
		case 1:
			close(firstStarted)
			<-releaseFirst
			return staleProviders, nil
		case 2:
			close(secondStarted)
			<-releaseSecond
			return freshProviders, nil
		default:
			return nil, xerrors.Errorf("unexpected provider call %d", call)
		}
	}
	cache := newChatConfigCache(ctx, store, clock)

	type result struct {
		providers []database.ChatProvider
		err       error
	}

	firstResult := make(chan result, 1)
	go func() {
		providers, err := cache.EnabledProviders(ctx)
		firstResult <- result{providers: providers, err: err}
	}()

	waitForSignal(t, firstStarted)
	cache.InvalidateProviders()

	secondResult := make(chan result, 1)
	go func() {
		providers, err := cache.EnabledProviders(ctx)
		secondResult <- result{providers: providers, err: err}
	}()

	waitForSignal(t, secondStarted)
	close(releaseFirst)
	first := <-firstResult
	require.NoError(t, first.err)
	require.Equal(t, staleProviders, first.providers)
	require.Nil(t, cache.providers)

	close(releaseSecond)
	second := <-secondResult
	require.NoError(t, second.err)
	require.Equal(t, freshProviders, second.providers)
	require.Equal(t, int32(2), store.enabledProvidersCalls.Load())

	third, err := cache.EnabledProviders(ctx)
	require.NoError(t, err)
	require.Equal(t, freshProviders, third)
	require.Equal(t, int32(2), store.enabledProvidersCalls.Load())
}

func TestConfigCache_InvalidateProviders_CascadesToModelConfigs(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	configID := uuid.New()
	store := &stubChatConfigStore{}
	store.getChatModelConfigByID = func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
		call := store.modelConfigByIDCalls.Load()
		return testChatModelConfig(configID, fmt.Sprintf("model-%d", call)), nil
	}
	cache := newChatConfigCache(ctx, store, clock)

	first, err := cache.ModelConfigByID(ctx, configID)
	require.NoError(t, err)
	cache.InvalidateProviders()
	second, err := cache.ModelConfigByID(ctx, configID)
	require.NoError(t, err)

	require.NotEqual(t, first, second)
	require.Equal(t, int32(2), store.modelConfigByIDCalls.Load())
}

func TestConfigCache_InvalidateProviders_CascadesToDefaultModelConfig(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	store := &stubChatConfigStore{}
	store.getDefaultChatModelConfig = func(context.Context) (database.ChatModelConfig, error) {
		call := store.defaultModelConfigCall.Load()
		return testChatModelConfig(uuid.New(), fmt.Sprintf("default-model-%d", call)), nil
	}
	cache := newChatConfigCache(ctx, store, clock)

	first, err := cache.DefaultModelConfig(ctx)
	require.NoError(t, err)
	cache.InvalidateProviders()
	second, err := cache.DefaultModelConfig(ctx)
	require.NoError(t, err)

	require.NotEqual(t, first, second)
	require.Equal(t, int32(2), store.defaultModelConfigCall.Load())
}

func TestConfigCache_InvalidateProviders_BlocksStaleInFlightModelConfig(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	clock := quartz.NewMock(t)
	configID := uuid.New()
	staleConfig := testChatModelConfig(configID, "stale-model")
	freshConfig := testChatModelConfig(configID, "fresh-model")
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	releaseSecond := make(chan struct{})
	store := &stubChatConfigStore{}
	store.getChatModelConfigByID = func(context.Context, uuid.UUID) (database.ChatModelConfig, error) {
		switch call := store.modelConfigByIDCalls.Load(); call {
		case 1:
			close(firstStarted)
			<-releaseFirst
			return staleConfig, nil
		case 2:
			close(secondStarted)
			<-releaseSecond
			return freshConfig, nil
		default:
			return database.ChatModelConfig{}, xerrors.Errorf("unexpected model config call %d", call)
		}
	}
	cache := newChatConfigCache(ctx, store, clock)

	type result struct {
		config database.ChatModelConfig
		err    error
	}

	firstResult := make(chan result, 1)
	go func() {
		config, err := cache.ModelConfigByID(ctx, configID)
		firstResult <- result{config: config, err: err}
	}()

	waitForSignal(t, firstStarted)
	cache.InvalidateProviders()

	secondResult := make(chan result, 1)
	go func() {
		config, err := cache.ModelConfigByID(ctx, configID)
		secondResult <- result{config: config, err: err}
	}()

	waitForSignal(t, secondStarted)
	close(releaseFirst)
	first := <-firstResult
	require.NoError(t, first.err)
	require.Equal(t, staleConfig, first.config)
	_, ok := cache.modelConfigs[configID]
	require.False(t, ok)

	close(releaseSecond)
	second := <-secondResult
	require.NoError(t, second.err)
	require.Equal(t, freshConfig, second.config)
	require.Equal(t, int32(2), store.modelConfigByIDCalls.Load())

	third, err := cache.ModelConfigByID(ctx, configID)
	require.NoError(t, err)
	require.Equal(t, freshConfig, third)
	require.Equal(t, int32(2), store.modelConfigByIDCalls.Load())
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
