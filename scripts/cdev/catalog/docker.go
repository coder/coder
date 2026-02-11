package catalog

import (
	"context"
	"fmt"
	"sync"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
)

var _ Service[*dockertest.Pool] = (*Docker)(nil)

func OnDocker() string {
	return (&Docker{}).Name()
}

// VolumeOptions configures a Docker volume to be lazily created.
type VolumeOptions struct {
	Name   string
	Labels map[string]string
	UID    int // 0 means skip chown.
	GID    int
}

type volumeOnce struct {
	once sync.Once
	vol  *docker.Volume
	err  error
}

type Docker struct {
	pool      *dockertest.Pool
	volumes   map[string]*volumeOnce
	volumesMu sync.Mutex
}

func NewDocker() *Docker {
	return &Docker{
		volumes: make(map[string]*volumeOnce),
	}
}

func (d *Docker) Name() string {
	return "docker"
}
func (d *Docker) Emoji() string {
	return "🐳"
}

func (d *Docker) DependsOn() []string {
	return []string{}
}

func (d *Docker) Start(ctx context.Context, _ *Catalog) error {
	pool, err := dockertest.NewPool("")
	if err != nil {
		return err
	}
	d.pool = pool
	return nil
}

func (d *Docker) Stop(ctx context.Context) error {
	return nil
}

func (d *Docker) Result() *dockertest.Pool {
	return d.pool
}

// EnsureVolume lazily creates a named Docker volume, returning it
// on all subsequent calls without repeating the creation work.
func (d *Docker) EnsureVolume(ctx context.Context, opts VolumeOptions) (*docker.Volume, error) {
	d.volumesMu.Lock()
	vo, ok := d.volumes[opts.Name]
	if !ok {
		vo = &volumeOnce{}
		d.volumes[opts.Name] = vo
	}
	d.volumesMu.Unlock()

	vo.once.Do(func() {
		vo.vol, vo.err = d.ensureVolume(ctx, opts)
	})
	return vo.vol, vo.err
}

func (d *Docker) ensureVolume(ctx context.Context, opts VolumeOptions) (*docker.Volume, error) {
	vol, err := d.pool.Client.InspectVolume(opts.Name)
	if err != nil {
		vol, err = d.pool.Client.CreateVolume(docker.CreateVolumeOptions{
			Name:   opts.Name,
			Labels: opts.Labels,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create volume %s: %w", opts.Name, err)
		}
		if opts.UID != 0 || opts.GID != 0 {
			if err := d.chownVolume(ctx, opts); err != nil {
				return nil, fmt.Errorf("failed to chown volume %s: %w", opts.Name, err)
			}
		}
	}
	return vol, nil
}

func (d *Docker) chownVolume(ctx context.Context, opts VolumeOptions) error {
	initCmd := fmt.Sprintf("chown -R %d:%d /mnt/volume", opts.UID, opts.GID)
	runOpts := &dockertest.RunOptions{
		Repository: dogfoodImage,
		Tag:        dogfoodTag,
		User:       "0:0",
		Mounts:     []string{fmt.Sprintf("%s:/mnt/volume", opts.Name)},
		Cmd:        []string{"sh", "-c", initCmd},
		Labels: map[string]string{
			CDevLabel:          "true",
			CDevLabelEphemeral: "true",
		},
	}
	resource, err := d.pool.RunWithOptions(runOpts, func(config *docker.HostConfig) {
		config.AutoRemove = true
	})
	if err != nil {
		return fmt.Errorf("failed to start init container: %w", err)
	}
	exitCode, err := d.pool.Client.WaitContainerWithContext(resource.Container.ID, ctx)
	if err != nil {
		return fmt.Errorf("failed waiting for init: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("init volumes failed with exit code %d", exitCode)
	}
	return nil
}
