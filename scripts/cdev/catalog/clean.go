package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

// Down stops and deletes selective resources, while keeping things like caches
func Down(ctx context.Context, logger slog.Logger, pool *dockertest.Pool) error {
	servicesToDown := []ServiceName{
		CDevLoadBalancer,
		CDevPostgres,
		CDevCoderd,
		CDevOIDC,
		CDevSite,
		CDevPrometheus,
		CDevProvisioner,
	}

	for _, service := range servicesToDown {
		err := Containers(ctx, logger.With(slog.F("service", service)), pool, NewServiceLabels(service).Filter())
		if err != nil {
			return xerrors.Errorf("stop %s containers: %w", service, err)
		}
	}

	return nil
}

// TODO: Cleanup old build-slim images? Can we reliably identify stale coder images that have been replaced
//
//	by a new "latest" tag? If so, we can delete those, and reduce the random "my disk is out of space"
//	"time to docker system prune"
func Cleanup(ctx context.Context, logger slog.Logger, pool *dockertest.Pool) error {
	filter := NewLabels().Filter() // Remove it all

	err := Containers(ctx, logger, pool, filter)
	if err != nil {
		logger.Error(ctx, "failed to clean up containers", slog.Error(err))
	}

	err = Volumes(ctx, logger, pool, filter)
	if err != nil {
		logger.Error(ctx, "failed to clean up volumes", slog.Error(err))
	}

	err = Images(ctx, logger, pool, filter)
	if err != nil {
		logger.Error(ctx, "failed to clean up images", slog.Error(err))
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
		return xerrors.Errorf("list containers: %w", err)
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

func Containers(ctx context.Context, logger slog.Logger, pool *dockertest.Pool, filter map[string][]string) error {
	err := StopContainers(ctx, logger, pool, NewLabels().Filter())
	if err != nil {
		return xerrors.Errorf("stop containers: %w", err)
	}

	res, err := pool.Client.PruneContainers(docker.PruneContainersOptions{
		Filters: filter,
		Context: ctx,
	})
	if err != nil {
		return xerrors.Errorf("prune containers: %w", err)
	}

	if len(res.ContainersDeleted) == 0 {
		return nil
	}

	logger.Info(ctx, fmt.Sprintf("📋 Deleted %d containers and reclaimed %s of space",
		len(res.ContainersDeleted), humanize.Bytes(uint64(max(0, res.SpaceReclaimed))), //nolint:gosec // G115 SpaceReclaimed is non-negative in practice
	))
	for _, id := range res.ContainersDeleted {
		logger.Debug(ctx, "🧹 Deleted container %s",
			slog.F("container_id", id),
		)
	}
	return nil
}

func Volumes(ctx context.Context, logger slog.Logger, pool *dockertest.Pool, filter map[string][]string) error {
	vols, err := pool.Client.ListVolumes(docker.ListVolumesOptions{
		Filters: filter,
	})
	if err != nil {
		return xerrors.Errorf("list volumes: %w", err)
	}

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

func Images(ctx context.Context, logger slog.Logger, pool *dockertest.Pool, filter map[string][]string) error {
	imgs, err := pool.Client.ListImages(docker.ListImagesOptions{
		Filters: filter,
	})
	if err != nil {
		return xerrors.Errorf("list images: %w", err)
	}

	for _, img := range imgs {
		err = pool.Client.RemoveImage(img.ID)
		if err != nil {
			logger.Error(ctx, fmt.Sprintf("Failed to remove image %s: %v", img.ID, err))
		} else {
			logger.Debug(ctx, "🧹 Deleted image %s",
				slog.F("image_id", img.ID),
				slog.F("image_size", humanize.Bytes(uint64(max(0, img.Size)))), //nolint:gosec // G115 Size is non-negative in practice
			)
		}
	}
	return nil
}
