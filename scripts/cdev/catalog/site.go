package catalog

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

const (
	sitePort = 8080
)

// SiteResult contains the connection info for the running Site dev server.
type SiteResult struct {
	// URL is the access URL for the frontend dev server.
	URL string
	// Port is the host port mapped to the container's 8080.
	Port string
}

var _ Service[SiteResult] = (*Site)(nil)

func OnSite() ServiceName {
	return (&Site{}).Name()
}

// Site runs the Coder frontend dev server via docker compose.
type Site struct {
	currentStep atomic.Pointer[string]
	result      SiteResult
}

func (s *Site) CurrentStep() string {
	if st := s.currentStep.Load(); st != nil {
		return *st
	}
	return ""
}

func (s *Site) URL() string {
	return s.result.URL
}

func (s *Site) setStep(step string) {
	s.currentStep.Store(&step)
}

func NewSite() *Site {
	return &Site{}
}

func (*Site) Name() ServiceName {
	return CDevSite
}

func (*Site) Emoji() string {
	return "🌐"
}

func (*Site) DependsOn() []ServiceName {
	return []ServiceName{
		OnDocker(),
		OnSetup(),
	}
}

func (s *Site) Start(ctx context.Context, logger slog.Logger, c *Catalog) error {
	defer s.setStep("")

	dkr, ok := c.MustGet(OnDocker()).(*Docker)
	if !ok {
		return xerrors.New("unexpected type for Docker service")
	}

	// Get coderd result for the backend URL.
	coderd, ok := c.MustGet(OnCoderd()).(*Coderd)
	if !ok {
		return xerrors.New("unexpected type for Coderd service")
	}

	// Get current working directory for mounting.
	cwd, err := os.Getwd()
	if err != nil {
		return xerrors.Errorf("get working directory: %w", err)
	}

	portStr := fmt.Sprintf("%d", sitePort)

	s.setStep("Registering site compose service")
	logger.Info(ctx, "registering site compose service", slog.F("port", sitePort))

	dkr.SetComposeVolume("site_node_modules", ComposeVolume{})
	dkr.SetCompose("site", ComposeService{
		Image:      dogfoodImage + ":" + dogfoodTag,
		Networks:   []string{composeNetworkName},
		WorkingDir: "/app/site",
		Environment: map[string]string{
			"CODER_HOST": fmt.Sprintf("http://coderd-0:3000"),
		},
		Ports: []string{fmt.Sprintf("%s:%s", portStr, portStr)},
		Volumes: []string{
			fmt.Sprintf("%s:/app", cwd),
			"site_node_modules:/app/site/node_modules",
		},
		Command: `sh -c "pnpm install --frozen-lockfile && pnpm dev --host"`,
		DependsOn: map[string]ComposeDependsOn{
			"coderd-0": {Condition: "service_healthy"},
		},
		Restart: "unless-stopped",
		Labels:  composeServiceLabels("site"),
	})

	s.setStep("Starting site via compose")
	if err := dkr.DockerComposeUp(ctx, "site"); err != nil {
		return xerrors.Errorf("docker compose up site: %w", err)
	}

	s.result = SiteResult{
		URL:  fmt.Sprintf("http://localhost:%d", sitePort),
		Port: portStr,
	}

	// Use coderd URL for reference (ensures dep is used).
	_ = coderd.Result()

	s.setStep("Waiting for dev server")
	return s.waitForReady(ctx, logger)
}

func (s *Site) waitForReady(ctx context.Context, logger slog.Logger) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Site dev server can take a while to start, especially on first run
	// with pnpm install.
	timeout := time.After(5 * time.Minute)
	healthURL := s.result.URL

	logger.Info(ctx, "waiting for site dev server to be ready", slog.F("health_url", healthURL))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return xerrors.New("timeout waiting for site dev server to be ready")
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
				logger.Info(ctx, "site dev server is ready and accepting connections", slog.F("url", s.result.URL))
				return nil
			}
		}
	}
}

func (*Site) Stop(_ context.Context) error {
	// Don't stop the container - it persists across runs.
	// Use "cdev down" to fully clean up.
	return nil
}

func (s *Site) Result() SiteResult {
	return s.result
}
