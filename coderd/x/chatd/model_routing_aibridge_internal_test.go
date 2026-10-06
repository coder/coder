package chatd

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestStandaloneGatewayModelAuthentication(t *testing.T) {
	t.Parallel()
	for _, provider := range []string{"openai", "anthropic", "openai-compat"} {
		for _, mode := range []string{"generate", "stream", "provider401"} {
			t.Run(provider+"/"+mode, func(t *testing.T) {
				t.Parallel()
				var requests, dispatched atomic.Int32
				delegation := aibridged.DelegationMiddleware("gateway-key")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					dispatched.Add(1)
					http.Error(w, "invalid provider API key", http.StatusUnauthorized)
				}))
				gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					delegation.ServeHTTP(w, r)
				}))
				t.Cleanup(gateway.Close)
				target, err := url.Parse(gateway.URL)
				require.NoError(t, err)
				key := "wrong-key"
				if mode == "provider401" {
					key = "gateway-key"
				}
				rt, err := aibridged.NewHTTPTransportFactory(target, key).TransportFor("test-provider", aibridge.SourceAgents)
				require.NoError(t, err)
				model, err := newLanguageModel(provider, "test-model", chatprovider.ProviderAPIKeys{
					ByProvider:        map[string]string{provider: "placeholder"},
					BaseURLByProvider: map[string]string{provider: "http://coder-aibridge/v1"},
				}, "", nil, &http.Client{Transport: &aiGatewayRoundTripper{base: rt, apiKeyID: "synthetic-key"}}, &codersdk.ChatModelCallConfig{})
				require.NoError(t, err)
				ctx := testutil.Context(t, testutil.WaitShort)
				call := fantasy.Call{Prompt: []fantasy.Message{{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}}}}}
				if mode == "stream" {
					var stream fantasy.StreamResponse
					stream, err = model.LanguageModel().Stream(ctx, call)
					if err == nil {
						for part := range stream {
							if part.Type == fantasy.StreamPartTypeError {
								err = part.Error
								break
							}
						}
					}
				} else {
					_, err = model.LanguageModel().Generate(ctx, call)
				}
				require.Error(t, err)
				classified := chaterror.Classify(err).WithProvider(provider)
				require.Equal(t, http.StatusUnauthorized, classified.StatusCode)
				require.EqualValues(t, 1, requests.Load(), "provider SDK must not add retries")
				if mode == "provider401" {
					require.False(t, classified.Retryable)
					require.EqualValues(t, 1, dispatched.Load())
				} else {
					require.ErrorIs(t, err, aibridge.ErrGatewayKeyMismatch)
					require.True(t, classified.Retryable)
					require.Equal(t, aibridge.ErrGatewayKeyMismatch.Error(), classified.Detail)
					require.Zero(t, dispatched.Load())
				}
			})
		}
	}
}
