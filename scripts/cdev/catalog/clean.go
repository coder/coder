package catalog

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/ory/dockertest/v3/docker"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

// Down stops containers via docker compose down.
func Down(ctx context.Context, logger slog.Logger) error {
	logger.Info(ctx, "running docker compose down")
	//nolint:gosec // Arguments are controlled.
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", composeFilePath(), "down")
	cmd.Stdout = LogWriter(logger, slog.LevelInfo, "compose-down")
	cmd.Stderr = LogWriter(logger, slog.LevelWarn, "compose-down")
	if err := cmd.Run(); err != nil {
		return xerrors.Errorf("docker compose down: %w", err)
	}
	return nil
}

// Cleanup removes all compose resources including volumes and locally
// built images.
func Cleanup(ctx context.Context, logger slog.Logger) error {
	logger.Info(ctx, "running docker compose down -v --rmi local")
	//nolint:gosec // Arguments are controlled.
	cmd := exec.CommandContext(ctx,
		"docker", "compose", "-f", composeFilePath(),
		"down", "-v", "--rmi", "local",
	)
	cmd.Stdout = LogWriter(logger, slog.LevelInfo, "compose-cleanup")
	cmd.Stderr = LogWriter(logger, slog.LevelWarn, "compose-cleanup")
	if err := cmd.Run(); err != nil {
		// If the compose file doesn't exist, fall back to direct
		// Docker cleanup via labels.
		logger.Warn(ctx, "compose down failed, falling back to label-based cleanup", slog.Error(err))
	}

	// Also clean up any remaining cdev-labeled resources that may
	// not be in the compose file.
	client, err := docker.NewClientFromEnv()
	if err != nil {
		return xerrors.Errorf("connect to docker: %w", err)
	}

	filter := NewLabels().Filter()

	if err := cleanContainers(ctx, logger, client, filter); err != nil {
		logger.Error(ctx, "failed to clean up containers", slog.Error(err))
	}

	if err := cleanVolumes(ctx, logger, client, filter); err != nil {
		logger.Error(ctx, "failed to clean up volumes", slog.Error(err))
	}

	if err := cleanImages(ctx, logger, client, filter); err != nil {
		logger.Error(ctx, "failed to clean up images", slog.Error(err))
	}

	return nil
}

func StopContainers(ctx context.Context, logger slog.Logger, client *docker.Client, filter map[string][]string) error {
	containers, err := client.ListContainers(docker.ListContainersOptions{
		All:     true,
		Filters: filter,
		Context: ctx,
	})
	if err != nil {
		return xerrors.Errorf("list containers: %w", err)
	}
	for _, cnt := range containers {
		err := client.StopContainer(cnt.ID, 10)
		if err != nil && !strings.Contains(err.Error(), "Container not running") {
			logger.Error(ctx, fmt.Sprintf("Failed to stop container %s: %v", cnt.ID, err))
			// Continue trying to stop other containers even if one fails.
			continue
		}
	}
	return nil
}

func Containers(ctx context.Context, logger slog.Logger, client *docker.Client, filter map[string][]string) error {
	err := StopContainers(ctx, logger, client, NewLabels().Filter())
	if err != nil {
		return xerrors.Errorf("stop containers: %w", err)
	}

	res, err := client.PruneContainers(docker.PruneContainersOptions{
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

func cleanContainers(ctx context.Context, logger slog.Logger, client *docker.Client, filter map[string][]string) error {
	return Containers(ctx, logger, client, filter)
}

func Volumes(ctx context.Context, logger slog.Logger, client *docker.Client, filter map[string][]string) error {
	vols, err := client.ListVolumes(docker.ListVolumesOptions{
		Filters: filter,
	})
	if err != nil {
		return xerrors.Errorf("list volumes: %w", err)
	}

	for _, vol := range vols {
		err = client.RemoveVolumeWithOptions(docker.RemoveVolumeOptions{
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

func cleanVolumes(ctx context.Context, logger slog.Logger, client *docker.Client, filter map[string][]string) error {
	return Volumes(ctx, logger, client, filter)
}

func Images(ctx context.Context, logger slog.Logger, client *docker.Client, filter map[string][]string) error {
	imgs, err := client.ListImages(docker.ListImagesOptions{
		Filters: filter,
	})
	if err != nil {
		return xerrors.Errorf("list images: %w", err)
	}

	for _, img := range imgs {
		err = client.RemoveImage(img.ID)
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

func cleanImages(ctx context.Context, logger slog.Logger, client *docker.Client, filter map[string][]string) error {
	return Images(ctx, logger, client, filter)
}
