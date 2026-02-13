package catalog

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"sync/atomic"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

const (
	testidpImage     = "cdev-testidp"
	testidpTag       = "latest"
	testidpPort      = "4500/tcp"
	testidpHostPort  = "4500"
	testidpClientID  = "static-client-id"
	testidpClientSec = "static-client-secret"
	testidpIssuerURL = "http://localhost:4500"
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

func OnOIDC() ServiceName {
	return (&OIDC{}).Name()
}

// OIDC runs a fake OIDC identity provider via docker compose.
type OIDC struct {
	currentStep atomic.Pointer[string]
	result      OIDCResult
	dkr         *Docker
}

func (o *OIDC) CurrentStep() string {
	if s := o.currentStep.Load(); s != nil {
		return *s
	}
	return ""
}

func (o *OIDC) URL() string {
	return o.result.IssuerURL
}

func (o *OIDC) setStep(step string) {
	o.currentStep.Store(&step)
}

func NewOIDC() *OIDC {
	return &OIDC{}
}

func (*OIDC) Name() ServiceName {
	return CDevOIDC
}

func (*OIDC) Emoji() string {
	return "🔒"
}

func (*OIDC) DependsOn() []ServiceName {
	return []ServiceName{
		OnDocker(),
	}
}

func (o *OIDC) Start(ctx context.Context, logger slog.Logger, c *Catalog) error {
	defer o.setStep("")

	d, ok := c.MustGet(OnDocker()).(*Docker)
	if !ok {
		return xerrors.New("unexpected type for Docker service")
	}
	o.dkr = d

	o.setStep("building testidp docker image (this can take awhile)")
	// Build the testidp image from the Dockerfile.
	if err := o.buildImage(ctx, logger); err != nil {
		return xerrors.Errorf("build testidp image: %w", err)
	}

	o.setStep("Registering OIDC compose service")
	logger.Info(ctx, "registering oidc compose service")

	d.SetCompose("oidc", ComposeService{
		Image: testidpImage + ":" + testidpTag,
		Command: []string{
			"-client-id", testidpClientID,
			"-client-sec", testidpClientSec,
			"-issuer", testidpIssuerURL,
		},
		Ports:    []string{"4500:4500"},
		Networks: []string{composeNetworkName},
		Labels:   composeServiceLabels("oidc"),
		Healthcheck: &ComposeHealthcheck{
			Test:     []string{"CMD-SHELL", "curl -sf http://localhost:4500/.well-known/openid-configuration || exit 1"},
			Interval: "2s",
			Timeout:  "5s",
			Retries:  15,
		},
	})

	o.setStep("Starting OIDC via compose")
	if err := d.DockerComposeUp(ctx, "oidc"); err != nil {
		return xerrors.Errorf("docker compose up oidc: %w", err)
	}

	o.result = OIDCResult{
		IssuerURL:    testidpIssuerURL,
		ClientID:     testidpClientID,
		ClientSecret: testidpClientSec,
		Port:         testidpHostPort,
	}

	return o.waitForReady(ctx, logger)
}

func (*OIDC) buildImage(ctx context.Context, logger slog.Logger) error {
	// Check if image already exists.
	//nolint:gosec // Arguments are controlled.
	checkCmd := exec.CommandContext(ctx, "docker", "image", "inspect", testidpImage+":"+testidpTag)
	if err := checkCmd.Run(); err == nil {
		logger.Info(ctx, "testidp image already exists, skipping build")
		return nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return xerrors.Errorf("get working directory: %w", err)
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
	stdoutLog := LogWriter(logger, slog.LevelInfo, "testidp-build")
	stderrLog := LogWriter(logger, slog.LevelWarn, "testidp-build")
	defer stdoutLog.Close()
	defer stderrLog.Close()
	cmd.Stdout = stdoutLog
	cmd.Stderr = stderrLog

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
			return xerrors.New("timeout waiting for oidc to be ready")
		case <-ticker.C:
			// Check the well-known endpoint.
			wellKnownURL := o.result.IssuerURL + "/.well-known/openid-configuration"
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnownURL, nil)
			if err != nil {
				continue
			}
			resp, err := client.Do(req)
			if err != nil {
				continue
			}
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				logger.Info(ctx, "oidc provider is ready and accepting connections",
					slog.F("issuer_url", o.result.IssuerURL),
					slog.F("client_id", o.result.ClientID),
				)
				return nil
			}
		}
	}
}

func (o *OIDC) Stop(ctx context.Context) error {
	if o.dkr == nil {
		return nil
	}
	return o.dkr.DockerComposeStop(ctx, "oidc")
}

func (o *OIDC) Result() OIDCResult {
	return o.result
}
