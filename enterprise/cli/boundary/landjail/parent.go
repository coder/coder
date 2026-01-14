//go:build linux

package landjail

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/coder/coder/v2/enterprise/cli/boundary/audit"
	"github.com/coder/coder/v2/enterprise/cli/boundary/config"
	"github.com/coder/coder/v2/enterprise/cli/boundary/rulesengine"
	"github.com/coder/coder/v2/enterprise/cli/boundary/tls"
)

func RunParent(ctx context.Context, logger *slog.Logger, config config.AppConfig) error {
	if len(config.AllowRules) == 0 {
		logger.Warn("No allow rules specified; all network traffic will be denied by default")
	}

	// Parse allow rules
	allowRules, err := rulesengine.ParseAllowSpecs(config.AllowRules)
	if err != nil {
		logger.Error("Failed to parse allow rules", "error", err)
		return fmt.Errorf("failed to parse allow rules: %v", err)
	}

	// Create rule engine
	ruleEngine := rulesengine.NewRuleEngine(allowRules, logger)

	// Create auditor
	auditor, err := audit.SetupAuditor(ctx, logger, config.DisableAuditLogs, config.LogProxySocketPath)
	if err != nil {
		return fmt.Errorf("failed to setup auditor: %v", err)
	}

	// Create TLS certificate manager
	certManager, err := tls.NewCertificateManager(tls.Config{
		Logger:    logger,
		ConfigDir: config.UserInfo.ConfigDir,
		Uid:       config.UserInfo.Uid,
		Gid:       config.UserInfo.Gid,
	})
	if err != nil {
		logger.Error("Failed to create certificate manager", "error", err)
		return fmt.Errorf("failed to create certificate manager: %v", err)
	}

	// Setup TLS to get cert path for jailer
	tlsConfig, err := certManager.SetupTLSAndWriteCACert()
	if err != nil {
		return fmt.Errorf("failed to setup TLS and CA certificate: %v", err)
	}

	landjail, err := NewLandJail(ruleEngine, auditor, tlsConfig, logger, config)
	if err != nil {
		return fmt.Errorf("failed to create landjail: %v", err)
	}

	return landjail.Run(ctx)
}
