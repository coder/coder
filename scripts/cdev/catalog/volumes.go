package catalog

import (
	"context"
	"fmt"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
)

var _ Service[*docker.Volume] = (*Volume)(nil)

type Volume struct {
	name string
	vol  *docker.Volume
}

func VolumeGoCache() *Volume {
	return NewVolume("cdev_go_cache")
}

func VolumeCoderCache() *Volume {
	return NewVolume("cdev_coder_cache")
}

func OnVolumeOnGoCache() string {
	return VolumeGoCache().Name()
}

func OnVolumeCoderCache() string {
	return VolumeCoderCache().Name()
}

func NewVolume(name string) *Volume {
	return &Volume{
		name: name,
	}
}

func OnVolume(name string) string {
	return Volume{name: name}.Name()
}

func (v Volume) Name() string {
	return v.name
}

func (v Volume) DependsOn() []string {
	return []string{
		OnDocker(),
	}
}

func (v *Volume) Start(ctx context.Context, c *Catalog) error {
	pool := Get[*Docker](c)

	vol, err := pool.Client.InspectVolume(v.name)
	if err != nil {
		// Volume doesn't exist, create it.
		vol, err = pool.Client.CreateVolume(docker.CreateVolumeOptions{
			Name: v.name,
			Labels: map[string]string{
				CDevLabel:      "true",
				CDevLabelCache: "true",
			},
		})
		if err != nil {
			return fmt.Errorf("failed to create volume %s: %w", v.name, err)
		}

		//err = v.chown(ctx, pool)
		//if err != nil {
		//	return fmt.Errorf("failed to chown volume %s: %w", v.name, err)
		//}
	}
	v.vol = vol

	return nil
}

// initVolumes sets correct ownership on volumes (like init-volumes in docker-compose).
// Docker creates volumes as root by default, so we chown them to uid 1000 (coder user).
func (v *Volume) chown(ctx context.Context, pool *dockertest.Pool) error {
	initCmd := "chown -R 1000:1000 /"

	runOpts := &dockertest.RunOptions{
		Repository: dogfoodImage,
		Tag:        dogfoodTag,
		User:       "0:0", // Run as root to chown.
		Mounts: []string{
			fmt.Sprintf("%s:/", v.name),
		},
		Cmd: []string{"sh", "-c", initCmd},
		Labels: map[string]string{
			CDevLabel:          "true",
			CDevLabelEphemeral: "true",
		},
	}

	resource, err := pool.RunWithOptions(runOpts, func(config *docker.HostConfig) {
		config.AutoRemove = true
	})
	if err != nil {
		return fmt.Errorf("failed to start init container: %w", err)
	}

	exitCode, err := pool.Client.WaitContainerWithContext(resource.Container.ID, ctx)
	if err != nil {
		return fmt.Errorf("failed waiting for init: %w", err)
	}

	if exitCode != 0 {
		return fmt.Errorf("init volumes failed with exit code %d", exitCode)
	}

	return nil
}

func (v *Volume) Stop(ctx context.Context) error {
	return nil
}

func (v *Volume) Result() *docker.Volume {
	return v.vol
}
