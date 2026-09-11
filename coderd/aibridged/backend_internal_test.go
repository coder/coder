package aibridged

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/quartz"
)

// plainPooler is a Pooler without a KeyPools method, exercising the branch
// where the optional interface assertion fails.
type plainPooler struct{}

func (*plainPooler) Acquire(context.Context, Request, ClientFunc, MCPProxyBuilder) (http.Handler, error) {
	return http.NotFoundHandler(), nil
}
func (*plainPooler) ReplaceProviders([]aibridge.Provider) {}
func (*plainPooler) Shutdown(context.Context) error       { return nil }

var _ Pooler = &plainPooler{}

// TestServerKeyPools_InterceptionDelegates asserts that in interception mode
// Server.KeyPools delegates to the pool's KeyPools. It drives the production
// CachedBridgePool so the test regresses if that pool ever loses the KeyPools
// method the optional interface assertion in requestBackend.KeyPools relies on.
func TestServerKeyPools_InterceptionDelegates(t *testing.T) {
	t.Parallel()

	pool, err := keypool.New("openai", []string{"key"}, quartz.NewReal(), nil)
	require.NoError(t, err)
	provider := aibridge.NewOpenAIProvider(config.OpenAI{Name: "openai", BaseURL: "http://upstream.test", KeyPool: pool})

	cbp, err := NewCachedBridgePool(DefaultPoolOptions, []aibridge.Provider{provider}, slogtest.Make(t, nil), nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cbp.Shutdown(context.Background()) })

	var s Server
	s.backend.Store(&requestBackend{pool: cbp})

	require.Equal(t, []*keypool.Pool{pool}, s.KeyPools(),
		"interception KeyPools must delegate to the production CachedBridgePool")
}

// TestServerKeyPools_InterceptionPoolWithoutKeyPools asserts that a pool which
// does not expose KeyPools reports none rather than panicking on the failed
// interface assertion.
func TestServerKeyPools_InterceptionPoolWithoutKeyPools(t *testing.T) {
	t.Parallel()

	var s Server
	s.backend.Store(&requestBackend{pool: &plainPooler{}})

	require.Nil(t, s.KeyPools(),
		"a pool without a KeyPools method must report none")
}
