package cli

import (
	"os"

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

func mergeStartConfig(file string, flagCfg startConfig) startConfig {
	var fileCfg startConfig
	b, err := os.ReadFile(file)
	if err != nil {
		return flagCfg
	}
	err = yaml.Unmarshal(b, fileCfg)
	if err != nil {
		return flagCfg
	}

	if flagCfg.AccessURL != "" {
		fileCfg.AccessURL = flagCfg.AccessURL
	}
	if flagCfg.Address != "" {
		fileCfg.Address = flagCfg.Address
	}
	if flagCfg.CacheDir != "" {
		fileCfg.CacheDir = flagCfg.CacheDir
	}
	if flagCfg.Dev {
		fileCfg.Dev = flagCfg.Dev
	}
	if flagCfg.PostgresURL != "" {
		fileCfg.PostgresURL = flagCfg.PostgresURL
	}
	if flagCfg.ProvisionerDaemonCount != 0 {
		fileCfg.ProvisionerDaemonCount = flagCfg.ProvisionerDaemonCount
	}
	if flagCfg.TLSCertFile != "" {
		fileCfg.TLSCertFile = flagCfg.TLSCertFile
	}
	if flagCfg.TLSClientCAFile != "" {
		fileCfg.TLSClientCAFile = flagCfg.TLSClientCAFile
	}
	if flagCfg.TLSClientAuth != "" {
		fileCfg.TLSClientAuth = flagCfg.TLSClientAuth
	}
	if flagCfg.TLSEnable {
		fileCfg.TLSEnable = flagCfg.TLSEnable
	}
	if flagCfg.TLSKeyFile != "" {
		fileCfg.TLSKeyFile = flagCfg.TLSKeyFile
	}
	if flagCfg.TLSMinVersion != "" {
		fileCfg.TLSMinVersion = flagCfg.TLSMinVersion
	}
	if flagCfg.SkipTunnel {
		fileCfg.SkipTunnel = flagCfg.SkipTunnel
	}
	if flagCfg.TraceDatadog {
		fileCfg.TraceDatadog = flagCfg.TraceDatadog
	}
	if flagCfg.SecureAuthCookie {
		fileCfg.SecureAuthCookie = flagCfg.SecureAuthCookie
	}
	if flagCfg.SSHKeygenAlgorithmRaw != "" {
		fileCfg.SSHKeygenAlgorithmRaw = flagCfg.SSHKeygenAlgorithmRaw
	}

	return fileCfg
}
