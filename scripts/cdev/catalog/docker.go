package catalog

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ory/dockertest/v3/docker"
	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"cdr.dev/slog/v3"
)

// waitForHealthy polls Docker's container health status until it
// reports "healthy" or the timeout expires. The container must
// have a Healthcheck configured in its docker.Config.
func waitForHealthy(ctx context.Context, logger slog.Logger, client *docker.Client, containerName string, timeout time.Duration) error {
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
			ctr, err := client.InspectContainer(containerName)
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

var _ Service[*docker.Client] = (*Docker)(nil)

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
	client      *docker.Client
	volumes     map[string]*volumeOnce
	volumesMu   sync.Mutex
	networkID   string
	networkOnce sync.Once
	networkErr  error

	composeMu  sync.Mutex

	// compose holds registered compose services keyed by name.
	compose map[string]ComposeService
	// composeVolumes holds registered compose volumes keyed by name.
	composeVolumes map[string]ComposeVolume
}

func NewDocker() *Docker {
	return &Docker{
		volumes:        make(map[string]*volumeOnce),
		compose:        make(map[string]ComposeService),
		composeVolumes: make(map[string]ComposeVolume),
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
	client, err := docker.NewClientFromEnv()
	if err != nil {
		return xerrors.Errorf("connect to docker: %w", err)
	}
	d.client = client
	d.setStep("")
	return nil
}

func (*Docker) Stop(_ context.Context) error {
	return nil
}

func (d *Docker) Result() *docker.Client {
	return d.client
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
	networks, err := d.client.FilteredListNetworks(docker.NetworkFilterOpts{
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
	net, err := d.client.CreateNetwork(docker.CreateNetworkOptions{
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
	vol, err := d.client.InspectVolume(opts.Name)
	if err != nil {
		vol, err = d.client.CreateVolume(docker.CreateVolumeOptions{
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

	container, err := d.client.CreateContainer(docker.CreateContainerOptions{
		Config: &docker.Config{
			Image: dogfoodImage + ":" + dogfoodTag,
			User:  "0:0",
			Cmd:   []string{"sh", "-c", initCmd},
			Labels: map[string]string{
				CDevLabel:          "true",
				CDevLabelEphemeral: "true",
			},
		},
		HostConfig: &docker.HostConfig{
			AutoRemove: true,
			Binds:      []string{fmt.Sprintf("%s:/mnt/volume", opts.Name)},
		},
	})
	if err != nil {
		return xerrors.Errorf("failed to create init container: %w", err)
	}
	if err := d.client.StartContainer(container.ID, nil); err != nil {
		return xerrors.Errorf("failed to start init container: %w", err)
	}
	exitCode, err := d.client.WaitContainerWithContext(container.ID, ctx)
	if err != nil {
		return xerrors.Errorf("failed waiting for init: %w", err)
	}
	if exitCode != 0 {
		return xerrors.Errorf("init volumes failed with exit code %d", exitCode)
	}
	return nil
}

// SetCompose registers a compose service definition.
func (d *Docker) SetCompose(name string, svc ComposeService) {
	d.composeMu.Lock()
	defer d.composeMu.Unlock()
	d.compose[name] = svc
}

// SetComposeVolume registers a compose volume definition.
func (d *Docker) SetComposeVolume(name string, vol ComposeVolume) {
	d.composeMu.Lock()
	defer d.composeMu.Unlock()
	d.composeVolumes[name] = vol
}

// composeFilePath returns the path to the compose file.
func composeFilePath() string {
	return filepath.Join(".cdev", "docker-compose.yml")
}

// WriteCompose writes the current compose state to
// .cdev/docker-compose.yml.
func (d *Docker) WriteCompose(_ context.Context) error {
	d.composeMu.Lock()
	defer d.composeMu.Unlock()

	// Strip depends_on entries referencing services not yet
	// registered — the catalog DAG handles ordering, and
	// partial compose files may not contain all services.
	services := make(map[string]ComposeService, len(d.compose))
	for name, svc := range d.compose {
		if len(svc.DependsOn) > 0 {
			filtered := make(map[string]ComposeDependsOn, len(svc.DependsOn))
			for dep, cond := range svc.DependsOn {
				if _, ok := d.compose[dep]; ok {
					filtered[dep] = cond
				}
			}
			svc.DependsOn = filtered
		}
		services[name] = svc
	}

	cf := &ComposeFile{
		Services: services,
		Volumes:  d.composeVolumes,
		Networks: map[string]ComposeNetwork{
			composeNetworkName: {Driver: "bridge"},
		},
	}

	data, err := yaml.Marshal(cf)
	if err != nil {
		return xerrors.Errorf("marshal compose file: %w", err)
	}

	if err := os.MkdirAll(".cdev", 0o755); err != nil {
		return xerrors.Errorf("create .cdev directory: %w", err)
	}

	// Atomic write: temp file + rename to avoid readers
	// seeing a truncated file.
	tmp := composeFilePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return xerrors.Errorf("write compose temp file: %w", err)
	}
	if err := os.Rename(tmp, composeFilePath()); err != nil {
		return xerrors.Errorf("rename compose file: %w", err)
	}

	return nil
}

// DockerComposeUp runs `docker compose up -d` for the given services.
func (d *Docker) DockerComposeUp(ctx context.Context, services ...string) error {
	if err := d.WriteCompose(ctx); err != nil {
		return err
	}
	args := []string{"compose", "-f", composeFilePath(), "up", "-d"}
	args = append(args, services...)
	//nolint:gosec // Arguments are controlled.
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return xerrors.Errorf("docker compose up: %w", err)
	}
	return nil
}

// DockerComposeRun runs `docker compose run --rm` for a blocking
// one-shot service.
func (d *Docker) DockerComposeRun(ctx context.Context, service string) error {
	if err := d.WriteCompose(ctx); err != nil {
		return err
	}
	args := []string{
		"compose", "-f", composeFilePath(),
		"run", "--rm", service,
	}
	//nolint:gosec // Arguments are controlled.
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return xerrors.Errorf("docker compose run %s: %w", service, err)
	}
	return nil
}

// DockerComposeStop runs `docker compose stop` for the given services.
func (d *Docker) DockerComposeStop(ctx context.Context, services ...string) error {
	args := []string{"compose", "-f", composeFilePath(), "stop"}
	args = append(args, services...)
	//nolint:gosec // Arguments are controlled.
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return xerrors.Errorf("docker compose stop: %w", err)
	}
	return nil
}
