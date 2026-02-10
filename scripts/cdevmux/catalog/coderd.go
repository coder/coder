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
	defaultCoderdAccessURL = "http://127.0.0.1:3000"
)

// Coderd runs the main Coder server.
type Coderd struct {
	AccessURL string
	AGPL      bool

	mu   sync.Mutex
	env  []string
	cmd  *exec.Cmd
	done chan struct{}
}

func NewCoderd() *Coderd {
	return &Coderd{
		AccessURL: defaultCoderdAccessURL,
	}
}

func (c *Coderd) Name() string {
	return CoderdName
}

func (c *Coderd) DependsOn() []string {
	return []string{BuildSlimName, DatabaseName}
}

// SetEnv adds an environment variable to be passed to coderd.
func (c *Coderd) SetEnv(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.env = append(c.env, fmt.Sprintf("%s=%s", key, value))
}

func (c *Coderd) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Use go run to start the server (similar to docker-compose.dev.yml).
	// This allows for faster iteration since slim binaries are pre-built.
	cmdPath := "./enterprise/cmd/coder"
	if c.AGPL {
		cmdPath = "./cmd/coder"
	}

	c.cmd = exec.CommandContext(ctx, "go", "run", cmdPath, "server",
		"--http-address", "0.0.0.0:3000",
		"--access-url", c.AccessURL,
		"--swagger-enable",
		"--dangerous-allow-cors-requests=true",
		"--enable-terraform-debug-mode",
	)
	c.cmd.Env = append(os.Environ(), c.env...)
	c.cmd.Stdout = os.Stdout
	c.cmd.Stderr = os.Stderr

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start coderd: %w", err)
	}

	c.done = make(chan struct{})
	go func() {
		_ = c.cmd.Wait()
		close(c.done)
	}()

	// Wait for the server to be ready.
	return c.waitReady(ctx)
}

func (c *Coderd) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cmd == nil || c.cmd.Process == nil {
		return nil
	}

	if err := c.cmd.Process.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("failed to send interrupt: %w", err)
	}

	// Wait for graceful shutdown or force kill.
	select {
	case <-c.done:
		return nil
	case <-time.After(10 * time.Second):
		_ = c.cmd.Process.Kill()
		return nil
	case <-ctx.Done():
		_ = c.cmd.Process.Kill()
		return ctx.Err()
	}
}

func (c *Coderd) Healthy(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.AccessURL+"/api/v2/buildinfo", nil)
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

func (c *Coderd) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		if c.Healthy(ctx) == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.done:
			return fmt.Errorf("coderd exited unexpectedly")
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("coderd failed to become ready within 2 minutes")
}
