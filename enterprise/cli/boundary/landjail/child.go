//go:build linux

package landjail

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	"github.com/landlock-lsm/go-landlock/landlock"

	"github.com/coder/coder/v2/enterprise/cli/boundary/config"
	"github.com/coder/coder/v2/enterprise/cli/boundary/util"
)

type LandlockConfig struct {
	// TODO(yevhenii):
	// - should it be able to bind to any port?
	// - should it be able to connect to any port on localhost?
	// BindTCPPorts    []int
	ConnectTCPPorts []int
}

func ApplyLandlockRestrictions(logger *slog.Logger, cfg LandlockConfig) error {
	// Get the Landlock version which works for Kernel 6.7+
	llCfg := landlock.V4

	// Collect our rules
	var netRules []landlock.Rule

	// Add rules for TCP connections
	for _, port := range cfg.ConnectTCPPorts {
		logger.Debug("Adding TCP connect port", "port", port)
		netRules = append(netRules, landlock.ConnectTCP(uint16(port)))
	}

	err := llCfg.RestrictNet(netRules...)
	if err != nil {
		return fmt.Errorf("failed to apply Landlock network restrictions: %w", err)
	}

	return nil
}

func RunChild(logger *slog.Logger, config config.AppConfig) error {
	landjailCfg := LandlockConfig{
		ConnectTCPPorts: []int{int(config.ProxyPort)},
	}

	err := ApplyLandlockRestrictions(logger, landjailCfg)
	if err != nil {
		return fmt.Errorf("failed to apply Landlock network restrictions: %v", err)
	}

	// Build command
	cmd := exec.Command(config.TargetCMD[0], config.TargetCMD[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	logger.Info("Executing target command", "command", config.TargetCMD)

	// Run the command - this will block until it completes
	err = cmd.Run()
	if err != nil {
		// Check if this is a normal exit with non-zero status code
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode := exitError.ExitCode()
			logger.Debug("Command exited with non-zero status", "exit_code", exitCode)
			return fmt.Errorf("command exited with code %d", exitCode)
		}
		// This is an unexpected error
		logger.Error("Command execution failed", "error", err)
		return fmt.Errorf("command execution failed: %v", err)
	}

	logger.Debug("Command completed successfully")
	return nil
}

// Returns environment variables intended to be set on the child process,
// so they can later be inherited by the target process.
func getEnvsForTargetProcess(configDir string, caCertPath string, httpProxyPort int) []string {
	e := os.Environ()

	proxyAddr := fmt.Sprintf("http://localhost:%d", httpProxyPort)
	e = util.MergeEnvs(e, map[string]string{
		// Set standard CA certificate environment variables for common tools
		// This makes tools like curl, git, etc. trust our dynamically generated CA
		"SSL_CERT_FILE":       caCertPath, // OpenSSL/LibreSSL-based tools
		"SSL_CERT_DIR":        configDir,  // OpenSSL certificate directory
		"CURL_CA_BUNDLE":      caCertPath, // curl
		"GIT_SSL_CAINFO":      caCertPath, // Git
		"REQUESTS_CA_BUNDLE":  caCertPath, // Python requests
		"NODE_EXTRA_CA_CERTS": caCertPath, // Node.js

		"HTTP_PROXY":  proxyAddr,
		"HTTPS_PROXY": proxyAddr,
		"http_proxy":  proxyAddr,
		"https_proxy": proxyAddr,
	})

	return e
}
