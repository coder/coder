package catalog

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
)

const (
	// Docker image used for building.
	dogfoodImage = "codercom/oss-dogfood"
	dogfoodTag   = "latest"
)

// BuildSlim builds the slim Coder binaries inside a Docker container.
type BuildSlim struct {
	// Verbose enables verbose output from the build.
	Verbose bool

	pool *dockertest.Pool
}

func NewBuildSlim() *BuildSlim {
	return &BuildSlim{
		Verbose: true, // Default to verbose for dev experience.
	}
}

func (b *BuildSlim) Name() string {
	return "build-slim"
}

func (b *BuildSlim) DependsOn() []string {
	return []string{
		OnDocker(),
	}
}

func (b *BuildSlim) Start(ctx context.Context, c *Catalog) error {
	dkr := c.MustGet(OnDocker()).(*Docker)
	goCache, err := dkr.EnsureVolume(ctx, VolumeOptions{
		Name:   "cdev_go_cache",
		Labels: map[string]string{CDevLabel: "true", CDevLabelCache: "true"},
		UID:    1000, GID: 1000,
	})
	if err != nil {
		return fmt.Errorf("failed to ensure go cache volume: %w", err)
	}
	coderCache, err := dkr.EnsureVolume(ctx, VolumeOptions{
		Name:   "cdev_coder_cache",
		Labels: map[string]string{CDevLabel: "true", CDevLabelCache: "true"},
		UID:    1000, GID: 1000,
	})
	if err != nil {
		return fmt.Errorf("failed to ensure coder cache volume: %w", err)
	}
	pool := dkr.Result()

	// Get current working directory for mounting.
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
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

	// Configure container run options.
	runOpts := &dockertest.RunOptions{
		Repository: dogfoodImage,
		Tag:        dogfoodTag,
		WorkingDir: "/app",
		Env: []string{
			"GOMODCACHE=/go-cache/mod",
			"GOCACHE=/go-cache/build",
			fmt.Sprintf("DOCKER_HOST=unix://%s", dockerSocket),
		},
		Mounts: []string{
			fmt.Sprintf("%s:/app", cwd),
			fmt.Sprintf("%s:/go-cache", goCache.Name),
			fmt.Sprintf("%s:/cache", coderCache.Name),
			fmt.Sprintf("%s:%s", dockerSocket, dockerSocket),
		},
		Cmd:    []string{"sh", "-c", buildCmd},
		Labels: map[string]string{"cdev": "true"},
	}

	// Set up output handling.
	var stdout, stderr bytes.Buffer
	var stdoutWriter, stderrWriter io.Writer = &stdout, &stderr
	if b.Verbose {
		stdoutWriter = io.MultiWriter(&stdout, os.Stdout)
		stderrWriter = io.MultiWriter(&stderr, os.Stderr)
	}
	fmt.Println("🔨 Building slim binaries...")

	// Create container (don't start yet, so we can attach first).
	container, err := pool.Client.CreateContainer(docker.CreateContainerOptions{
		Config: &docker.Config{
			Image:        dogfoodImage + ":" + dogfoodTag,
			WorkingDir:   "/app",
			Env:          runOpts.Env,
			Cmd:          runOpts.Cmd,
			Labels:       runOpts.Labels,
			AttachStdout: true,
			AttachStderr: true,
		},
		HostConfig: &docker.HostConfig{
			Binds:       runOpts.Mounts,
			GroupAdd:    []string{dockerGroup},
			NetworkMode: "host",
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create build container: %w", err)
	}

	defer func() {
		_ = pool.Client.RemoveContainer(docker.RemoveContainerOptions{
			ID:    container.ID,
			Force: true,
		})
	}()

	// Attach BEFORE starting to capture all output from the beginning.
	attachDone := make(chan error, 1)
	go func() {
		attachDone <- pool.Client.AttachToContainer(docker.AttachToContainerOptions{
			Container:    container.ID,
			OutputStream: stdoutWriter,
			ErrorStream:  stderrWriter,
			Stdout:       true,
			Stderr:       true,
			Stream:       true,
		})
	}()

	// Start the container.
	if err := pool.Client.StartContainer(container.ID, nil); err != nil {
		return fmt.Errorf("failed to start build container: %w", err)
	}

	// Wait for container to complete.
	exitCode, err := pool.Client.WaitContainerWithContext(container.ID, ctx)
	if err != nil {
		return fmt.Errorf("failed waiting for build: %w", err)
	}

	// Wait for attach to finish (ensures all logs are flushed).
	<-attachDone

	if exitCode != 0 {
		return fmt.Errorf("build failed with exit code %d", exitCode)
	}

	fmt.Println("✅ Slim binaries built successfully")
	return nil
}

func (b *BuildSlim) Stop(_ context.Context) error {
	// Build is a one-shot task, nothing to stop.
	return nil
}
