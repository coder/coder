package catalog

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/ory/dockertest/v3/docker"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/serpent"
)

const (
	coderdBasePort     = 3000
	pprofBasePort      = 6060
	prometheusBasePort = 2112
)

// PprofPortNum returns the pprof port number for a given coderd
// instance index. Instance 0 uses port 6060, instance 1 uses 6061,
// etc.
func PprofPortNum(index int) int {
	return pprofBasePort + index
}

// PrometheusPortNum returns the Prometheus metrics port number for a
// given coderd instance index. Instance 0 uses port 2112, instance 1
// uses 2113, etc.
func PrometheusPortNum(index int) int {
	return prometheusBasePort + index
}

// coderdPortNum returns the port number for a given coderd instance index.
// Instance 0 uses port 3000, instance 1 uses 3001, etc.
func coderdPortNum(index int) int {
	return coderdBasePort + index
}

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
	haCount int64

	// ExtraEnv contains additional "KEY=VALUE" environment variables
	// for the coderd container, set by Configure callbacks.
	ExtraEnv []string
	// ExtraArgs contains additional CLI arguments for the coderd
	// server command, set by Configure callbacks.
	ExtraArgs []string

	containerID string
	result      CoderdResult
}

func NewCoderd() *Coderd {
	return &Coderd{}
}

func (*Coderd) Name() string {
	return "coderd"
}
func (*Coderd) Emoji() string {
	return "🖥️"
}

// HACount returns the number of coderd instances configured for HA.
func (c *Coderd) HACount() int64 { return c.haCount }

func (*Coderd) DependsOn() []string {
	return []string{
		OnDocker(),
		OnPostgres(),
		OnBuildSlim(),
		OnOIDC(),
	}
}

func (c *Coderd) Options() serpent.OptionSet {
	return serpent.OptionSet{
		{
			Name:        "Coderd HA Count",
			Description: "Number of coderd instances to run in HA mode.",
			Required:    false,
			Flag:        "coderd-count",
			Env:         "CDEV_CODERD_COUNT",
			Default:     "1",
			Value:       serpent.Int64Of(&c.haCount),
		},
	}
}

func OnBuildSlim() string {
	return (&BuildSlim{}).Name()
}

func (c *Coderd) Start(ctx context.Context, logger slog.Logger, cat *Catalog) error {
	dkr, ok := cat.MustGet(OnDocker()).(*Docker)
	if !ok {
		return xerrors.New("unexpected type for Docker service")
	}
	pool := dkr.Result()

	labels := NewServiceLabels(CDevCoderd)
	filter := labels.Filter()

	// Kill any existing coderd containers
	containers, err := pool.Client.ListContainers(docker.ListContainersOptions{
		Filters: filter,
	})
	if err != nil {
		return xerrors.Errorf("list containers: %w", err)
	}

	for _, cnt := range containers {
		logger.Info(ctx, "removing existing coderd container", slog.F("id", cnt.ID), slog.F("names", cnt.Names))
		if err := pool.Client.RemoveContainer(docker.RemoveContainerOptions{
			ID:            cnt.ID,
			Force:         true,
			RemoveVolumes: true,
		}); err != nil {
			return xerrors.Errorf("remove container %s: %w", cnt.ID, err)
		}
	}

	if err := EnsureLicense(ctx, logger, cat); err != nil {
		if c.haCount > 1 {
			// Ensure license is present for HA deployments.
			return xerrors.Errorf("ensure license: %w", err)
		}
	}

	for i := range c.haCount {
		container, err := c.startCoderd(ctx, logger, cat, int(i))
		if err != nil {
			return xerrors.Errorf("start coderd instance %d: %w", i, err)
		}
		if i == 0 {
			// Primary instance uses base port, others use base + index.
			port := coderdPortNum(0)
			c.containerID = container.Container.ID
			c.result = CoderdResult{
				URL:  fmt.Sprintf("http://localhost:%d", port),
				Port: fmt.Sprintf("%d", port),
			}
		}
	}

	return c.waitForReady(ctx, logger)
}

func (c *Coderd) startCoderd(ctx context.Context, logger slog.Logger, cat *Catalog, index int) (*ContainerRunResult, error) {
	dkr, ok := cat.MustGet(OnDocker()).(*Docker)
	if !ok {
		return nil, xerrors.New("unexpected type for Docker service")
	}
	pool := dkr.Result()
	pg, ok := cat.MustGet(OnPostgres()).(*Postgres)
	if !ok {
		return nil, xerrors.New("unexpected type for Postgres service")
	}
	oidc, ok := cat.MustGet(OnOIDC()).(*OIDC)
	if !ok {
		return nil, xerrors.New("unexpected type for OIDC service")
	}
	build := Get[*BuildSlim](cat)

	labels := NewServiceLabels(CDevCoderd)

	// Ensure config volume exists (not created by build step).
	coderConfig, err := dkr.EnsureVolume(ctx, VolumeOptions{
		Name:   fmt.Sprintf("cdev_coderv2_config_%d", index),
		Labels: labels,
		UID:    1000, GID: 1000,
	})
	if err != nil {
		return nil, xerrors.Errorf("ensure coderv2 config volume: %w", err)
	}

	// Get current working directory for mounting.
	cwd, err := os.Getwd()
	if err != nil {
		return nil, xerrors.Errorf("get working directory: %w", err)
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

	// Calculate port for this instance (base port + index).
	port := coderdPortNum(index)
	portStr := fmt.Sprintf("%d", port)
	pprofPort := PprofPortNum(index)
	pprofPortStr := fmt.Sprintf("%d", pprofPort)
	prometheusPort := PrometheusPortNum(index)
	prometheusPortStr := fmt.Sprintf("%d", prometheusPort)
	httpAddress := fmt.Sprintf("0.0.0.0:%d", port)
	accessURL := fmt.Sprintf("http://localhost:%d", port)

	logger.Info(ctx, "starting coderd container", slog.F("index", index), slog.F("port", port))

	cntSink := NewLoggerSink(cat.w, c)
	cntLogger := slog.Make(cntSink)
	defer cntSink.Close()

	env := []string{
		// Use host networking for postgres since it's on localhost.
		fmt.Sprintf("CODER_PG_CONNECTION_URL=%s", pg.Result().URL),
		fmt.Sprintf("CODER_HTTP_ADDRESS=%s", httpAddress),
		fmt.Sprintf("CODER_ACCESS_URL=%s", accessURL),
		"CODER_SWAGGER_ENABLE=true",
		"CODER_DANGEROUS_ALLOW_CORS_REQUESTS=true",
		"CODER_TELEMETRY_ENABLE=false",
		"GOMODCACHE=/go-cache/mod",
		"GOCACHE=/go-cache/build",
		"CODER_CACHE_DIRECTORY=/cache",
		fmt.Sprintf("DOCKER_HOST=unix://%s", dockerSocket),
		"CODER_PPROF_ENABLE=true",
		fmt.Sprintf("CODER_PPROF_ADDRESS=0.0.0.0:%d", PprofPortNum(index)),
		"CODER_PROMETHEUS_ENABLE=true",
		fmt.Sprintf("CODER_PROMETHEUS_ADDRESS=0.0.0.0:%d", PrometheusPortNum(index)),
	}
	env = append(env, c.ExtraEnv...)

	cmd := []string{
		"go", "run", "./enterprise/cmd/coder", "server",
		"--http-address", httpAddress,
		"--access-url", accessURL,
		"--swagger-enable",
		"--dangerous-allow-cors-requests=true",
		"--enable-terraform-debug-mode",
		"--pprof-enable",
		"--pprof-address", fmt.Sprintf("127.0.0.1:%d", PprofPortNum(index)),
		"--prometheus-enable",
		"--prometheus-address", fmt.Sprintf("0.0.0.0:%d", PrometheusPortNum(index)),
		// OIDC configuration from the OIDC service.
		"--oidc-issuer-url", oidc.Result().IssuerURL,
		"--oidc-client-id", oidc.Result().ClientID,
		"--oidc-client-secret", oidc.Result().ClientSecret,
	}
	cmd = append(cmd, c.ExtraArgs...)

	// Start new container.
	result, err := RunContainer(ctx, pool, CDevCoderd, ContainerRunOptions{
		CreateOpts: docker.CreateContainerOptions{
			Name: fmt.Sprintf("cdev_coderd_%d", index),
			Config: &docker.Config{
				Image:      dogfoodImage + ":" + dogfoodTag,
				WorkingDir: "/app",
				Env:        env,
				Cmd:        cmd,
				Labels:     labels,
				ExposedPorts: map[docker.Port]struct{}{
					docker.Port(portStr + "/tcp"):           {},
					docker.Port(pprofPortStr + "/tcp"):      {},
					docker.Port(prometheusPortStr + "/tcp"): {},
				},
			},
			HostConfig: &docker.HostConfig{
				Binds: []string{
					fmt.Sprintf("%s:/app", cwd),
					fmt.Sprintf("%s:/go-cache", build.GoCache.Name),
					fmt.Sprintf("%s:/cache", build.CoderCache.Name),
					fmt.Sprintf("%s:/home/coder/.config/coderv2", coderConfig.Name),
					fmt.Sprintf("%s:%s", dockerSocket, dockerSocket),
				},
				GroupAdd:      []string{dockerGroup},
				NetworkMode:   "host",
				RestartPolicy: docker.RestartPolicy{Name: "unless-stopped"},
				PortBindings: map[docker.Port][]docker.PortBinding{
					docker.Port(portStr + "/tcp"):           {{HostIP: "127.0.0.1", HostPort: portStr}},
					docker.Port(pprofPortStr + "/tcp"):      {{HostIP: "127.0.0.1", HostPort: pprofPortStr}},
					docker.Port(prometheusPortStr + "/tcp"): {{HostIP: "127.0.0.1", HostPort: prometheusPortStr}},
				},
			},
		},
		Logger:   cntLogger,
		Detached: true,
	})
	if err != nil {
		return nil, xerrors.Errorf("run container: %w", err)
	}

	return result, nil
}

func (c *Coderd) waitForReady(ctx context.Context, logger slog.Logger) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Coderd can take a while to start, especially on first run with go run.
	timeout := time.After(5 * time.Minute)
	healthURL := c.result.URL + "/api/v2/buildinfo" // this actually returns when the server is ready, as opposed to healthz

	logger.Info(ctx, "waiting for coderd to be ready", slog.F("health_url", healthURL))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return xerrors.New("timeout waiting for coderd to be ready")
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
			if err != nil {
				continue
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				continue
			}
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				logger.Info(ctx, "coderd server is ready and accepting connections", slog.F("url", c.result.URL))
				return nil
			}
		}
	}
}

func (*Coderd) Stop(_ context.Context) error {
	// Don't stop the container - it persists across runs.
	// Use "cdev down" to fully clean up.
	return nil
}

func (c *Coderd) Result() CoderdResult {
	return c.result
}
