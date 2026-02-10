package catalog

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"
)

const (
	defaultOIDCPort = "4000"
	fakeOIDCImage   = "ghcr.io/coder/fake-oidc-idp:latest"
)

// OIDCVariant specifies which OIDC provider to use.
type OIDCVariant string

const (
	OIDCVariantFake OIDCVariant = "fake"
	OIDCVariantOkta OIDCVariant = "okta"
)

// OIDC runs an OIDC identity provider for testing authentication.
type OIDC struct {
	Variant OIDCVariant
	Port    string

	mu   sync.Mutex
	cmd  *exec.Cmd
	done chan struct{}
}

func NewOIDC(variant OIDCVariant) *OIDC {
	return &OIDC{
		Variant: variant,
		Port:    defaultOIDCPort,
	}
}

func (o *OIDC) Name() string {
	return OIDCName + "/" + string(o.Variant)
}

func (o *OIDC) DependsOn() []string {
	return nil // OIDC starts independently, but coderd depends on its config.
}

func (o *OIDC) EnablementFlag() string {
	return "--oidc"
}

// Configure sets the OIDC environment variables on coderd.
func (o *OIDC) Configure(c *Catalog) error {
	svc, ok := c.Get(CoderdName)
	if !ok {
		return fmt.Errorf("coderd service not found")
	}
	envSetter, ok := svc.(EnvSetter)
	if !ok {
		return fmt.Errorf("coderd service does not support SetEnv")
	}

	issuerURL := fmt.Sprintf("http://127.0.0.1:%s", o.Port)
	envSetter.SetEnv("CODER_OIDC_ISSUER_URL", issuerURL)
	envSetter.SetEnv("CODER_OIDC_CLIENT_ID", "coder")
	envSetter.SetEnv("CODER_OIDC_CLIENT_SECRET", "coder")

	return nil
}

func (o *OIDC) Start(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	switch o.Variant {
	case OIDCVariantFake:
		return o.startFake(ctx)
	case OIDCVariantOkta:
		return o.startOkta(ctx)
	default:
		return fmt.Errorf("unknown OIDC variant: %s", o.Variant)
	}
}

func (o *OIDC) startFake(ctx context.Context) error {
	containerName := "cdev-oidc"

	// Remove any existing container.
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", containerName).Run()

	cmd := exec.CommandContext(ctx, "docker", "run",
		"-d",
		"--name", containerName,
		"--label", "cdev=true",
		"-p", fmt.Sprintf("%s:8080", o.Port),
		"-e", "OIDC_CLIENT_ID=coder",
		"-e", "OIDC_CLIENT_SECRET=coder",
		fakeOIDCImage,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start fake OIDC: %w: %s", err, out)
	}

	return o.waitReady(ctx)
}

func (o *OIDC) startOkta(ctx context.Context) error {
	// Okta integration would use environment variables for configuration.
	// This is a placeholder for real Okta dev tenant setup.
	oktaIssuer := os.Getenv("OKTA_ISSUER_URL")
	if oktaIssuer == "" {
		return fmt.Errorf("OKTA_ISSUER_URL environment variable required for Okta variant")
	}
	// Okta is external, no process to start.
	return nil
}

func (o *OIDC) Stop(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.Variant == OIDCVariantFake {
		cmd := exec.CommandContext(ctx, "docker", "stop", "cdev-oidc")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to stop fake OIDC: %w: %s", err, out)
		}
	}
	return nil
}

func (o *OIDC) Healthy(ctx context.Context) error {
	url := fmt.Sprintf("http://127.0.0.1:%s/.well-known/openid-configuration", o.Port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

func (o *OIDC) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if o.Healthy(ctx) == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("OIDC provider failed to become ready within 30s")
}
