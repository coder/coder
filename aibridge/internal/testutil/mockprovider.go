package testutil

import (
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel/trace"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/quartz"
)

// SingleKeyPool builds a centralized key pool containing a single key, or nil
// when key is empty (no centralized credential). It panics if the pool cannot
// be built, which does not happen for a non-empty key.
func SingleKeyPool(name, key string) *keypool.Pool {
	if key == "" {
		return nil
	}
	pool, err := keypool.New(name, []string{key}, quartz.NewReal(), nil)
	if err != nil {
		panic(err)
	}
	return pool
}

type MockProvider struct {
	NameStr         string
	URL             string
	Disabled        bool
	Bridged         []string
	Passthrough     []string
	InterceptorFunc func(w http.ResponseWriter, r *http.Request, tracer trace.Tracer) (intercept.Interceptor, error)
}

func (m *MockProvider) Type() string                { return m.NameStr }
func (m *MockProvider) Name() string                { return m.NameStr }
func (m *MockProvider) Enabled() bool               { return !m.Disabled }
func (m *MockProvider) BaseURL() string             { return m.URL }
func (m *MockProvider) RoutePrefix() string         { return fmt.Sprintf("/%s", m.NameStr) }
func (m *MockProvider) BridgedRoutes() []string     { return m.Bridged }
func (m *MockProvider) PassthroughRoutes() []string { return m.Passthrough }
func (*MockProvider) AuthHeader() string            { return "Authorization" }

func (*MockProvider) KeyPool() *keypool.Pool { return nil }
func (*MockProvider) KeyFailoverConfig(_ slog.Logger) keypool.KeyFailoverConfig {
	return keypool.KeyFailoverConfig{}
}
func (*MockProvider) CircuitBreakerConfig() *config.CircuitBreaker { return nil }
func (*MockProvider) APIDumpDir() string                           { return "" }
func (*MockProvider) CategorizeError(error) *recorder.ErrorType    { return nil }

func (m *MockProvider) CreateInterceptor(w http.ResponseWriter, r *http.Request, tracer trace.Tracer) (intercept.Interceptor, error) {
	if m.InterceptorFunc != nil {
		return m.InterceptorFunc(w, r, tracer)
	}
	return nil, nil //nolint:nilnil // mock: no interceptor configured is not an error
}
