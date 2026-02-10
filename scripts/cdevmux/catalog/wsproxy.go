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
	defaultProxyPort = "3001"
	defaultAccessURL = "http://127.0.0.1:3000"
)

// WSProxy runs an external workspace proxy.
type WSProxy struct {
	Port      string
	ProxyName string

	mu   sync.Mutex
	cmd  *exec.Cmd
	done chan struct{}
}

func NewWSProxy() *WSProxy {
	return &WSProxy{
		Port:      defaultProxyPort,
		ProxyName: "dev-proxy",
	}
}

func (w *WSProxy) Name() string {
	return WSProxyName
}

func (w *WSProxy) DependsOn() []string {
	return []string{CoderdName}
}

func (w *WSProxy) EnablementFlag() string {
	return "--wsproxy"
}

func (w *WSProxy) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// First, register the proxy with coderd to get a token.
	// This would typically use the coder CLI.
	// For now, we assume the binary is already built.
	
	w.cmd = exec.CommandContext(ctx, "./.coderv2/coder", "wsproxy", "server",
		"--http-address", fmt.Sprintf("127.0.0.1:%s", w.Port),
		"--proxy-session-token", os.Getenv("CODER_PROXY_SESSION_TOKEN"),
		"--primary-access-url", defaultAccessURL,
	)
	w.cmd.Env = os.Environ()
	w.cmd.Stdout = os.Stdout
	w.cmd.Stderr = os.Stderr

	if err := w.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start wsproxy: %w", err)
	}

	w.done = make(chan struct{})
	go func() {
		_ = w.cmd.Wait()
		close(w.done)
	}()

	return w.waitReady(ctx)
}

func (w *WSProxy) Stop(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cmd == nil || w.cmd.Process == nil {
		return nil
	}

	if err := w.cmd.Process.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("failed to send interrupt: %w", err)
	}

	select {
	case <-w.done:
		return nil
	case <-time.After(10 * time.Second):
		_ = w.cmd.Process.Kill()
		return nil
	case <-ctx.Done():
		_ = w.cmd.Process.Kill()
		return ctx.Err()
	}
}

func (w *WSProxy) Healthy(ctx context.Context) error {
	url := fmt.Sprintf("http://127.0.0.1:%s/healthz", w.Port)
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

func (w *WSProxy) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if w.Healthy(ctx) == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-w.done:
			return fmt.Errorf("wsproxy exited unexpectedly")
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("wsproxy failed to become ready within 30s")
}
