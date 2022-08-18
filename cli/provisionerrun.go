package cli

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
)

func provisionerRun() *cobra.Command {
	var (
		cacheDir string
		verbose  bool
	)
	root := &cobra.Command{
		Use:   "run",
		Short: "Run a standalone Coder provisioner",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			notifyCtx, notifyStop := signal.NotifyContext(ctx, interruptSignals...)
			defer notifyStop()

			logger := slog.Make(sloghuman.Sink(cmd.ErrOrStderr()))
			if verbose {
				logger = logger.Leveled(slog.LevelDebug)
			}

			client, err := createClient(cmd)
			if err != nil {
				return err
			}

			errCh := make(chan error, 1)
			provisionerDaemon, err := newProvisionerDaemon(ctx, client.ListenProvisionerDaemon, logger, cacheDir, errCh, false)
			if err != nil {
				return xerrors.Errorf("create provisioner daemon: %w", err)
			}

			var exitErr error
			select {
			case <-notifyCtx.Done():
				exitErr = notifyCtx.Err()
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Bold.Render(
					"Interrupt caught, gracefully exiting. Use ctrl+\\ to force quit",
				))
			case exitErr = <-errCh:
			}

			err = provisionerDaemon.Close()
			if err != nil {
				cmd.PrintErrf("Close provisioner daemon: %s\n", err)
				return err
			}

			return exitErr
		},
	}
	defaultCacheDir := filepath.Join(os.TempDir(), "coder-cache")
	if dir := os.Getenv("CACHE_DIRECTORY"); dir != "" {
		// For compatibility with systemd.
		defaultCacheDir = dir
	}
	cliflag.StringVarP(root.Flags(), &cacheDir, "cache-dir", "", "CODER_CACHE_DIRECTORY", defaultCacheDir, "Specifies a directory to cache binaries for provision operations. If unspecified and $CACHE_DIRECTORY is set, it will be used for compatibility with systemd.")
	cliflag.BoolVarP(root.Flags(), &verbose, "verbose", "v", "CODER_VERBOSE", false, "Enables verbose logging.")
	return root
}
