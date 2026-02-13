package catalog

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

// waitForHealthy polls Docker's container health status until it
// reports "healthy" or the timeout expires. The container must
// have a Healthcheck configured in its docker.Config.
func waitForHealthy(ctx context.Context, logger slog.Logger, pool *dockertest.Pool, containerName string, timeout time.Duration) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(timeout)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return xerrors.Errorf("timeout waiting for %s to be healthy", containerName)
		case <-ticker.C:
			ctr, err := pool.Client.InspectContainer(containerName)
			if err != nil {
				continue
			}
			if ctr.State.Health.Status == "healthy" {
				logger.Info(ctx, "container is healthy", slog.F("container", containerName))
				return nil
			}
		}
	}
}


var _ Service[*dockertest.Pool] = (*Docker)(nil)

func OnDocker() ServiceName {
	return (&Docker{}).Name()
}

// VolumeOptions configures a Docker volume to be lazily created.
type VolumeOptions struct {
	Name   string
	Labels map[string]string
	UID    int // 0 means skip chown.
	GID    int
}

// CDevNetworkName is the Docker bridge network used by all cdev
// containers.
const CDevNetworkName = "cdev"

type volumeOnce struct {
	once sync.Once
	vol  *docker.Volume
	err  error
}

type Docker struct {
	currentStep atomic.Pointer[string]
	pool        *dockertest.Pool
	volumes     map[string]*volumeOnce
	volumesMu   sync.Mutex
	networkID   string
	networkOnce sync.Once
	networkErr  error
}

func NewDocker() *Docker {
	return &Docker{
		volumes: make(map[string]*volumeOnce),
	}
}

func (*Docker) Name() ServiceName {
	return CDevDocker
}
func (*Docker) Emoji() string {
	return "🐳"
}

func (*Docker) DependsOn() []ServiceName {
	return []ServiceName{}
}

func (d *Docker) CurrentStep() string {
	if s := d.currentStep.Load(); s != nil {
		return *s
	}
	return ""
}

func (d *Docker) setStep(step string) {
	d.currentStep.Store(&step)
}

func (d *Docker) Start(_ context.Context, _ slog.Logger, _ *Catalog) error {
	d.setStep("Connecting to Docker daemon")
	pool, err := dockertest.NewPool("")
	if err != nil {
		return err
	}
	d.pool = pool
	d.setStep("")
	return nil
}

func (*Docker) Stop(_ context.Context) error {
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
		vo.vol, vo.err = d.createVolumeIfNeeded(ctx, opts)
	})
	return vo.vol, vo.err
}
// EnsureNetwork lazily creates the cdev Docker bridge network,
// returning its ID on all subsequent calls without repeating the
// creation work.
func (d *Docker) EnsureNetwork(_ context.Context, labels map[string]string) (string, error) {
	d.networkOnce.Do(func() {
		d.networkID, d.networkErr = d.createNetworkIfNeeded(labels)
	})
	return d.networkID, d.networkErr
}

func (d *Docker) createNetworkIfNeeded(labels map[string]string) (string, error) {
	networks, err := d.pool.Client.FilteredListNetworks(docker.NetworkFilterOpts{
		"name": map[string]bool{CDevNetworkName: true},
	})
	if err != nil {
		return "", xerrors.Errorf("failed to list networks: %w", err)
	}
	// FilteredListNetworks does substring matching, so check for
	// an exact name match before deciding to create.
	for _, n := range networks {
		if n.Name == CDevNetworkName {
			return n.ID, nil
		}
	}
	net, err := d.pool.Client.CreateNetwork(docker.CreateNetworkOptions{
		Name:   CDevNetworkName,
		Driver: "bridge",
		Labels: labels,
	})
	if err != nil {
		return "", xerrors.Errorf("failed to create network %s: %w", CDevNetworkName, err)
	}
	return net.ID, nil
}


func (d *Docker) createVolumeIfNeeded(ctx context.Context, opts VolumeOptions) (*docker.Volume, error) {
	vol, err := d.pool.Client.InspectVolume(opts.Name)
	if err != nil {
		vol, err = d.pool.Client.CreateVolume(docker.CreateVolumeOptions{
			Name:   opts.Name,
			Labels: opts.Labels,
		})
		if err != nil {
			return nil, xerrors.Errorf("failed to create volume %s: %w", opts.Name, err)
		}
		if opts.UID != 0 || opts.GID != 0 {
			if err := d.chownVolume(ctx, opts); err != nil {
				return nil, xerrors.Errorf("failed to chown volume %s: %w", opts.Name, err)
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
		return xerrors.Errorf("failed to start init container: %w", err)
	}
	exitCode, err := d.pool.Client.WaitContainerWithContext(resource.Container.ID, ctx)
	if err != nil {
		return xerrors.Errorf("failed waiting for init: %w", err)
	}
	if exitCode != 0 {
		return xerrors.Errorf("init volumes failed with exit code %d", exitCode)
	}
	return nil
}
