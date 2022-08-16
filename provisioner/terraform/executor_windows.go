//go:build windows

package terraform

import (
	"context"
	"os/exec"
)

func interruptCommandOnCancel(ctx, killCtx context.Context, cmd *exec.Cmd) {
	go func() {
		select {
		case <-ctx.Done():
			// On Windows we can't sent an interrupt, so we just kill the process.
			_ = cmd.Process.Kill()
		case <-killCtx.Done():
		}
	}()
}
