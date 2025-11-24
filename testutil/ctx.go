package testutil

import (
	"context"
	"testing"
	"time"
)

func Context(t testing.TB, dur time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), dur)
	t.Cleanup(cancel)
	return ctx
}
