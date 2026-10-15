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

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/mcpmock"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/testutil"
)

func TestPoolDrain(t *testing.T) {
	t.Parallel()

	for _, scenario := range []string{"Complete", "DrainTimeout", "ShutdownRetired", "ShutdownCached", "ShutdownBlockedAcquire"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			// Ristretto uses the standard clock internally. A synctest bubble
			// advances both its TTL cleanup and the bridge drain deadlines.
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				started := make(chan context.Context, 1)
				complete := make(chan struct{})
				const firstEvent = "data: partial\n\n"
				const lastEvent = "data: [DONE]\n\n"
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
				opts.TTL = time.Second
				if scenario == "ShutdownCached" {
					opts.TTL = time.Hour
				}
				pool, err := aibridged.NewCachedBridgePool(opts, []aibridge.Provider{prov}, slogtest.Make(t, nil), nil, testTracer)
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

				if scenario != "ShutdownCached" {
					// Exceed the TTL, cleanup lag, and the previous five-second
					// drain. The original stream must still be running.
					<-time.After(20 * time.Second)
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

				if scenario == "ShutdownBlockedAcquire" {
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

				switch scenario {
				case "Complete":
					close(complete)
				case "DrainTimeout":
					<-time.After(9 * time.Minute)
					synctest.Wait()
					require.NoError(t, requestCtx.Err())
					<-time.After(time.Minute)
				case "ShutdownRetired", "ShutdownCached", "ShutdownBlockedAcquire":
					go func() { _ = pool.Shutdown(context.Background()) }()
					synctest.Wait()
					<-time.After(4 * time.Second)
					synctest.Wait()
					require.NoError(t, requestCtx.Err())
					<-time.After(time.Second)
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
				if scenario == "Complete" {
					require.Equal(t, firstEvent+lastEvent, response.Body.String())
				} else {
					require.ErrorIs(t, requestCtx.Err(), context.Canceled)
					require.Equal(t, firstEvent, response.Body.String())
				}
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
