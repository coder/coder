package provider

import (
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel/trace"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// DisabledStub is a Provider placeholder for a configured-but-disabled
// provider. Only Name and Enabled return meaningful values; all other
// methods return empty/nil so the stub never influences routing.
type DisabledStub struct {
	name         string
	providerType string
}

// NewDisabledStub returns a Provider stub that reports Enabled() == false.
// The type string is preserved so callers can distinguish provider families.
func NewDisabledStub(name, providerType string) *DisabledStub {
	return &DisabledStub{name: name, providerType: providerType}
}

func (d *DisabledStub) Type() string  { return d.providerType }
func (d *DisabledStub) Name() string  { return d.name }
func (*DisabledStub) Enabled() bool   { return false }
func (*DisabledStub) BaseURL() string { return "" }
func (d *DisabledStub) RoutePrefix() string {
	return fmt.Sprintf("/%s", d.name)
}
func (*DisabledStub) BridgedRoutes() []string     { return nil }
func (*DisabledStub) PassthroughRoutes() []string { return nil }
func (*DisabledStub) AuthHeader() string          { return "" }
func (*DisabledStub) KeyPool() *keypool.Pool      { return nil }
func (*DisabledStub) KeyFailoverConfig(_ slog.Logger) keypool.KeyFailoverConfig {
	return keypool.KeyFailoverConfig{}
}
func (*DisabledStub) CircuitBreakerConfig() *config.CircuitBreaker { return nil }
func (*DisabledStub) APIDumpDir() string                           { return "" }
func (*DisabledStub) CategorizeError(error) *recorder.ErrorType    { return nil }

func (*DisabledStub) CreateInterceptor(_ http.ResponseWriter, _ *http.Request, _ trace.Tracer) (intercept.Interceptor, error) {
	//nolint:nilnil // disabled providers never reach the interceptor.
	return nil, nil
}
