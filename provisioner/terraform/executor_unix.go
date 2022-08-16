//go:build !windows

package terraform

import (
	"context"
	"os"
	"os/exec"
)

func interruptCommandOnCancel(ctx, killCtx context.Context, cmd *exec.Cmd) {
	go func() {
		select {
		case <-ctx.Done():
			_ = cmd.Process.Signal(os.Interrupt)
		case <-killCtx.Done():
		}
	}()
}
