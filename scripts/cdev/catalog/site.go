package catalog

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/ory/dockertest/v3"
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
	pool        *dockertest.Pool
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
	pool := dkr.Result()
	s.pool = pool

	labels := NewServiceLabels(CDevSite)

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

	networkID, err := dkr.EnsureNetwork(ctx, labels)
	if err != nil {
		return xerrors.Errorf("ensure network: %w", err)
	}

	// The backend URL needs to be accessible from inside the container
	// via the Docker bridge network.
	coderHost := "http://load-balancer:3000"

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
				Labels: labels,
				Healthcheck: &docker.HealthConfig{
					Test:     []string{"CMD-SHELL", fmt.Sprintf("wget -q --spider http://localhost:%d || exit 1", sitePort)},
					Interval: 2 * time.Second,
					Timeout:  2 * time.Second,
					Retries:  3,
				},
				ExposedPorts: map[docker.Port]struct{}{
					docker.Port(portStr + "/tcp"): {},
				},
			},
			HostConfig: &docker.HostConfig{
				Binds: []string{
					// Mount the entire repo root for hot reload support.
					// This allows changes to shared code outside site/ to be picked up.
					fmt.Sprintf("%s:/app", cwd),
					fmt.Sprintf("%s:/app/site/node_modules", nodeModulesVol.Name),
				},
				RestartPolicy: docker.RestartPolicy{Name: "unless-stopped"},
			},
			NetworkingConfig: &docker.NetworkingConfig{
				EndpointsConfig: map[string]*docker.EndpointConfig{
					CDevNetworkName: {
						NetworkID: networkID,
						Aliases:   []string{"site"},
					},
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
	return waitForHealthy(ctx, logger, s.pool, "cdev_site", 5*time.Minute)
}

func (*Site) Stop(_ context.Context) error {
	// Don't stop the container - it persists across runs.
	// Use "cdev down" to fully clean up.
	return nil
}

func (s *Site) Result() SiteResult {
	return s.result
}
