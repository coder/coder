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
	cache := newChatConfigCache(store, clock)

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
	cache := newChatConfigCache(store, clock)

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
	cache := newChatConfigCache(store, clock)

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
	cache := newChatConfigCache(store, clock)

	first, err := cache.ModelConfigByID(ctx, configID)
	require.NoError(t, err)
	second, err := cache.ModelConfigByID(ctx, configID)
	require.NoError(t, err)

	require.Equal(t, config, first)
	require.Equal(t, config, second)
	require.Equal(t, int32(1), store.modelConfigByIDCalls.Load())
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
	cache := newChatConfigCache(store, clock)

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
	cache := newChatConfigCache(store, clock)

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

func TestConfigCache_UserPrompt_ShorterTTL(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	userID := uuid.New()
	store := &stubChatConfigStore{}
	store.getUserChatCustomPrompt = func(context.Context, uuid.UUID) (string, error) {
		call := store.userPromptCalls.Load()
		return fmt.Sprintf("prompt-%d", call), nil
	}
	cache := newChatConfigCache(store, clock)

	first, err := cache.UserPrompt(ctx, userID)
	require.NoError(t, err)
	clock.Advance(chatConfigUserPromptTTL + time.Second).MustWait(ctx)
	second, err := cache.UserPrompt(ctx, userID)
	require.NoError(t, err)

	require.NotEqual(t, first, second)
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
	cache := newChatConfigCache(store, clock)

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
	cache := newChatConfigCache(store, clock)

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
