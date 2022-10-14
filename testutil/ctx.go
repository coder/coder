package testutil

import (
	"context"
	"testing"
)

func Context(t *testing.T) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), WaitLong)
	t.Cleanup(cancel)
	return ctx, cancel
}
