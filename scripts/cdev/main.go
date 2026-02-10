package main

import (
	"fmt"
	"os"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"

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
			cleanCmd(),
		},
	}

	err := cmd.Invoke().WithOS().Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func cleanCmd() *serpent.Command {
	return &serpent.Command{
		Use:   "clean",
		Short: "Remove all cdev-managed resources (volumes, containers, etc.)",
		Handler: func(inv *serpent.Invocation) error {
			pool, err := dockertest.NewPool("")
			if err != nil {
				return fmt.Errorf("failed to connect to docker: %w", err)
			}

			res, err := pool.Client.PruneContainers(docker.PruneContainersOptions{
				Filters: map[string][]string{
					"label": {catalog.CDevLabel},
				},
				Context: nil,
			})
			if err != nil {
				return fmt.Errorf("failed to prune containers: %w", err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "🧹 Deleted %d containers and reclaimed %d bytes of space\n", len(res.ContainersDeleted), res.SpaceReclaimed)
			for _, id := range res.ContainersDeleted {
				_, _ = fmt.Fprintf(inv.Stdout, "🧹 Deleted container %s\n", id)
			}

			vols, err := pool.Client.ListVolumes(docker.ListVolumesOptions{
				Filters: map[string][]string{
					"label": {catalog.CDevLabel},
				},
			})

			for _, vol := range vols {
				err = pool.Client.RemoveVolumeWithOptions(docker.RemoveVolumeOptions{
					Context: nil,
					Name:    vol.Name,
					Force:   true,
				})
				if err != nil {
					_, _ = fmt.Fprintf(inv.Stderr, "failed to remove volume %s: %v\n", vol.Name, err)
					// Continue trying to remove other volumes even if one fails.
				}
				_, _ = fmt.Fprintf(inv.Stdout, "🧹 Deleted volume %s\n", vol.Name)
			}

			return nil
		},
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

			services := catalog.New(logger)
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
