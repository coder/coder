package catalog

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/ory/dockertest/v3/docker"
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

// Site runs the Coder frontend dev server inside a Docker container.
type Site struct {
	currentStep atomic.Pointer[string]
	containerID string
	result      SiteResult
}

func (s *Site) CurrentStep() string {
	if st := s.currentStep.Load(); st != nil {
		return *st
	}
	return ""
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
	pool := dkr.Result()

	labels := NewServiceLabels(CDevSite)

	// Get coderd result for the backend URL.
	coderd, ok := c.MustGet(OnCoderd()).(*Coderd)
	if !ok {
		return xerrors.New("unexpected type for Coderd service")
	}
	coderdResult := coderd.Result()

	// Ensure node_modules volume exists.
	nodeModulesVol, err := dkr.EnsureVolume(ctx, VolumeOptions{
		Name:   "cdev_site_node_modules",
		Labels: labels,
		UID:    1000, GID: 1000,
	})
	if err != nil {
		return xerrors.Errorf("ensure site node_modules volume: %w", err)
	}

	// Get current working directory for mounting.
	cwd, err := os.Getwd()
	if err != nil {
		return xerrors.Errorf("get working directory: %w", err)
	}

	portStr := fmt.Sprintf("%d", sitePort)

	s.setStep("Starting frontend dev server")
	logger.Info(ctx, "starting site dev server container", slog.F("port", sitePort))

	cntSink := NewLoggerSink(c.w, s)
	cntLogger := slog.Make(cntSink)
	defer cntSink.Close()

	// The backend URL needs to be accessible from inside the container.
	// Since we're using host networking, localhost works.
	coderHost := coderdResult.URL

	env := []string{
		fmt.Sprintf("CODER_HOST=%s", coderHost),
	}

	// Command to install dependencies and start the dev server.
	s.setStep("Installing pnpm dependencies")
	cmd := []string{
		"sh", "-c",
		"pnpm install --frozen-lockfile && pnpm dev --host",
	}

	// Start new container.
	result, err := RunContainer(ctx, pool, CDevSite, ContainerRunOptions{
		CreateOpts: docker.CreateContainerOptions{
			Name: "cdev_site",
			Config: &docker.Config{
				Image:      dogfoodImage + ":" + dogfoodTag,
				WorkingDir: "/app/site",
				Env:        env,
				Cmd:        cmd,
				Labels:     labels,
				ExposedPorts: map[docker.Port]struct{}{
					docker.Port(portStr + "/tcp"): {},
				},
			},
			HostConfig: &docker.HostConfig{
				Binds: []string{
					fmt.Sprintf("%s/site:/app/site", cwd),
					fmt.Sprintf("%s:/app/site/node_modules", nodeModulesVol.Name),
				},
				NetworkMode:   "host",
				RestartPolicy: docker.RestartPolicy{Name: "unless-stopped"},
				PortBindings: map[docker.Port][]docker.PortBinding{
					docker.Port(portStr + "/tcp"): {{HostIP: "0.0.0.0", HostPort: portStr}},
				},
			},
		},
		Logger:          cntLogger,
		Detached:        true,
		DestroyExisting: true,
	})
	if err != nil {
		return xerrors.Errorf("run container: %w", err)
	}

	s.containerID = result.Container.ID
	s.result = SiteResult{
		URL:  fmt.Sprintf("http://localhost:%d", sitePort),
		Port: portStr,
	}

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
