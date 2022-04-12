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

func parseStartConfig(path string) startConfig {
	var cfg startConfig
	b, err := os.ReadFile(path)
	if err != nil {
		return startConfig{}
	}
	err = yaml.Unmarshal(b, cfg)
	if err != nil {
		return startConfig{}
	}

	return cfg
}
