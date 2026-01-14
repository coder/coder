//go:build linux

package landjail

import (
	"context"
	"log/slog"
	"os"

	"github.com/coder/coder/v2/enterprise/cli/boundary/config"
)

func isChild() bool {
	return os.Getenv("CHILD") == "true"
}

// Run is the main entry point that determines whether to execute as a parent or child process.
// If running as a child (CHILD env var is set), it applies landlock restrictions
// and executes the target command. Otherwise, it runs as the parent process, sets up the proxy server,
// and manages the child process lifecycle.
func Run(ctx context.Context, logger *slog.Logger, config config.AppConfig) error {
	if isChild() {
		return RunChild(logger, config)
	}

	return RunParent(ctx, logger, config)
}
