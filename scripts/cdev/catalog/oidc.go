package catalog

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"

	"cdr.dev/slog/v3"
)

const (
	testidpImage     = "cdev-testidp"
	testidpTag       = "latest"
	testidpPort      = "4500/tcp"
	testidpClientID  = "static-client-id"
	testidpClientSec = "static-client-secret"
)

// OIDCResult contains the connection info for the running OIDC IDP.
type OIDCResult struct {
	// IssuerURL is the OIDC issuer URL.
	IssuerURL string
	// ClientID is the OIDC client ID.
	ClientID string
	// ClientSecret is the OIDC client secret.
	ClientSecret string
	// Port is the host port mapped to the container's 4500.
	Port string
}

var _ Service[OIDCResult] = (*OIDC)(nil)

func OnOIDC() string {
	return (&OIDC{}).Name()
}

// OIDC runs a fake OIDC identity provider in a Docker container using testidp.
type OIDC struct {
	containerID string
	result      OIDCResult
	pool        *dockertest.Pool
}

func NewOIDC() *OIDC {
	return &OIDC{}
}

func (o *OIDC) Name() string {
	return "oidc"
}

func (o *OIDC) Emoji() string {
	return "🔒"
}

func (o *OIDC) DependsOn() []string {
	return []string{
		OnDocker(),
	}
}

func (o *OIDC) Start(ctx context.Context, c *Catalog) error {
	logger := c.Logger()
	d := c.MustGet(OnDocker()).(*Docker)
	o.pool = d.Result()

	labels := NewServiceLabels(CDevOIDC).With(CDevLabelEphemeral, "true")

	// Build the testidp image from the Dockerfile.
	if err := o.buildImage(ctx, logger); err != nil {
		return fmt.Errorf("build testidp image: %w", err)
	}

	logger.Info(ctx, "starting oidc container")

	cntSink := NewLoggerSink(c.w, o)
	cntLogger := slog.Make(cntSink)
	defer cntSink.Close()

	// Start new container (ephemeral, will be removed on stop).
	result, err := RunContainer(ctx, o.pool, CDevOIDC, ContainerRunOptions{
		CreateOpts: docker.CreateContainerOptions{
			Name: fmt.Sprintf("cdev_oidc_%d", time.Now().UnixNano()),
			Config: &docker.Config{
				Image: testidpImage + ":" + testidpTag,
				Cmd: []string{
					"-client-id", testidpClientID,
					"-client-sec", testidpClientSec,
				},
				Labels:       labels,
				ExposedPorts: map[docker.Port]struct{}{testidpPort: {}},
			},
			HostConfig: &docker.HostConfig{
				AutoRemove: true,
				PortBindings: map[docker.Port][]docker.PortBinding{
					testidpPort: {{HostIP: "127.0.0.1", HostPort: ""}},
				},
			},
		},
		Logger:   cntLogger,
		Detached: true,
	})
	if err != nil {
		return fmt.Errorf("run container: %w", err)
	}

	// The networking port takes some time to be available.
	timeout, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	for {
		if timeout.Err() != nil {
			return fmt.Errorf("timeout waiting for oidc container to start: %w", timeout.Err())
		}
		if len(result.Container.NetworkSettings.Ports["4500/tcp"]) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	o.containerID = result.Container.ID
	hostPort := result.Container.NetworkSettings.Ports["4500/tcp"][0].HostPort
	o.result = OIDCResult{
		IssuerURL:    fmt.Sprintf("http://localhost:%s", hostPort),
		ClientID:     testidpClientID,
		ClientSecret: testidpClientSec,
		Port:         hostPort,
	}

	return o.waitForReady(ctx, logger)
}

func (o *OIDC) buildImage(ctx context.Context, logger slog.Logger) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	logger.Info(ctx, "building testidp image")

	labels := NewServiceLabels(CDevOIDC)

	// Use docker CLI directly because go-dockerclient doesn't handle BuildKit
	// output properly (Docker 23+ uses BuildKit by default).
	args := []string{
		"build",
		"-f", "scripts/testidp/Dockerfile.testidp",
		"-t", testidpImage + ":" + testidpTag,
	}
	for k, v := range labels {
		args = append(args, "--label", k+"="+v)
	}
	args = append(args, cwd)

	//nolint:gosec // Arguments are controlled, not arbitrary user input.
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func (o *OIDC) waitForReady(ctx context.Context, logger slog.Logger) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(60 * time.Second)
	client := &http.Client{Timeout: 2 * time.Second}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for oidc to be ready")
		case <-ticker.C:
			// Check the well-known endpoint.
			resp, err := client.Get(o.result.IssuerURL + "/.well-known/openid-configuration")
			if err != nil {
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				logger.Info(ctx, "oidc is ready",
					slog.F("issuer_url", o.result.IssuerURL),
					slog.F("client_id", o.result.ClientID),
				)
				return nil
			}
		}
	}
}

func (o *OIDC) Stop(_ context.Context) error {
	if o.containerID == "" || o.pool == nil {
		return nil
	}

	// Container has AutoRemove set, so just stop it.
	return o.pool.Client.StopContainer(o.containerID, 5)
}

func (o *OIDC) Result() OIDCResult {
	return o.result
}
