package cli

import (
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type startConfig struct {
	AccessURL              string `yaml:"access-url"`
	Address                string `yaml:"address"`
	CacheDir               string `yaml:"cache-dir"`
	Dev                    bool   `yaml:"dev"`
	PostgresURL            string `yaml:"postgres-url"`
	ProvisionerDaemonCount uint8  `yaml:"provisioner-daemons"`
	TLSCertFile            string `yaml:"tls-cert-file"`
	TLSClientCAFile        string `yaml:"tls-client-ca-file"`
	TLSClientAuth          string `yaml:"tls-client-auth"`
	TLSEnable              bool   `yaml:"tls-enable"`
	TLSKeyFile             string `yaml:"tls-key-file"`
	TLSMinVersion          string `yaml:"tls-min-version"`
	SkipTunnel             bool   `yaml:"skip-tunnel"`
	TraceDatadog           bool   `yaml:"trace-datadog"`
	SecureAuthCookie       bool   `yaml:"secure-auth-cookie"`
	SSHKeygenAlgorithmRaw  string `yaml:"ssh-keygen-algorithm"`
}

func mergeStartConfig(cmd *cobra.Command, flags startConfig) startConfig {
	var cfg startConfig
	f, err := cmd.Flags().GetString(varStartConfig)
	if err != nil {
		return flags
	}
	b, err := os.ReadFile(f)
	if err != nil {
		return flags
	}
	err = yaml.Unmarshal(b, cfg)
	if err != nil {
		return flags
	}

	if flags.AccessURL != "" {
		cfg.AccessURL = flags.AccessURL
	}
	if flags.Address != "" {
		cfg.Address = flags.Address
	}
	if flags.CacheDir != "" {
		cfg.CacheDir = flags.CacheDir
	}
	if flags.Dev {
		cfg.Dev = flags.Dev
	}
	if flags.PostgresURL != "" {
		cfg.PostgresURL = flags.PostgresURL
	}
	if flags.ProvisionerDaemonCount != 0 {
		cfg.ProvisionerDaemonCount = flags.ProvisionerDaemonCount
	}
	if flags.TLSCertFile != "" {
		cfg.TLSCertFile = flags.TLSCertFile
	}
	if flags.TLSClientCAFile != "" {
		cfg.TLSClientCAFile = flags.TLSClientCAFile
	}
	if flags.TLSClientAuth != "" {
		cfg.TLSClientAuth = flags.TLSClientAuth
	}
	if flags.TLSEnable {
		cfg.TLSEnable = flags.TLSEnable
	}
	if flags.TLSKeyFile != "" {
		cfg.TLSKeyFile = flags.TLSKeyFile
	}
	if flags.TLSMinVersion != "" {
		cfg.TLSMinVersion = flags.TLSMinVersion
	}
	if flags.SkipTunnel {
		cfg.SkipTunnel = flags.SkipTunnel
	}
	if flags.TraceDatadog {
		cfg.TraceDatadog = flags.TraceDatadog
	}
	if flags.SecureAuthCookie {
		cfg.SecureAuthCookie = flags.SecureAuthCookie
	}
	if flags.SSHKeygenAlgorithmRaw != "" {
		cfg.SSHKeygenAlgorithmRaw = flags.SSHKeygenAlgorithmRaw
	}

	return cfg
}
