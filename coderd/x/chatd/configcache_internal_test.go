package chatd

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
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type stubChatConfigStore struct {
	database.Store

	getAIProviders          func(context.Context) ([]database.AIProvider, error)
	getUserChatCustomPrompt func(context.Context, uuid.UUID) (string, error)
	getChatAdvisorConfig    func(context.Context) (string, error)

	enabledProvidersCalls atomic.Int32
	userPromptCalls       atomic.Int32
	advisorConfigCalls    atomic.Int32
}

func (s *stubChatConfigStore) GetAIProviders(ctx context.Context, _ database.GetAIProvidersParams) ([]database.AIProvider, error) {
	s.enabledProvidersCalls.Add(1)
	if s.getAIProviders == nil {
		panic("unexpected GetAIProviders call")
	}
	return s.getAIProviders(ctx)
}

func (s *stubChatConfigStore) GetUserChatCustomPrompt(ctx context.Context, userID uuid.UUID) (string, error) {
	s.userPromptCalls.Add(1)
	if s.getUserChatCustomPrompt == nil {
		panic("unexpected GetUserChatCustomPrompt call")
	}
	return s.getUserChatCustomPrompt(ctx, userID)
}

func (s *stubChatConfigStore) GetChatAdvisorConfig(ctx context.Context) (string, error) {
	s.advisorConfigCalls.Add(1)
	if s.getChatAdvisorConfig == nil {
		panic("unexpected GetChatAdvisorConfig call")
	}
	return s.getChatAdvisorConfig(ctx)
}

func TestConfigCache_EnabledProviders_CacheHit(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	providers := []database.AIProvider{testAIProvider("provider-a")}
	store := &stubChatConfigStore{
		getAIProviders: func(context.Context) ([]database.AIProvider, error) {
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
	store.getAIProviders = func(context.Context) ([]database.AIProvider, error) {
		call := store.enabledProvidersCalls.Load()
		return []database.AIProvider{testAIProvider(fmt.Sprintf("provider-%d", call))}, nil
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
	store.getAIProviders = func(context.Context) ([]database.AIProvider, error) {
		call := store.enabledProvidersCalls.Load()
		return []database.AIProvider{testAIProvider(fmt.Sprintf("provider-%d", call))}, nil
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
	providers := []database.AIProvider{testAIProvider("provider-a")}
	fetchStarted := make(chan struct{})
	releaseFetch := make(chan struct{})
	var startedOnce sync.Once
	store := &stubChatConfigStore{}
	store.getAIProviders = func(context.Context) ([]database.AIProvider, error) {
		startedOnce.Do(func() { close(fetchStarted) })
		<-releaseFetch
		return providers, nil
	}
	cache := newChatConfigCache(ctx, store, clock)

	const callers = 8
	results := make([][]database.AIProvider, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < callers; i++ {
		wg.Go(func() {
			<-start
			results[i], errs[i] = cache.EnabledProviders(ctx)
		})
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
	firstProviders := []database.AIProvider{testAIProvider("provider-a")}
	secondProviders := []database.AIProvider{testAIProvider("provider-b")}
	fetchStarted := make(chan struct{})
	releaseFetch := make(chan struct{})
	var startedOnce sync.Once
	store := &stubChatConfigStore{}
	store.getAIProviders = func(context.Context) ([]database.AIProvider, error) {
		call := store.enabledProvidersCalls.Load()
		if call == 1 {
			startedOnce.Do(func() { close(fetchStarted) })
			<-releaseFetch
			return firstProviders, nil
		}
		return secondProviders, nil
	}
	cache := newChatConfigCache(ctx, store, clock)

	resultCh := make(chan []database.AIProvider, 1)
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
	staleProviders := []database.AIProvider{testAIProvider("provider-stale")}
	freshProviders := []database.AIProvider{testAIProvider("provider-fresh")}
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	releaseSecond := make(chan struct{})
	store := &stubChatConfigStore{}
	store.getAIProviders = func(context.Context) ([]database.AIProvider, error) {
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
		providers []database.AIProvider
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

func testAIProvider(name string) database.AIProvider {
	return database.AIProvider{
		ID:          uuid.New(),
		Type:        database.AIProviderType(name),
		Name:        name,
		DisplayName: sql.NullString{String: name, Valid: true},
		Enabled:     true,
		CreatedAt:   time.Unix(0, 0).UTC(),
		UpdatedAt:   time.Unix(0, 0).UTC(),
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

// TestConfigCache_CallerCancellation verifies the DoChan-based cancellation semantics:
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

	userID := uuid.New()

	methods := []cacheMethod{
		{
			name: "EnabledProviders",
			setupBlocked: func(store *stubChatConfigStore, started, release chan struct{}) {
				var once sync.Once
				store.getAIProviders = func(ctx context.Context) ([]database.AIProvider, error) {
					once.Do(func() { close(started) })
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-release:
						return []database.AIProvider{testAIProvider("p")}, nil
					}
				}
			},
			setupCtxSensitive: func(store *stubChatConfigStore, started chan struct{}) {
				var once sync.Once
				store.getAIProviders = func(ctx context.Context) ([]database.AIProvider, error) {
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

func TestConfigCache_AdvisorConfig_CacheHit(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	const raw = `{"enabled":true,"max_uses_per_run":3,"max_output_tokens":16384}`
	store := &stubChatConfigStore{
		getChatAdvisorConfig: func(context.Context) (string, error) {
			return raw, nil
		},
	}
	cache := newChatConfigCache(ctx, store, clock)

	first, err := cache.AdvisorConfig(ctx)
	require.NoError(t, err)
	second, err := cache.AdvisorConfig(ctx)
	require.NoError(t, err)

	require.True(t, first.Enabled)
	require.Equal(t, 3, first.MaxUsesPerRun)
	require.Equal(t, int64(16384), first.MaxOutputTokens)
	require.Equal(t, first, second)
	require.Equal(t, int32(1), store.advisorConfigCalls.Load(),
		"second lookup must be served from cache")
}

func TestConfigCache_AdvisorConfig_TTLExpiry(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	store := &stubChatConfigStore{}
	store.getChatAdvisorConfig = func(context.Context) (string, error) {
		call := store.advisorConfigCalls.Load()
		return fmt.Sprintf(`{"max_uses_per_run":%d}`, call), nil
	}
	cache := newChatConfigCache(ctx, store, clock)

	first, err := cache.AdvisorConfig(ctx)
	require.NoError(t, err)
	clock.Advance(chatConfigAdvisorConfigTTL).MustWait(ctx)
	second, err := cache.AdvisorConfig(ctx)
	require.NoError(t, err)

	require.NotEqual(t, first.MaxUsesPerRun, second.MaxUsesPerRun,
		"TTL expiry must trigger a refetch")
	require.Equal(t, int32(2), store.advisorConfigCalls.Load())
}

func TestConfigCache_AdvisorConfig_DBErrorNotCached(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	expected := xerrors.New("boom")
	store := &stubChatConfigStore{
		getChatAdvisorConfig: func(context.Context) (string, error) {
			return "", expected
		},
	}
	cache := newChatConfigCache(ctx, store, clock)

	_, err := cache.AdvisorConfig(ctx)
	require.ErrorIs(t, err, expected)
	_, err = cache.AdvisorConfig(ctx)
	require.ErrorIs(t, err, expected)

	require.Equal(t, int32(2), store.advisorConfigCalls.Load(),
		"errors must not populate the cache; every call retries")
}

func TestConfigCache_AdvisorConfig_InvalidJSONNotCached(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	store := &stubChatConfigStore{
		getChatAdvisorConfig: func(context.Context) (string, error) {
			return "not valid json", nil
		},
	}
	cache := newChatConfigCache(ctx, store, clock)

	_, err := cache.AdvisorConfig(ctx)
	require.Error(t, err, "malformed JSON must surface as an error")
	_, err = cache.AdvisorConfig(ctx)
	require.Error(t, err)

	require.Equal(t, int32(2), store.advisorConfigCalls.Load(),
		"parse errors must not populate the cache; every call retries")
}

func TestConfigCache_AdvisorConfig_EmptyJSONYieldsZeroValue(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	// GetChatAdvisorConfig returns "{}" when the site-config row is
	// absent. That must unmarshal to a zero-value AdvisorConfig rather
	// than a parse error.
	store := &stubChatConfigStore{
		getChatAdvisorConfig: func(context.Context) (string, error) {
			return "{}", nil
		},
	}
	cache := newChatConfigCache(ctx, store, clock)

	cfg, err := cache.AdvisorConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, codersdk.AdvisorConfig{}, cfg)
}

// Guards the pubsub-driven invalidation path. Without this, an admin
// writing PUT /api/experimental/chats/config/advisor could keep every
// replica serving stale enabled/model/limits for up to
// chatConfigAdvisorConfigTTL, which defeats the subscriber in chatd.go.
func TestConfigCache_InvalidateAdvisorConfig(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	clock := quartz.NewMock(t)
	store := &stubChatConfigStore{}
	store.getChatAdvisorConfig = func(context.Context) (string, error) {
		call := store.advisorConfigCalls.Load()
		return fmt.Sprintf(`{"max_uses_per_run":%d}`, call), nil
	}
	cache := newChatConfigCache(ctx, store, clock)

	first, err := cache.AdvisorConfig(ctx)
	require.NoError(t, err)

	cache.InvalidateAdvisorConfig()

	second, err := cache.AdvisorConfig(ctx)
	require.NoError(t, err)

	require.NotEqual(t, first.MaxUsesPerRun, second.MaxUsesPerRun,
		"invalidation must force a refetch without waiting for TTL expiry")
	require.Equal(t, int32(2), store.advisorConfigCalls.Load())
}

// Guards against the invalidation-during-singleflight race. A stale
// in-flight fill started before InvalidateAdvisorConfig must not
// re-cache its pre-update value, which would defeat the pubsub
// invalidation path for up to chatConfigAdvisorConfigTTL.
func TestConfigCache_InvalidateAdvisorConfig_BlocksStaleInFlight(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitMedium)
	clock := quartz.NewMock(t)
	staleConfig := `{"max_uses_per_run":1}`
	freshConfig := `{"max_uses_per_run":2}`
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	releaseSecond := make(chan struct{})
	store := &stubChatConfigStore{}
	store.getChatAdvisorConfig = func(context.Context) (string, error) {
		switch call := store.advisorConfigCalls.Load(); call {
		case 1:
			close(firstStarted)
			<-releaseFirst
			return staleConfig, nil
		case 2:
			close(secondStarted)
			<-releaseSecond
			return freshConfig, nil
		default:
			return "", xerrors.Errorf("unexpected advisor config call %d", call)
		}
	}
	cache := newChatConfigCache(ctx, store, clock)

	type result struct {
		config codersdk.AdvisorConfig
		err    error
	}

	firstResult := make(chan result, 1)
	go func() {
		config, err := cache.AdvisorConfig(ctx)
		firstResult <- result{config: config, err: err}
	}()

	waitForSignal(t, firstStarted)
	cache.InvalidateAdvisorConfig()

	secondResult := make(chan result, 1)
	go func() {
		config, err := cache.AdvisorConfig(ctx)
		secondResult <- result{config: config, err: err}
	}()

	waitForSignal(t, secondStarted)
	close(releaseFirst)
	first := <-firstResult
	require.NoError(t, first.err)
	require.EqualValues(t, 1, first.config.MaxUsesPerRun)
	require.Nil(t, cache.advisorConfig,
		"stale fill must not re-cache after invalidation")

	close(releaseSecond)
	second := <-secondResult
	require.NoError(t, second.err)
	require.EqualValues(t, 2, second.config.MaxUsesPerRun)
	require.Equal(t, int32(2), store.advisorConfigCalls.Load())

	third, err := cache.AdvisorConfig(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 2, third.MaxUsesPerRun)
	require.Equal(t, int32(2), store.advisorConfigCalls.Load())
}

func TestConfigCache_InvalidatesProvidersOnAIProvidersChangedEvent(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	// The generation counter only advances through InvalidateProviders,
	// so this cannot false-pass via TTL expiry.
	gen := server.configCache.providersGeneration()
	require.NoError(t, ps.Publish(coderdpubsub.AIProvidersChangedChannel, nil))

	require.Eventually(t, func() bool {
		return server.configCache.providersGeneration() > gen
	}, testutil.WaitShort, testutil.IntervalFast)
}
