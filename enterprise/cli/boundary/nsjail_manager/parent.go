//go:build linux

package nsjail_manager

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/coder/coder/v2/enterprise/cli/boundary/audit"
	"github.com/coder/coder/v2/enterprise/cli/boundary/config"
	"github.com/coder/coder/v2/enterprise/cli/boundary/nsjail_manager/nsjail"
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

	// Create jailer with cert path from TLS setup
	jailer, err := nsjail.NewLinuxJail(nsjail.Config{
		Logger:                           logger,
		HttpProxyPort:                    int(config.ProxyPort),
		HomeDir:                          config.UserInfo.HomeDir,
		ConfigDir:                        config.UserInfo.ConfigDir,
		CACertPath:                       config.UserInfo.CACertPath(),
		ConfigureDNSForLocalStubResolver: config.ConfigureDNSForLocalStubResolver,
	})
	if err != nil {
		return fmt.Errorf("failed to create jailer: %v", err)
	}

	// Create boundary instance
	nsJailMgr, err := NewNSJailManager(ruleEngine, auditor, tlsConfig, jailer, logger, config)
	if err != nil {
		return fmt.Errorf("failed to create boundary instance: %v", err)
	}

	return nsJailMgr.Run(ctx)
}
