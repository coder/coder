//go:build linux

package nsjail_manager

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
	"github.com/coder/coder/v2/enterprise/cli/boundary/nsjail_manager/nsjail"
	"github.com/coder/coder/v2/enterprise/cli/boundary/proxy"
	"github.com/coder/coder/v2/enterprise/cli/boundary/rulesengine"
)

type NSJailManager struct {
	jailer      nsjail.Jailer
	proxyServer *proxy.Server
	logger      *slog.Logger
	config      config.AppConfig
}

func NewNSJailManager(
	ruleEngine rulesengine.Engine,
	auditor audit.Auditor,
	tlsConfig *tls.Config,
	jailer nsjail.Jailer,
	logger *slog.Logger,
	config config.AppConfig,
) (*NSJailManager, error) {
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

	return &NSJailManager{
		config:      config,
		jailer:      jailer,
		proxyServer: proxyServer,
		logger:      logger,
	}, nil
}

func (b *NSJailManager) Run(ctx context.Context) error {
	b.logger.Info("Start namespace-jail manager")
	err := b.setupHostAndStartProxy()
	if err != nil {
		return fmt.Errorf("failed to start namespace-jail manager: %v", err)
	}

	defer func() {
		b.logger.Info("Stop namespace-jail manager")
		err := b.stopProxyAndCleanupHost()
		if err != nil {
			b.logger.Error("Failed to stop namespace-jail manager", "error", err)
		}
	}()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		defer cancel()
		b.RunChildProcess(os.Args)
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

func (b *NSJailManager) RunChildProcess(command []string) {
	cmd := b.jailer.Command(command)

	b.logger.Debug("Executing command in boundary", "command", strings.Join(os.Args, " "))
	err := cmd.Start()
	if err != nil {
		b.logger.Error("Command failed to start", "error", err)
		return
	}

	err = b.jailer.ConfigureHostNsCommunication(cmd.Process.Pid)
	if err != nil {
		b.logger.Error("configuration after command execution failed", "error", err)
		return
	}

	b.logger.Debug("waiting on a child process to finish")
	err = cmd.Wait()
	if err != nil {
		// Check if this is a normal exit with non-zero status code
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode := exitError.ExitCode()
			// Log at debug level for non-zero exits (normal behavior)
			b.logger.Debug("Command exited with non-zero status", "exit_code", exitCode)
		} else {
			// This is an unexpected error (not just a non-zero exit)
			b.logger.Error("Command execution failed", "error", err)
		}
		return
	}
	b.logger.Debug("Command completed successfully")
}

func (b *NSJailManager) setupHostAndStartProxy() error {
	// Configure the jailer (network isolation)
	err := b.jailer.ConfigureHost()
	if err != nil {
		return fmt.Errorf("failed to start jailer: %v", err)
	}

	// Start proxy server in background
	err = b.proxyServer.Start()
	if err != nil {
		b.logger.Error("Proxy server error", "error", err)
		return err
	}

	// Give proxy time to start
	time.Sleep(100 * time.Millisecond)

	return nil
}

func (b *NSJailManager) stopProxyAndCleanupHost() error {
	// Stop proxy server
	if b.proxyServer != nil {
		err := b.proxyServer.Stop()
		if err != nil {
			b.logger.Error("Failed to stop proxy server", "error", err)
		}
	}

	// Close jailer
	return b.jailer.Close()
}
