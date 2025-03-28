package terraform

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// TODO: replace this with the upstream OTEL env propagation when it is
// released.

// envCarrier is a propagation.TextMapCarrier that is used to extract or
// inject tracing environment variables. This is used with a
// propagation.TextMapPropagator
type envCarrier struct {
	Env []string
}

var _ propagation.TextMapCarrier = (*envCarrier)(nil)

func toKey(key string) string {
	key = strings.ToUpper(key)
	key = strings.ReplaceAll(key, "-", "_")
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' {
			return r
		}
		return -1
	}, key)
}

func (c *envCarrier) Set(key, value string) {
	if c == nil {
		return
	}
	key = toKey(key)
	for i, e := range c.Env {
		if strings.HasPrefix(e, key+"=") {
			// don't directly update the slice so we don't modify the slice
			// passed in
			newEnv := slices.Clone(c.Env)
			newEnv = append(newEnv[:i], append([]string{fmt.Sprintf("%s=%s", key, value)}, newEnv[i+1:]...)...)
			c.Env = newEnv
			return
		}
	}
	c.Env = append(c.Env, fmt.Sprintf("%s=%s", key, value))
}

func (*envCarrier) Get(_ string) string {
	// Get not necessary to inject environment variables
	panic("Not implemented")
}

func (*envCarrier) Keys() []string {
	// Keys not necessary to inject environment variables
	panic("Not implemented")
}

// otelEnvInject will add add any necessary environment variables for the span
// found in the Context.  If environment variables are already present
// in `environ` then they will be updated.  If no variables are found the
// new ones will be appended.  The new environment will be returned, `environ`
// will never be modified.
func otelEnvInject(ctx context.Context, environ []string) []string {
	c := &envCarrier{Env: environ}
	otel.GetTextMapPropagator().Inject(ctx, c)
	return c.Env
}
