package cleanup

import (
	"context"
	"fmt"

	"github.com/dustin/go-humanize"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/scripts/cdev/catalog"
)

// TODO: Cleanup old build-slim images? Can we reliably identify stale coder images that have been replaced
//
//	by a new "latest" tag? If so, we can delete those, and reduce the random "my disk is out of space"
//	"time to docker system prune"
func Cleanup(ctx context.Context, logger slog.Logger, pool *dockertest.Pool) error {
	pool, err := dockertest.NewPool("")
	if err != nil {
		return fmt.Errorf("failed to connect to docker: %w", err)
	}

	err = CleanupContainers(ctx, logger, pool)
	if err != nil {
		logger.Error(ctx, "Failed to clean up containers: %v", slog.F("error", err))
	}

	err = CleanupVolumes(ctx, logger, pool)
	if err != nil {
		logger.Error(ctx, "Failed to clean up volumes: %v", slog.F("error", err))
	}

	return nil
}

func CleanupContainers(ctx context.Context, logger slog.Logger, pool *dockertest.Pool) error {
	res, err := pool.Client.PruneContainers(docker.PruneContainersOptions{
		Filters: catalog.NewLabels().Filter(),
		Context: ctx,
	})
	if err != nil {
		return fmt.Errorf("prune containers: %w", err)
	}

	logger.Info(ctx, fmt.Sprintf("📋 Deleted %d containers and reclaimed %s bytes of space",
		len(res.ContainersDeleted), humanize.Bytes(uint64(res.SpaceReclaimed)),
	))
	for _, id := range res.ContainersDeleted {
		logger.Debug(ctx, "🧹 Deleted container %s",
			slog.F("container_id", id),
		)
	}
	return nil
}

func CleanupVolumes(ctx context.Context, logger slog.Logger, pool *dockertest.Pool) error {
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
			logger.Error(ctx, fmt.Sprintf("Failed to remove volume %s: %v", vol.Name, err))
			// Continue trying to remove other volumes even if one fails.
		} else {
			logger.Debug(ctx, "🧹 Deleted volume %s",
				slog.F("volume_name", vol.Name),
			)
		}
	}
	return nil
}
