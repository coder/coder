package catalog

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/ory/dockertest/v3/docker"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

const (
	// Docker image used for building.
	dogfoodImage = "codercom/oss-dogfood"
	dogfoodTag   = "latest"
)

var _ Service[BuildResult] = (*BuildSlim)(nil)

// BuildSlim builds the slim Coder binaries inside a Docker container.
type BuildSlim struct {
	currentStep atomic.Pointer[string]

	// Verbose enables verbose output from the build.
	Verbose bool

	result BuildResult
}

type BuildResult struct {
	CoderCache *docker.Volume
	GoCache    *docker.Volume
}

func NewBuildSlim() *BuildSlim {
	return &BuildSlim{
		Verbose: true, // Default to verbose for dev experience.
	}
}

func (b *BuildSlim) Result() BuildResult {
	return b.result
}

func (*BuildSlim) Name() ServiceName {
	return CDevBuildSlim
}
func (*BuildSlim) Emoji() string {
	return "🔨"
}

func (*BuildSlim) DependsOn() []ServiceName {
	return []ServiceName{
		OnDocker(),
	}
}

func (b *BuildSlim) CurrentStep() string {
	if s := b.currentStep.Load(); s != nil {
		return *s
	}
	return ""
}

func (b *BuildSlim) setStep(step string) {
	b.currentStep.Store(&step)
}

func (b *BuildSlim) Start(ctx context.Context, logger slog.Logger, c *Catalog) error {
	b.setStep("Initializing Docker volumes")
	dkr, ok := c.MustGet(OnDocker()).(*Docker)
	if !ok {
		return xerrors.New("unexpected type for Docker service")
	}
	goCache, err := dkr.EnsureVolume(ctx, VolumeOptions{
		Name:   "cdev_go_cache",
		Labels: NewServiceLabels(CDevBuildSlim),
		UID:    1000, GID: 1000,
	})
	if err != nil {
		return xerrors.Errorf("failed to ensure go cache volume: %w", err)
	}
	coderCache, err := dkr.EnsureVolume(ctx, VolumeOptions{
		Name:   "cdev_coder_cache",
		Labels: NewServiceLabels(CDevBuildSlim),
		UID:    1000, GID: 1000,
	})
	if err != nil {
		return xerrors.Errorf("failed to ensure coder cache volume: %w", err)
	}
	pool := dkr.Result()

	// Get current working directory for mounting.
	cwd, err := os.Getwd()
	if err != nil {
		return xerrors.Errorf("failed to get working directory: %w", err)
	}

	// Get docker group ID for socket access.
	dockerGroup := os.Getenv("DOCKER_GROUP")
	if dockerGroup == "" {
		dockerGroup = "999"
	}

	// Get docker socket path.
	dockerSocket := os.Getenv("DOCKER_SOCKET")
	if dockerSocket == "" {
		dockerSocket = "/var/run/docker.sock"
	}

	// Build command matching docker-compose.dev.yml.
	buildCmd := `
		make -j build-slim &&
		mkdir -p /cache/site/orig/bin &&
		cp site/out/bin/coder-* /cache/site/orig/bin/ 2>/dev/null || true &&
		echo "Slim binaries built and cached."
	`

	b.setStep("Running make build-slim")
	logger.Info(ctx, "building slim binaries")

	var stdout, stderr bytes.Buffer
	_, err = RunContainer(ctx, pool, CDevBuildSlim, ContainerRunOptions{
		CreateOpts: docker.CreateContainerOptions{
			Name: "cdev-build-slim",
			Config: &docker.Config{
				Image:      dogfoodImage + ":" + dogfoodTag,
				WorkingDir: "/app",
				Env: []string{
					"GOMODCACHE=/go-cache/mod",
					"GOCACHE=/go-cache/build",
					fmt.Sprintf("DOCKER_HOST=unix://%s", dockerSocket),
				},
				Cmd:          []string{"sh", "-c", buildCmd},
				Labels:       NewServiceLabels(CDevBuildSlim),
				AttachStdout: true,
				AttachStderr: true,
			},
			HostConfig: &docker.HostConfig{
				AutoRemove: true,
				Binds: []string{
					fmt.Sprintf("%s:/app", cwd),
					fmt.Sprintf("%s:/go-cache", goCache.Name),
					fmt.Sprintf("%s:/cache", coderCache.Name),
					fmt.Sprintf("%s:%s", dockerSocket, dockerSocket),
				},
				GroupAdd:    []string{dockerGroup},
				NetworkMode: "host",
			},
		},
		Logger: logger,
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return err
	}

	b.setStep("")
	logger.Info(ctx, "slim binaries built successfully")
	b.result.CoderCache = coderCache
	b.result.GoCache = goCache
	return nil
}

func (*BuildSlim) Stop(_ context.Context) error {
	// Build is a one-shot task, nothing to stop.
	return nil
}
