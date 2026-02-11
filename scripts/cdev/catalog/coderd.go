package catalog

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/ory/dockertest/v3/docker"

	"cdr.dev/slog/v3"
)

const (
	coderdPort = "3000/tcp"
)

// CoderdResult contains the connection info for the running Coderd instance.
type CoderdResult struct {
	// URL is the access URL for the Coder instance.
	URL string
	// Port is the host port mapped to the container's 3000.
	Port string
}

var _ Service[CoderdResult] = (*Coderd)(nil)

func OnCoderd() string {
	return (&Coderd{}).Name()
}

// Coderd runs the Coder server inside a Docker container.
type Coderd struct {
	containerID string
	result      CoderdResult
}

func NewCoderd() *Coderd {
	return &Coderd{}
}

func (c *Coderd) Name() string {
	return "coderd"
}

func (c *Coderd) DependsOn() []string {
	return []string{
		OnDocker(),
		OnPostgres(),
		OnBuildSlim(),
	}
}

func OnBuildSlim() string {
	return (&BuildSlim{}).Name()
}

func (c *Coderd) Start(ctx context.Context, cat *Catalog) error {
	logger := cat.Logger()
	dkr := cat.MustGet(OnDocker()).(*Docker)
	pool := dkr.Result()
	pg := cat.MustGet(OnPostgres()).(*Postgres)
	build := Get[*BuildSlim](cat)

	name := "cdev_coderd"
	labels := NewServiceLabels(CDevCoderd)
	filter := labels.Filter()
	filter["name"] = []string{name}

	// Check if container already exists and is running.
	containers, err := pool.Client.ListContainers(docker.ListContainersOptions{
		Filters: filter,
	})
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}

	if len(containers) > 0 {
		// Reuse existing container.
		container := containers[0]
		c.containerID = container.ID
		for _, port := range container.Ports {
			if port.PrivatePort == 3000 {
				c.result = CoderdResult{
					URL:  fmt.Sprintf("http://localhost:%d", port.PublicPort),
					Port: fmt.Sprintf("%d", port.PublicPort),
				}
				break
			}
		}
		logger.Info(ctx, "reusing existing coderd container", slog.F("container_id", c.containerID[:12]))
		return c.waitForReady(ctx, logger)
	}

	// Reuse volumes from build step.
	goCache := build.GoCache
	coderCache := build.CoderCache

	// Ensure config volume exists (not created by build step).
	coderConfig, err := dkr.EnsureVolume(ctx, VolumeOptions{
		Name:   "cdev_coderv2_config",
		Labels: labels,
		UID:    1000, GID: 1000,
	})
	if err != nil {
		return fmt.Errorf("ensure coderv2 config volume: %w", err)
	}

	// Get current working directory for mounting.
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
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

	logger.Info(ctx, "starting coderd container")

	cntSink := controllableLoggerSink(logger)
	cntLogger := slog.Make(cntSink)
	defer cntSink.Close()

	// Start new container.
	result, err := RunContainer(ctx, pool, CDevCoderd, ContainerRunOptions{
		CreateOpts: docker.CreateContainerOptions{
			Name: name,
			Config: &docker.Config{
				Image:      dogfoodImage + ":" + dogfoodTag,
				WorkingDir: "/app",
				Env: []string{
					// Use host networking for postgres since it's on localhost.
					fmt.Sprintf("CODER_PG_CONNECTION_URL=%s", pg.Result().URL),
					"CODER_HTTP_ADDRESS=0.0.0.0:3000",
					"CODER_ACCESS_URL=http://localhost:3000",
					"CODER_SWAGGER_ENABLE=true",
					"CODER_DANGEROUS_ALLOW_CORS_REQUESTS=true",
					"CODER_TELEMETRY_ENABLE=false",
					"GOMODCACHE=/go-cache/mod",
					"GOCACHE=/go-cache/build",
					"CODER_CACHE_DIRECTORY=/cache",
					fmt.Sprintf("DOCKER_HOST=unix://%s", dockerSocket),
				},
				Cmd: []string{
					"go", "run", "./enterprise/cmd/coder", "server",
					"--http-address", "0.0.0.0:3000",
					"--access-url", "http://localhost:3000",
					"--swagger-enable",
					"--dangerous-allow-cors-requests=true",
					"--enable-terraform-debug-mode",
				},
				Labels:       labels,
				ExposedPorts: map[docker.Port]struct{}{coderdPort: {}},
			},
			HostConfig: &docker.HostConfig{
				Binds: []string{
					fmt.Sprintf("%s:/app", cwd),
					fmt.Sprintf("%s:/go-cache", goCache.Name),
					fmt.Sprintf("%s:/cache", coderCache.Name),
					fmt.Sprintf("%s:/home/coder/.config/coderv2", coderConfig.Name),
					fmt.Sprintf("%s:%s", dockerSocket, dockerSocket),
				},
				GroupAdd:      []string{dockerGroup},
				NetworkMode:   "host",
				RestartPolicy: docker.RestartPolicy{Name: "unless-stopped"},
				PortBindings: map[docker.Port][]docker.PortBinding{
					coderdPort: {{HostIP: "127.0.0.1", HostPort: ""}},
				},
			},
		},
		Logger:   cntLogger,
		Detached: true,
	})
	if err != nil {
		return fmt.Errorf("run container: %w", err)
	}

	c.containerID = result.Container.ID

	// With host networking, port is always 3000.
	c.result = CoderdResult{
		URL:  "http://localhost:3000",
		Port: "3000",
	}

	return c.waitForReady(ctx, logger)
}

func (c *Coderd) waitForReady(ctx context.Context, logger slog.Logger) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Coderd can take a while to start, especially on first run with go run.
	timeout := time.After(5 * time.Minute)
	healthURL := c.result.URL + "/healthz"

	logger.Info(ctx, "waiting for coderd to be ready", slog.F("health_url", healthURL))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for coderd to be ready")
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
			if err != nil {
				continue
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				logger.Info(ctx, "coderd is ready", slog.F("url", c.result.URL))
				return nil
			}
		}
	}
}

func (c *Coderd) Stop(_ context.Context) error {
	// Don't stop the container - it persists across runs.
	// Use "cdev down" to fully clean up.
	return nil
}

func (c *Coderd) Result() CoderdResult {
	return c.result
}
