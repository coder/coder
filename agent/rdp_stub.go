//go:build !windows

package agent

import (
	"context"
)

func (a *agent) monitorRDP(ctx context.Context) {
	// No-op on non-Windows
}
