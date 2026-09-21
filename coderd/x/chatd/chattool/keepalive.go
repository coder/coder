package chattool

import "context"

// attemptKeepaliveKey carries the task attempt keepalive kick
// function through tool execution contexts.
type attemptKeepaliveKey struct{}

// WithAttemptKeepalive returns a context carrying kick, a function
// that resets the owning task attempt's idle watchdog. Only the
// execute and process_output tools call KickAttemptKeepalive, after
// each successful process-API round-trip, to signal that the agent
// is still responsive while a potentially long-running command
// runs. Other workspace tools deliberately do not extend the idle
// window.
func WithAttemptKeepalive(ctx context.Context, kick func()) context.Context {
	if kick == nil {
		return ctx
	}
	return context.WithValue(ctx, attemptKeepaliveKey{}, kick)
}

// KickAttemptKeepalive resets the task attempt idle watchdog carried
// by ctx. It is a no-op when the context carries no keepalive or the
// attempt already ended, so callers never need to guard the call.
func KickAttemptKeepalive(ctx context.Context) {
	kick, _ := ctx.Value(attemptKeepaliveKey{}).(func())
	if kick != nil {
		kick()
	}
}
