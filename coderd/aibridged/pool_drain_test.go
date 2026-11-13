package aibridged_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/mcpmock"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/testutil"
)

func TestPoolDrain(t *testing.T) {
	t.Parallel()

	const firstEvent = "data: partial\n\n"
	const lastEvent = "data: [DONE]\n\n"
	// Allow for Ristretto's unexported expiration bucket and cleanup ticker
	// intervals, which together add up to 7.5 seconds of cleanup lag:
	// https://github.com/dgraph-io/ristretto/blob/v2.4.0/ttl.go#L13-L25
	// https://github.com/dgraph-io/ristretto/blob/v2.4.0/cache.go#L219-L233
	const cacheCleanupAllowance = 10 * time.Second
	tests := []struct {
		name         string
		expireBridge bool
		shutdownPool bool
		blockAcquire bool
		// shutdownCtxTimeout bounds the context passed to pool shutdown. Zero
		// passes a context without a deadline.
		shutdownCtxTimeout time.Duration
		aliveFor           time.Duration
		cancelWithin       time.Duration // Zero completes the stream instead of waiting for cancellation.
		wantBody           string
		wantWarning        bool
	}{
		{
			name:         "Complete",
			expireBridge: true,
			wantBody:     firstEvent + lastEvent,
		},
		{
			name:         "DrainTimeout",
			expireBridge: true,
			// Cleanup may start the drain anywhere within the allowance.
			// Check just before the earliest and just after the latest deadline.
			aliveFor:     aibridged.BridgeDrainTimeoutForTest - cacheCleanupAllowance - time.Millisecond,
			cancelWithin: cacheCleanupAllowance + 2*time.Millisecond,
			wantBody:     firstEvent,
			wantWarning:  true,
		},
		{
			name:         "ShutdownRetired",
			expireBridge: true,
			shutdownPool: true,
			aliveFor:     aibridged.PoolShutdownTimeoutForTest - time.Millisecond,
			cancelWithin: 2 * time.Millisecond,
			wantBody:     firstEvent,
		},
		{
			name:         "ShutdownCached",
			shutdownPool: true,
			aliveFor:     aibridged.PoolShutdownTimeoutForTest - time.Millisecond,
			cancelWithin: 2 * time.Millisecond,
			wantBody:     firstEvent,
		},
		{
			name:               "ShutdownContext",
			expireBridge:       true,
			shutdownPool:       true,
			shutdownCtxTimeout: time.Second,
			aliveFor:           time.Second - time.Millisecond,
			cancelWithin:       2 * time.Millisecond,
			wantBody:           firstEvent,
		},
		{
			name:         "ShutdownBlockedAcquire",
			expireBridge: true,
			shutdownPool: true,
			blockAcquire: true,
			aliveFor:     aibridged.PoolShutdownTimeoutForTest - time.Millisecond,
			cancelWithin: 2 * time.Millisecond,
			wantBody:     firstEvent,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Ristretto uses the standard clock internally. A synctest bubble
			// advances both its TTL cleanup and the bridge drain deadlines.
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				started := make(chan context.Context, 1)
				complete := make(chan struct{})
				transport := testutil.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
					if strings.HasSuffix(r.URL.Path, "/models") {
						return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("fresh"))}, nil
					}
					reader, writer := io.Pipe()
					go func() {
						defer writer.Close()
						if _, err := io.WriteString(writer, firstEvent); err != nil {
							return
						}
						started <- r.Context()
						select {
						case <-complete:
							_, _ = io.WriteString(writer, lastEvent)
						case <-r.Context().Done():
							_ = writer.CloseWithError(r.Context().Err())
						}
					}()
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": {"text/event-stream"}},
						Body:       reader,
					}, nil
				})
				prov := &drainTestProvider{
					Provider:  aibridge.NewOpenAIProvider(config.OpenAI{}),
					transport: transport,
				}
				opts := aibridged.DefaultPoolOptions
				opts.TTL = time.Hour
				if tt.expireBridge {
					opts.TTL = time.Second
				}
				sink := testutil.NewFakeSink(t)
				pool, err := aibridged.NewCachedBridgePool(opts, []aibridge.Provider{prov}, sink.Logger(), nil, testTracer)
				require.NoError(t, err)
				defer func() {
					_ = pool.Shutdown(context.Background())
					synctest.Wait()
				}()

				ctrl := gomock.NewController(t)
				mcpProxy := mcpmock.NewMockServerProxier(ctrl)
				mcpProxy.EXPECT().Init(gomock.Any()).Return(nil)
				cleaned := make(chan struct{})
				mcpProxy.EXPECT().Shutdown(gomock.Any()).DoAndReturn(func(context.Context) error {
					close(cleaned)
					return nil
				})
				identity := aibridged.Request{InitiatorID: uuid.New(), APIKeyID: uuid.NewString()}
				bridge, err := pool.Acquire(ctx, identity, nil, newMockMCPFactory(mcpProxy))
				require.NoError(t, err)
				response := httptest.NewRecorder()
				finished := make(chan struct{})
				go func() {
					defer close(finished)
					req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/openai/v1/responses/stream", nil)
					req.Header.Set("Authorization", "Bearer test-key")
					bridge.ServeHTTP(response, req)
				}()
				requestCtx := <-started
				synctest.Wait()

				if tt.expireBridge {
					// Wait for TTL cleanup before checking that the retired
					// bridge's stream remains active.
					<-time.After(opts.TTL + cacheCleanupAllowance)
					synctest.Wait()
					require.NoError(t, requestCtx.Err())
					select {
					case <-cleaned:
						t.Fatal("bridge resources closed before the stream drained")
					default:
					}

					// Eviction stops admission to the retired bridge, but the
					// same identity can acquire and use a replacement immediately.
					rejected := httptest.NewRecorder()
					bridge.ServeHTTP(rejected, httptest.NewRequest(http.MethodGet, "/openai/v1/models", nil))
					require.Equal(t, http.StatusServiceUnavailable, rejected.Code)
					replacementProxy := mcpmock.NewMockServerProxier(ctrl)
					replacementProxy.EXPECT().Init(gomock.Any()).Return(nil)
					replacementProxy.EXPECT().Shutdown(gomock.Any()).Return(nil)
					replacement, err := pool.Acquire(ctx, identity, nil, newMockMCPFactory(replacementProxy))
					require.NoError(t, err)
					require.NotSame(t, bridge, replacement)
					assertHandlerBody(t, replacement, "/openai/v1/models", "fresh")
				}

				if tt.blockAcquire {
					// An unrelated bridge build must not delay cancellation of
					// streams already draining when pool shutdown begins.
					factory := newBlockingMCPFactory()
					acquired := make(chan error, 1)
					defer func() {
						close(factory.releaseFirst)
						require.NoError(t, <-acquired)
					}()
					go func() {
						_, err := pool.Acquire(ctx, aibridged.Request{InitiatorID: uuid.New(), APIKeyID: uuid.NewString()}, nil, factory)
						acquired <- err
					}()
					<-factory.firstStarted
				}

				if tt.shutdownPool {
					shutdownCtx := context.Background()
					if tt.shutdownCtxTimeout > 0 {
						var shutdownCancel context.CancelFunc
						//nolint:gocritic // The synctest clock makes this an exact simulated caller deadline.
						shutdownCtx, shutdownCancel = context.WithTimeout(shutdownCtx, tt.shutdownCtxTimeout)
						defer shutdownCancel()
					}
					go func() { _ = pool.Shutdown(shutdownCtx) }()
					synctest.Wait()
				}
				if tt.aliveFor > 0 {
					<-time.After(tt.aliveFor)
					synctest.Wait()
					require.NoError(t, requestCtx.Err())
				}
				if tt.cancelWithin > 0 {
					<-time.After(tt.cancelWithin)
					synctest.Wait()
					require.ErrorIs(t, requestCtx.Err(), context.Canceled)
				} else {
					close(complete)
				}
				synctest.Wait()
				select {
				case <-finished:
				default:
					t.Fatal("stream did not finish after completion or drain deadline")
				}
				select {
				case <-cleaned:
				default:
					t.Fatal("bridge resources were not closed after drainage")
				}
				require.Equal(t, http.StatusOK, response.Code)
				require.Equal(t, tt.wantBody, response.Body.String())
				warnings := sink.Entries(func(e slog.SinkEntry) bool {
					return e.Level == slog.LevelWarn && strings.Contains(e.Message, "did not drain in time")
				})
				require.Equal(t, tt.wantWarning, len(warnings) > 0, "drain timeout warning")
			})
		})
	}
}

// drainTestProvider exercises the bridge's streaming passthrough without sockets,
// allowing all request and eviction goroutines to use synctest's fake clock.
type drainTestProvider struct {
	aibridge.Provider
	transport http.RoundTripper
}

func (p *drainTestProvider) WrapPassthroughTransport(http.RoundTripper) http.RoundTripper {
	return p.transport
}
