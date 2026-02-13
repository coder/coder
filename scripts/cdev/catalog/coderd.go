package catalog

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

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

func OnCoderd() ServiceName {
	return (&Coderd{}).Name()
}

// Coderd runs the Coder server inside a Docker container via compose.
type Coderd struct {
	currentStep atomic.Pointer[string]
	haCount     int64

	// ExtraEnv contains additional "KEY=VALUE" environment variables
	// for the coderd container, set by Configure callbacks.
	ExtraEnv []string
	// ExtraArgs contains additional CLI arguments for the coderd
	// server command, set by Configure callbacks.
	ExtraArgs []string

	result CoderdResult
	logger slog.Logger
	dkr    *Docker
}

func (c *Coderd) CurrentStep() string {
	if s := c.currentStep.Load(); s != nil {
		return *s
	}
	return ""
}

func (c *Coderd) URL() string {
	return c.result.URL
}

func (c *Coderd) setStep(step string) {
	c.currentStep.Store(&step)
}

func NewCoderd() *Coderd {
	return &Coderd{}
}

func (*Coderd) Name() ServiceName {
	return CDevCoderd
}
func (*Coderd) Emoji() string {
	return "🖥️"
}

// HACount returns the number of coderd instances configured for HA.
func (c *Coderd) HACount() int64 { return c.haCount }

func (*Coderd) DependsOn() []ServiceName {
	return []ServiceName{
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

func OnBuildSlim() ServiceName {
	return (&BuildSlim{}).Name()
}

func (c *Coderd) Start(ctx context.Context, logger slog.Logger, cat *Catalog) error {
	defer c.setStep("")

	c.logger = logger
	dkr, ok := cat.MustGet(OnDocker()).(*Docker)
	if !ok {
		return xerrors.New("unexpected type for Docker service")
	}
	c.dkr = dkr

	oidc, ok := cat.MustGet(OnOIDC()).(*OIDC)
	if !ok {
		return xerrors.New("unexpected type for OIDC service")
	}

	// Get current working directory for mounting.
	cwd, err := os.Getwd()
	if err != nil {
		return xerrors.Errorf("get working directory: %w", err)
	}

	// Get docker socket path.
	dockerSocket := os.Getenv("DOCKER_SOCKET")
	if dockerSocket == "" {
		dockerSocket = "/var/run/docker.sock"
	}

	// Get docker group ID for socket access.
	dockerGroup := os.Getenv("DOCKER_GROUP")
	if dockerGroup == "" {
		dockerGroup = getDockerGroupID()
	}

	// Register each HA instance as a compose service.
	var serviceNames []string
	for i := range c.haCount {
		index := int(i)
		name := fmt.Sprintf("coderd-%d", index)
		serviceNames = append(serviceNames, name)

		c.setStep(fmt.Sprintf("Registering coderd-%d compose service", index))
		logger.Info(ctx, "registering coderd instance", slog.F("index", index))

		port := coderdPortNum(index)
		pprofPort := PprofPortNum(index)
		prometheusPort := PrometheusPortNum(index)
		accessURL := fmt.Sprintf("http://localhost:%d", port)
		wildcardAccessURL := fmt.Sprintf("*.localhost:%d", port)

		volName := fmt.Sprintf("coderv2_config_%d", index)
		dkr.SetComposeVolume(volName, ComposeVolume{})

		env := map[string]string{
			"CODER_PG_CONNECTION_URL":             "postgresql://coder:coder@database:5432/coder?sslmode=disable",
			"CODER_HTTP_ADDRESS":                  "0.0.0.0:3000",
			"CODER_ACCESS_URL":                    accessURL,
			"CODER_WILDCARD_ACCESS_URL":           wildcardAccessURL,
			"CODER_SWAGGER_ENABLE":                "true",
			"CODER_DANGEROUS_ALLOW_CORS_REQUESTS": "true",
			"CODER_TELEMETRY_ENABLE":              "false",
			"GOMODCACHE":                          "/go-cache/mod",
			"GOCACHE":                             "/go-cache/build",
			"CODER_CACHE_DIRECTORY":               "/cache",
			"DOCKER_HOST":                         fmt.Sprintf("unix://%s", dockerSocket),
			"CODER_PPROF_ENABLE":                  "true",
			"CODER_PPROF_ADDRESS":                 fmt.Sprintf("0.0.0.0:%d", pprofPort),
			"CODER_PROMETHEUS_ENABLE":             "true",
			"CODER_PROMETHEUS_ADDRESS":            fmt.Sprintf("0.0.0.0:%d", prometheusPort),
		}
		for _, kv := range c.ExtraEnv {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) == 2 {
				env[parts[0]] = parts[1]
			}
		}

		cmd := []string{
			"go", "run", "./enterprise/cmd/coder", "server",
			"--http-address", "0.0.0.0:3000",
			"--access-url", accessURL,
			"--wildcard-access-url", wildcardAccessURL,
			"--swagger-enable",
			"--dangerous-allow-cors-requests=true",
			"--enable-terraform-debug-mode",
			"--pprof-enable",
			"--pprof-address", fmt.Sprintf("0.0.0.0:%d", pprofPort),
			"--prometheus-enable",
			"--prometheus-address", fmt.Sprintf("0.0.0.0:%d", prometheusPort),
			"--oidc-issuer-url", oidc.Result().IssuerURL,
			"--oidc-client-id", oidc.Result().ClientID,
			"--oidc-client-secret", oidc.Result().ClientSecret,
		}
		cmd = append(cmd, c.ExtraArgs...)

		depends := map[string]ComposeDependsOn{
			"database":   {Condition: "service_healthy"},
			"build-slim": {Condition: "service_completed_successfully"},
		}

		dkr.SetCompose(name, ComposeService{
			Image:       dogfoodImage + ":" + dogfoodTag,
			WorkingDir:  "/app",
			Networks:    []string{composeNetworkName},
			GroupAdd:    []string{dockerGroup},
			Environment: env,
			Command:     cmd,
			Ports: []string{
				fmt.Sprintf("%d:3000", port),
				fmt.Sprintf("%d:%d", pprofPort, pprofPort),
				fmt.Sprintf("%d:%d", prometheusPort, prometheusPort),
			},
			Volumes: []string{
				fmt.Sprintf("%s:/app", cwd),
				"go_cache:/go-cache",
				"coder_cache:/cache",
				fmt.Sprintf("%s:/home/coder/.config/coderv2", volName),
				fmt.Sprintf("%s:%s", dockerSocket, dockerSocket),
			},
			DependsOn: depends,
			Restart:   "unless-stopped",
			Labels:    composeServiceLabels("coderd"),
			Healthcheck: &ComposeHealthcheck{
				Test:        []string{"CMD-SHELL", "curl -sf http://localhost:3000/api/v2/buildinfo || exit 1"},
				Interval:    "5s",
				Timeout:     "5s",
				Retries:     60,
				StartPeriod: "120s",
			},
		})
	}

	c.setStep("Starting coderd via compose")
	if err := dkr.DockerComposeUp(ctx, serviceNames...); err != nil {
		return xerrors.Errorf("docker compose up coderd: %w", err)
	}

	port := coderdPortNum(0)
	c.result = CoderdResult{
		URL:  fmt.Sprintf("http://localhost:%d", port),
		Port: fmt.Sprintf("%d", port),
	}

	c.setStep("Inserting license if set")
	logger.Info(ctx, "inserting license for coderd", slog.F("ha_count", c.haCount))
	if err := EnsureLicense(ctx, logger, cat); err != nil {
		if c.haCount > 1 {
			// Ensure license is present for HA deployments.
			return xerrors.Errorf("ensure license: %w", err)
		}
	}

	c.setStep("Waiting for coderd to be ready")
	return c.waitForReady(ctx, logger)
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

func (c *Coderd) Stop(ctx context.Context) error {
	if c.dkr == nil {
		return nil
	}
	return c.dkr.DockerComposeStop(ctx, "coderd-0")
}

func (c *Coderd) Result() CoderdResult {
	return c.result
}

// getDockerGroupID returns the GID of the docker group via getent,
// falling back to "999" if the lookup fails.
func getDockerGroupID() string {
	out, err := exec.Command("getent", "group", "docker").Output()
	if err == nil {
		// Format is "docker:x:GID:users", we want the third field.
		parts := strings.Split(strings.TrimSpace(string(out)), ":")
		if len(parts) >= 3 {
			return parts[2]
		}
	}
	return "999"
}
