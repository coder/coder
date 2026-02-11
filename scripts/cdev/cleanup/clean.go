package cleanup

import (
	"context"
	"fmt"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/scripts/cdev/catalog"
)

// Down stops and deletes selective resources, while keeping things like caches
func Down(ctx context.Context, logger slog.Logger, pool *dockertest.Pool) error {
	servicesToDown := []catalog.ServiceName{
		catalog.CDevPostgres,
		catalog.CDevCoderd,
	}

	for _, service := range servicesToDown {
		err := CleanupContainers(ctx, logger, pool, catalog.NewServiceLabels(service).Filter())
		if err != nil {
			return fmt.Errorf("stop %s containers: %w", service, err)
		}
	}

	return nil
}

// TODO: Cleanup old build-slim images? Can we reliably identify stale coder images that have been replaced
//
//	by a new "latest" tag? If so, we can delete those, and reduce the random "my disk is out of space"
//	"time to docker system prune"
func Cleanup(ctx context.Context, logger slog.Logger, pool *dockertest.Pool) error {
	filter := catalog.NewLabels().Filter() // Remove it all

	pool, err := dockertest.NewPool("")
	if err != nil {
		return fmt.Errorf("failed to connect to docker: %w", err)
	}

	err = CleanupContainers(ctx, logger, pool, filter)
	if err != nil {
		logger.Error(ctx, "Failed to clean up containers: %v", slog.F("error", err))
	}

	err = CleanupVolumes(ctx, logger, pool, filter)
	if err != nil {
		logger.Error(ctx, "Failed to clean up volumes: %v", slog.F("error", err))
	}

	return nil
}

func StopContainers(ctx context.Context, logger slog.Logger, pool *dockertest.Pool, filter map[string][]string) error {
	containers, err := pool.Client.ListContainers(docker.ListContainersOptions{
		All:     true,
		Filters: filter,
		Context: ctx,
	})
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}
	for _, cnt := range containers {
		err := pool.Client.StopContainer(cnt.ID, 10)
		if err != nil && !strings.Contains(err.Error(), "Container not running") {
			logger.Error(ctx, fmt.Sprintf("Failed to stop container %s: %v", cnt.ID, err))
			// Continue trying to stop other containers even if one fails.
			continue
		}
	}
	return nil
}

func CleanupContainers(ctx context.Context, logger slog.Logger, pool *dockertest.Pool, filter map[string][]string) error {
	err := StopContainers(ctx, logger, pool, catalog.NewLabels().Filter())
	if err != nil {
		return fmt.Errorf("stop containers: %w", err)
	}

	res, err := pool.Client.PruneContainers(docker.PruneContainersOptions{
		Filters: filter,
		Context: ctx,
	})
	if err != nil {
		return fmt.Errorf("prune containers: %w", err)
	}

	if len(res.ContainersDeleted) == 0 {
		return nil
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

func CleanupVolumes(ctx context.Context, logger slog.Logger, pool *dockertest.Pool, filter map[string][]string) error {
	vols, err := pool.Client.ListVolumes(docker.ListVolumesOptions{
		Filters: filter,
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
