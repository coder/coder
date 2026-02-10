package main

import (
	"fmt"
	"os"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/scripts/cdev/catalog"
	"github.com/coder/serpent"
)

const (
	// Volume names for cdev caches.
	VolumeGoCache    = "cdev_go_cache"
	VolumeCoderCache = "cdev_coder_cache"
)

func main() {
	cmd := &serpent.Command{
		Use:   "cdev",
		Short: "Development environment manager for Coder",
		Long:  "A smart, opinionated tool for running the Coder development stack.",
		Children: []*serpent.Command{
			upCmd(),
		},
	}

	err := cmd.Invoke().WithOS().Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func upCmd() *serpent.Command {
	return &serpent.Command{
		Use:   "up",
		Short: "Start the development environment",
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			logger := slog.Make(sloghuman.Sink(inv.Stderr))

			fmt.Fprintln(inv.Stdout, "🚀 Starting cdev...")

			services := catalog.New()
			err := services.Register(
				catalog.NewDocker(),
				catalog.VolumeCoderCache(),
				catalog.VolumeGoCache(),
				catalog.NewBuildSlim(),
			)
			if err != nil {
				return err
			}

			err = services.Start(ctx, logger)
			if err != nil {
				return fmt.Errorf("failed to start services: %w", err)
			}

			fmt.Fprintln(inv.Stdout, "✅ Volumes ready!")
			return nil
		},
	}
}
