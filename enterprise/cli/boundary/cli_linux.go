//go:build linux

//nolint:revive,gocritic,errname,unconvert

package boundary

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/coder/coder/v2/agent/boundarylogproxy"
	"github.com/coder/coder/v2/enterprise/cli/boundary/config"
	"github.com/coder/coder/v2/enterprise/cli/boundary/log"
	"github.com/coder/coder/v2/enterprise/cli/boundary/run"
	"github.com/coder/serpent"
)

// printVersion prints version information.
func printVersion(version string) {
	fmt.Println(version)
}

// NewCommand creates and returns the root serpent command
func NewCommand(version string) *serpent.Command {
	// To make the top level boundary command, we just make some minor changes to the base command
	cmd := BaseCommand(version)
	cmd.Use = "boundary [flags] -- command [args...]" // Add the flags and args pieces to usage.

	// Add example usage to the long description. This is different from usage as a subcommand because it
	// may be called something different when used as a subcommand / there will be a leading binary (i.e. `coder boundary` vs. `boundary`).
	cmd.Long += `Examples:
  # Allow only requests to github.com
  boundary --allow "domain=github.com" -- curl https://github.com

  # Monitor all requests to specific domains (allow only those)
  boundary --allow "domain=github.com path=/api/issues/*" --allow "method=GET,HEAD domain=github.com" -- npm install

  # Use allowlist from config file with additional CLI allow rules
  boundary --allow "domain=example.com" -- curl https://example.com

  # Block everything by default (implicit)`

	return cmd
}

// Base command returns the boundary serpent command without the information involved in making it the
// *top level* serpent command. We are creating this split to make it easier to integrate into the coder
// CLI if needed.
func BaseCommand(version string) *serpent.Command {
	cliConfig := config.CliConfig{}
	var showVersion serpent.Bool

	// Set default config path if file exists - serpent will load it automatically
	if home, err := os.UserHomeDir(); err == nil {
		defaultPath := filepath.Join(home, ".config", "coder_boundary", "config.yaml")
		if _, err := os.Stat(defaultPath); err == nil {
			cliConfig.Config = serpent.YAMLConfigPath(defaultPath)
		}
	}

	return &serpent.Command{
		Use:   "boundary",
		Short: "Network isolation tool for monitoring and restricting HTTP/HTTPS requests",
		Long:  `boundary creates an isolated network environment for target processes, intercepting HTTP/HTTPS traffic through a transparent proxy that enforces user-defined allow rules.`,
		Options: []serpent.Option{
			{
				Flag:        "config",
				Env:         "BOUNDARY_CONFIG",
				Description: "Path to YAML config file.",
				Value:       &cliConfig.Config,
				YAML:        "",
			},
			{
				Flag:        "allow",
				Env:         "BOUNDARY_ALLOW",
				Description: "Allow rule (repeatable). These are merged with allowlist from config file. Format: \"pattern\" or \"METHOD[,METHOD] pattern\".",
				Value:       &cliConfig.AllowStrings,
				YAML:        "", // CLI only, not loaded from YAML
			},
			{
				Flag:        "allowlist",
				Description: "Allowlist rules from config file (YAML only).",
				Value:       &cliConfig.AllowListStrings,
				YAML:        "allowlist",
				Hidden:      true, // Hidden because it's primarily for YAML config
			},
			{
				Flag:        "log-level",
				Env:         "BOUNDARY_LOG_LEVEL",
				Description: "Set log level (error, warn, info, debug).",
				Default:     "warn",
				Value:       &cliConfig.LogLevel,
				YAML:        "log_level",
			},
			{
				Flag:        "log-dir",
				Env:         "BOUNDARY_LOG_DIR",
				Description: "Set a directory to write logs to rather than stderr.",
				Value:       &cliConfig.LogDir,
				YAML:        "log_dir",
			},
			{
				Flag:        "proxy-port",
				Env:         "PROXY_PORT",
				Description: "Set a port for HTTP proxy.",
				Default:     "8080",
				Value:       &cliConfig.ProxyPort,
				YAML:        "proxy_port",
			},
			{
				Flag:        "pprof",
				Env:         "BOUNDARY_PPROF",
				Description: "Enable pprof profiling server.",
				Value:       &cliConfig.PprofEnabled,
				YAML:        "pprof_enabled",
			},
			{
				Flag:        "pprof-port",
				Env:         "BOUNDARY_PPROF_PORT",
				Description: "Set port for pprof profiling server.",
				Default:     "6060",
				Value:       &cliConfig.PprofPort,
				YAML:        "pprof_port",
			},
			{
				Flag:        "configure-dns-for-local-stub-resolver",
				Env:         "BOUNDARY_CONFIGURE_DNS_FOR_LOCAL_STUB_RESOLVER",
				Description: "Configure DNS for local stub resolver (e.g., systemd-resolved). Only needed when /etc/resolv.conf contains nameserver 127.0.0.53.",
				Value:       &cliConfig.ConfigureDNSForLocalStubResolver,
				YAML:        "configure_dns_for_local_stub_resolver",
			},
			{
				Flag:        "jail-type",
				Env:         "BOUNDARY_JAIL_TYPE",
				Description: "Jail type to use for network isolation. Options: nsjail (default), landjail.",
				Default:     "nsjail",
				Value:       &cliConfig.JailType,
				YAML:        "jail_type",
			},
			{
				Flag:        "disable-audit-logs",
				Env:         "DISABLE_AUDIT_LOGS",
				Description: "Disable sending of audit logs to the workspace agent when set to true.",
				Value:       &cliConfig.DisableAuditLogs,
				YAML:        "disable_audit_logs",
			},
			{
				Flag:        "log-proxy-socket-path",
				Description: "Path to the socket where the boundary log proxy server listens for audit logs.",
				// Important: this default must be the same default path used by the
				// workspace agent to ensure agreement of the default socket path without
				// explicit configuration.
				Default: boundarylogproxy.DefaultSocketPath(),
				// Important: this must be the same variable name used by the workspace agent
				// to allow a single environment variable to configure both boundary and the
				// workspace agent.
				Env:   "CODER_AGENT_BOUNDARY_LOG_PROXY_SOCKET_PATH",
				Value: &cliConfig.LogProxySocketPath,
				YAML:  "", // CLI only, not loaded from YAML
			},
			{
				Flag:        "version",
				Description: "Print version information and exit.",
				Value:       &showVersion,
				YAML:        "", // CLI only
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			// Handle --version flag early
			if showVersion.Value() {
				printVersion(version)
				return nil
			}
			appConfig, err := config.NewAppConfigFromCliConfig(cliConfig, inv.Args)
			if err != nil {
				return fmt.Errorf("failed to parse cli config file: %v", err)
			}

			// Get command arguments
			if len(appConfig.TargetCMD) == 0 {
				return fmt.Errorf("no command specified")
			}

			logger, err := log.SetupLogging(appConfig)
			if err != nil {
				return fmt.Errorf("could not set up logging: %v", err)
			}

			appConfigInJSON, err := json.Marshal(appConfig)
			if err != nil {
				return err
			}
			logger.Debug("Application config", "config", appConfigInJSON)

			return run.Run(inv.Context(), logger, appConfig)
		},
	}
}
