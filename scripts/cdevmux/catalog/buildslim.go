package catalog

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
)

const (
	// Docker image used for building.
	dogfoodImage = "codercom/oss-dogfood"
	dogfoodTag   = "latest"

	// Volume names (prefixed with cdev_ to avoid conflicts).
	VolumeGoCache    = "cdev_go_cache"
	VolumeCoderCache = "cdev_coder_cache"
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
	return BuildSlimName
}

func (b *BuildSlim) DependsOn() []string {
	return nil // No dependencies.
}

func (b *BuildSlim) Start(ctx context.Context) error {
	pool, err := dockertest.NewPool("")
	if err != nil {
		return fmt.Errorf("failed to connect to docker: %w", err)
	}
	b.pool = pool

	// Ensure volumes exist with correct permissions.
	if err := b.ensureVolumes(ctx); err != nil {
		return fmt.Errorf("failed to create volumes: %w", err)
	}

	// Initialize volume permissions (like init-volumes in docker-compose).
	if err := b.initVolumes(ctx); err != nil {
		return fmt.Errorf("failed to init volumes: %w", err)
	}

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
			fmt.Sprintf("%s:/go-cache", VolumeGoCache),
			fmt.Sprintf("%s:/cache", VolumeCoderCache),
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

	// Run the container.
	resource, err := pool.RunWithOptions(runOpts, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.GroupAdd = []string{dockerGroup}
		config.NetworkMode = "host"
	})
	if err != nil {
		return fmt.Errorf("failed to start build container: %w", err)
	}

	// Wait for container to complete.
	exitCode, err := pool.Client.WaitContainerWithContext(resource.Container.ID, ctx)
	if err != nil {
		return fmt.Errorf("failed waiting for build: %w", err)
	}

	// Get logs.
	_ = pool.Client.Logs(docker.LogsOptions{
		Container:    resource.Container.ID,
		OutputStream: stdoutWriter,
		ErrorStream:  stderrWriter,
		Stdout:       true,
		Stderr:       true,
	})

	if exitCode != 0 {
		return fmt.Errorf("build failed with exit code %d:\n%s", exitCode, stderr.String())
	}

	fmt.Println("✅ Slim binaries built successfully")
	return nil
}

func (b *BuildSlim) Stop(_ context.Context) error {
	// Build is a one-shot task, nothing to stop.
	return nil
}

// ensureVolumes creates the required Docker volumes if they don't exist.
func (b *BuildSlim) ensureVolumes(_ context.Context) error {
	volumes := []string{VolumeGoCache, VolumeCoderCache}

	for _, name := range volumes {
		_, err := b.pool.Client.InspectVolume(name)
		if err != nil {
			// Volume doesn't exist, create it.
			_, err = b.pool.Client.CreateVolume(docker.CreateVolumeOptions{
				Name: name,
				Labels: map[string]string{
					CDevLabel:      "true",
					CDevLabelCache: "true",
				},
			})
			if err != nil {
				return fmt.Errorf("failed to create volume %s: %w", name, err)
			}
		}
	}

	return nil
}

// initVolumes sets correct ownership on volumes (like init-volumes in docker-compose).
// Docker creates volumes as root by default, so we chown them to uid 1000 (coder user).
func (b *BuildSlim) initVolumes(ctx context.Context) error {
	initCmd := "chown -R 1000:1000 /go-cache /cache"

	runOpts := &dockertest.RunOptions{
		Repository: dogfoodImage,
		Tag:        dogfoodTag,
		User:       "0:0", // Run as root to chown.
		Mounts: []string{
			fmt.Sprintf("%s:/go-cache", VolumeGoCache),
			fmt.Sprintf("%s:/cache", VolumeCoderCache),
		},
		Cmd: []string{"sh", "-c", initCmd},
		Labels: map[string]string{
			CDevLabel:          "true",
			CDevLabelEphemeral: "true",
		},
	}

	resource, err := b.pool.RunWithOptions(runOpts, func(config *docker.HostConfig) {
		config.AutoRemove = true
	})
	if err != nil {
		return fmt.Errorf("failed to start init container: %w", err)
	}

	exitCode, err := b.pool.Client.WaitContainerWithContext(resource.Container.ID, ctx)
	if err != nil {
		return fmt.Errorf("failed waiting for init: %w", err)
	}

	if exitCode != 0 {
		return fmt.Errorf("init volumes failed with exit code %d", exitCode)
	}

	return nil
}

// VolumeNames returns the names of volumes created by this service.
func (b *BuildSlim) VolumeNames() []string {
	return []string{VolumeGoCache, VolumeCoderCache}
}

// CacheDir returns the path to the coder cache inside the container.
func CacheDir() string {
	return filepath.Join("/cache")
}

// GoCacheDir returns the path to the Go cache inside the container.
func GoCacheDir() string {
	return filepath.Join("/go-cache")
}
