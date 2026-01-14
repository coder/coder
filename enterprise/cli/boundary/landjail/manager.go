//go:build linux

package landjail

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/coder/coder/v2/enterprise/cli/boundary/audit"
	"github.com/coder/coder/v2/enterprise/cli/boundary/config"
	"github.com/coder/coder/v2/enterprise/cli/boundary/proxy"
	"github.com/coder/coder/v2/enterprise/cli/boundary/rulesengine"
)

type LandJail struct {
	proxyServer *proxy.Server
	logger      *slog.Logger
	config      config.AppConfig
}

func NewLandJail(
	ruleEngine rulesengine.Engine,
	auditor audit.Auditor,
	tlsConfig *tls.Config,
	logger *slog.Logger,
	config config.AppConfig,
) (*LandJail, error) {
	// Create proxy server
	proxyServer := proxy.NewProxyServer(proxy.Config{
		HTTPPort:     int(config.ProxyPort),
		RuleEngine:   ruleEngine,
		Auditor:      auditor,
		Logger:       logger,
		TLSConfig:    tlsConfig,
		PprofEnabled: config.PprofEnabled,
		PprofPort:    int(config.PprofPort),
	})

	return &LandJail{
		config:      config,
		proxyServer: proxyServer,
		logger:      logger,
	}, nil
}

func (b *LandJail) Run(ctx context.Context) error {
	b.logger.Info("Start landjail manager")
	err := b.startProxy()
	if err != nil {
		return fmt.Errorf("failed to start landjail manager: %v", err)
	}

	defer func() {
		b.logger.Info("Stop landjail manager")
		err := b.stopProxy()
		if err != nil {
			b.logger.Error("Failed to stop landjail manager", "error", err)
		}
	}()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		defer cancel()
		err := b.RunChildProcess(os.Args)
		if err != nil {
			b.logger.Error("Failed to run child process", "error", err)
		}
	}()

	// Setup signal handling BEFORE any setup
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for signal or context cancellation
	select {
	case sig := <-sigChan:
		b.logger.Info("Received signal, shutting down...", "signal", sig)
		cancel()
	case <-ctx.Done():
		// Context canceled by command completion
		b.logger.Info("Command completed, shutting down...")
	}

	return nil
}

func (b *LandJail) RunChildProcess(command []string) error {
	childCmd := b.getChildCommand(command)

	b.logger.Debug("Executing command in boundary", "command", strings.Join(os.Args, " "))
	err := childCmd.Start()
	if err != nil {
		b.logger.Error("Command failed to start", "error", err)
		return err
	}

	b.logger.Debug("waiting on a child process to finish")
	err = childCmd.Wait()
	if err != nil {
		// Check if this is a normal exit with non-zero status code
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode := exitError.ExitCode()
			// Log at debug level for non-zero exits (normal behavior)
			b.logger.Debug("Command exited with non-zero status", "exit_code", exitCode)
			return err
		}

		// This is an unexpected error (not just a non-zero exit)
		b.logger.Error("Command execution failed", "error", err)
		return err
	}
	b.logger.Debug("Command completed successfully")

	return nil
}

func (b *LandJail) getChildCommand(command []string) *exec.Cmd {
	cmd := exec.Command(command[0], command[1:]...)
	// Set env vars for the child process; they will be inherited by the target process.
	cmd.Env = getEnvsForTargetProcess(b.config.UserInfo.ConfigDir, b.config.UserInfo.CACertPath(), int(b.config.ProxyPort))
	cmd.Env = append(cmd.Env, "CHILD=true")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGTERM,
	}

	return cmd
}

func (b *LandJail) startProxy() error {
	// Start proxy server in background
	err := b.proxyServer.Start()
	if err != nil {
		b.logger.Error("Proxy server error", "error", err)
		return err
	}

	// Give proxy time to start
	time.Sleep(100 * time.Millisecond)

	return nil
}

func (b *LandJail) stopProxy() error {
	// Stop proxy server
	if b.proxyServer != nil {
		err := b.proxyServer.Stop()
		if err != nil {
			b.logger.Error("Failed to stop proxy server", "error", err)
		}
	}

	return nil
}
