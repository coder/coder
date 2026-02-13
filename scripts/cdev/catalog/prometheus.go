package catalog

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

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
func OnPrometheus() ServiceName {
	return (&Prometheus{}).Name()
}

// Prometheus runs a Prometheus container that scrapes coderd metrics
// via docker compose.
type Prometheus struct {
	currentStep atomic.Pointer[string]
	enabled     bool
	result      PrometheusResult
}

func (p *Prometheus) CurrentStep() string {
	if s := p.currentStep.Load(); s != nil {
		return *s
	}
	return ""
}

func (p *Prometheus) URL() string {
	return p.result.URL
}

func (p *Prometheus) setStep(step string) {
	p.currentStep.Store(&step)
}

// NewPrometheus creates a new Prometheus service.
func NewPrometheus() *Prometheus {
	return &Prometheus{}
}

// Enabled returns whether the Prometheus service is enabled.
func (p *Prometheus) Enabled() bool { return p.enabled }

func (*Prometheus) Name() ServiceName {
	return CDevPrometheus
}

func (*Prometheus) Emoji() string {
	return "📊"
}

func (*Prometheus) DependsOn() []ServiceName {
	return []ServiceName{OnDocker(), OnCoderd()}
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
		targets = append(targets, fmt.Sprintf("\"coderd-%d:2112\"", i))
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
	defer p.setStep("")

	dkr, ok := cat.MustGet(OnDocker()).(*Docker)
	if !ok {
		return xerrors.New("unexpected type for Docker service")
	}

	coderd, ok := cat.MustGet(OnCoderd()).(*Coderd)
	if !ok {
		return xerrors.New("unexpected type for Coderd service")
	}

	// Generate the scrape config based on HA count.
	haCount := int(coderd.HACount())
	if haCount < 1 {
		haCount = 1
	}
	configYAML := generateConfig(haCount)

	dkr.SetComposeVolume("prometheus", ComposeVolume{})

	// Register prometheus-init (one-shot config writer).
	configScript := fmt.Sprintf(
		"mkdir -p /prom-vol/config /prom-vol/data && printf '%%s' '%s' > /prom-vol/config/prometheus.yml",
		strings.ReplaceAll(configYAML, "'", "'\"'\"'"),
	)

	dkr.SetCompose("prometheus-init", ComposeService{
		Image:      prometheusImage + ":" + prometheusTag,
		Entrypoint: []string{"sh", "-c"},
		Command:    configScript,
		Volumes:    []string{"prometheus:/prom-vol"},
		Labels:     composeServiceLabels("prometheus-init"),
	})

	dkr.SetCompose("prometheus", ComposeService{
		Image: prometheusImage + ":" + prometheusTag,
		Command: []string{
			"--config.file=/prom-vol/config/prometheus.yml",
			"--storage.tsdb.path=/prom-vol/data",
			fmt.Sprintf("--web.listen-address=0.0.0.0:%d", prometheusUIPort),
		},
		Ports:    []string{fmt.Sprintf("%d:%d", prometheusUIPort, prometheusUIPort)},
		Networks: []string{composeNetworkName},
		Volumes:  []string{"prometheus:/prom-vol"},
		DependsOn: map[string]ComposeDependsOn{
			"prometheus-init": {Condition: "service_completed_successfully"},
			"coderd-0":        {Condition: "service_healthy"},
		},
		Labels: composeServiceLabels("prometheus"),
		Healthcheck: &ComposeHealthcheck{
			Test:     []string{"CMD-SHELL", fmt.Sprintf("curl -sf http://localhost:%d/-/ready || exit 1", prometheusUIPort)},
			Interval: "2s",
			Timeout:  "5s",
			Retries:  15,
		},
	})

	p.setStep("Starting Prometheus via compose")
	logger.Info(ctx, "starting prometheus via compose")

	if err := dkr.DockerComposeUp(ctx, "prometheus-init", "prometheus"); err != nil {
		return xerrors.Errorf("docker compose up prometheus: %w", err)
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
