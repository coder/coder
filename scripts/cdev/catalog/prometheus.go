package catalog

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ory/dockertest/v3/docker"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/serpent"
)

const (
	prometheusImage  = "prom/prometheus"
	prometheusTag    = "latest"
	prometheusUIPort = 9090
)

// PrometheusResult contains connection info for the running
// Prometheus instance.
type PrometheusResult struct {
	// URL is the base URL for the Prometheus UI.
	URL string
}

var _ Service[PrometheusResult] = (*Prometheus)(nil)
var _ ConfigurableService = (*Prometheus)(nil)

// OnPrometheus returns the service name for the Prometheus service.
func OnPrometheus() string {
	return (&Prometheus{}).Name()
}

// Prometheus runs a Prometheus container that scrapes coderd metrics.
type Prometheus struct {
	enabled bool
	result  PrometheusResult
}

// NewPrometheus creates a new Prometheus service.
func NewPrometheus() *Prometheus {
	return &Prometheus{}
}

// Enabled returns whether the Prometheus service is enabled.
func (p *Prometheus) Enabled() bool { return p.enabled }

func (*Prometheus) Name() string {
	return "prometheus"
}

func (*Prometheus) Emoji() string {
	return "📊"
}

func (*Prometheus) DependsOn() []string {
	return []string{OnDocker(), OnCoderd()}
}

func (p *Prometheus) Options() serpent.OptionSet {
	return serpent.OptionSet{{
		Name:        "Prometheus",
		Description: "Enable Prometheus metrics collection.",
		Flag:        "prometheus",
		Env:         "CDEV_PROMETHEUS",
		Default:     "false",
		Value:       serpent.BoolOf(&p.enabled),
	}}
}

// generateConfig builds a prometheus.yml scrape config targeting
// each coderd HA instance's metrics endpoint.
func generateConfig(haCount int) string {
	var targets []string
	for i := range haCount {
		targets = append(targets, fmt.Sprintf("\"localhost:%d\"", PrometheusPortNum(i)))
	}

	return fmt.Sprintf(`global:
  scrape_interval: 15s

scrape_configs:
  - job_name: "coder"
    static_configs:
      - targets: [%s]
`, strings.Join(targets, ", "))
}

func (p *Prometheus) Start(ctx context.Context, logger slog.Logger, cat *Catalog) error {
	dkr, ok := cat.MustGet(OnDocker()).(*Docker)
	if !ok {
		return xerrors.New("unexpected type for Docker service")
	}
	pool := dkr.Result()

	coderd, ok := cat.MustGet(OnCoderd()).(*Coderd)
	if !ok {
		return xerrors.New("unexpected type for Coderd service")
	}

	labels := NewServiceLabels(CDevPrometheus)

	// Create a volume for Prometheus config and data.
	vol, err := dkr.EnsureVolume(ctx, VolumeOptions{
		Name:   "cdev_prometheus",
		Labels: labels,
		// Prometheus runs as nobody (65534) inside the container.
		UID: 65534,
		GID: 65534,
	})
	if err != nil {
		return xerrors.Errorf("ensure prometheus volume: %w", err)
	}

	// Generate the scrape config based on HA count.
	haCount := int(coderd.HACount())
	if haCount < 1 {
		haCount = 1
	}
	configYAML := generateConfig(haCount)

	// Write config to volume using a short-lived busybox container.
	logger.Info(ctx, "writing prometheus config", slog.F("ha_count", haCount))
	_, err = RunContainer(ctx, pool, CDevPrometheus, ContainerRunOptions{
		CreateOpts: docker.CreateContainerOptions{
			Name: "cdev_prometheus_init",
			Config: &docker.Config{
				Image: prometheusImage + ":" + prometheusTag,
				Cmd: []string{
					"sh", "-c",
					fmt.Sprintf(
						"mkdir -p /vol/config /vol/data && chown 65534:65534 /vol/data && printf '%%s' '%s' > /vol/config/prometheus.yml",
						strings.ReplaceAll(configYAML, "'", "'\"'\"'"),
					),
				},
				Labels: labels,
			},
			HostConfig: &docker.HostConfig{
				Binds: []string{
					fmt.Sprintf("%s:/vol", vol.Name),
				},
			},
		},
		Logger:          logger,
		DestroyExisting: true,
	})
	if err != nil {
		return xerrors.Errorf("write prometheus config: %w", err)
	}

	// Start Prometheus container.
	logger.Info(ctx, "starting prometheus container")

	cntSink := NewLoggerSink(cat.w, p)
	cntLogger := slog.Make(cntSink)
	defer cntSink.Close()

	_, err = RunContainer(ctx, pool, CDevPrometheus, ContainerRunOptions{
		CreateOpts: docker.CreateContainerOptions{
			Name: "cdev_prometheus",
			Config: &docker.Config{
				Image: prometheusImage + ":" + prometheusTag,
				Cmd: []string{
					"--config.file=/prom-vol/config/prometheus.yml",
					"--storage.tsdb.path=/prom-vol/data",
					fmt.Sprintf("--web.listen-address=0.0.0.0:%d", prometheusUIPort),
				},
				Labels:       labels,
				ExposedPorts: map[docker.Port]struct{}{docker.Port(fmt.Sprintf("%d/tcp", prometheusUIPort)): {}},
			},
			HostConfig: &docker.HostConfig{
				NetworkMode: "host",
				Binds: []string{
					fmt.Sprintf("%s:/prom-vol", vol.Name),
				},
				PortBindings: map[docker.Port][]docker.PortBinding{
					docker.Port(fmt.Sprintf("%d/tcp", prometheusUIPort)): {
						{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", prometheusUIPort)},
					},
				},
			},
		},
		Logger:          cntLogger,
		Detached:        true,
		DestroyExisting: true,
	})
	if err != nil {
		return xerrors.Errorf("run prometheus container: %w", err)
	}

	p.result = PrometheusResult{
		URL: fmt.Sprintf("http://localhost:%d", prometheusUIPort),
	}

	return p.waitForReady(ctx, logger)
}

func (p *Prometheus) waitForReady(ctx context.Context, logger slog.Logger) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(60 * time.Second)
	client := &http.Client{Timeout: 2 * time.Second}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return xerrors.New("timeout waiting for prometheus to be ready")
		case <-ticker.C:
			readyURL := fmt.Sprintf("http://localhost:%d/-/ready", prometheusUIPort)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, readyURL, nil)
			if err != nil {
				continue
			}
			resp, err := client.Do(req)
			if err != nil {
				continue
			}
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				logger.Info(ctx, "prometheus is ready",
					slog.F("url", p.result.URL),
				)
				return nil
			}
		}
	}
}

func (*Prometheus) Stop(_ context.Context) error {
	return nil
}

func (p *Prometheus) Result() PrometheusResult {
	return p.result
}
