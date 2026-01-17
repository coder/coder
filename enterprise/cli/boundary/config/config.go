//nolint:revive,gocritic,errname,unconvert
package config

import (
	"strings"

	"github.com/spf13/pflag"
	"golang.org/x/xerrors"

	"github.com/coder/serpent"
)

// JailType represents the type of jail to use for network isolation
type JailType string

const (
	NSJailType   JailType = "nsjail"
	LandjailType JailType = "landjail"
)

func NewJailTypeFromString(str string) (JailType, error) {
	switch str {
	case "nsjail":
		return NSJailType, nil
	case "landjail":
		return LandjailType, nil
	default:
		return NSJailType, xerrors.Errorf("invalid JailType: %s", str)
	}
}

// AllowStringsArray is a custom type that implements pflag.Value to support
// repeatable --allow flags without splitting on commas. This allows comma-separated
// paths within a single allow rule (e.g., "path=/todos/1,/todos/2").
type AllowStringsArray []string

var _ pflag.Value = (*AllowStringsArray)(nil)

// Set implements pflag.Value. It appends the value to the slice without splitting on commas.
func (a *AllowStringsArray) Set(value string) error {
	*a = append(*a, value)
	return nil
}

// String implements pflag.Value.
func (a AllowStringsArray) String() string {
	return strings.Join(a, ",")
}

// Type implements pflag.Value.
func (a AllowStringsArray) Type() string {
	return "string"
}

// Value returns the underlying slice of strings.
func (a AllowStringsArray) Value() []string {
	return []string(a)
}

type CliConfig struct {
	Config                           serpent.YAMLConfigPath `yaml:"-"`
	AllowListStrings                 serpent.StringArray    `yaml:"allowlist"` // From config file
	AllowStrings                     AllowStringsArray      `yaml:"-"`         // From CLI flags only
	LogLevel                         serpent.String         `yaml:"log_level"`
	LogDir                           serpent.String         `yaml:"log_dir"`
	ProxyPort                        serpent.Int64          `yaml:"proxy_port"`
	PprofEnabled                     serpent.Bool           `yaml:"pprof_enabled"`
	PprofPort                        serpent.Int64          `yaml:"pprof_port"`
	ConfigureDNSForLocalStubResolver serpent.Bool           `yaml:"configure_dns_for_local_stub_resolver"`
	JailType                         serpent.String         `yaml:"jail_type"`
	DisableAuditLogs                 serpent.Bool           `yaml:"disable_audit_logs"`
	LogProxySocketPath               serpent.String         `yaml:"log_proxy_socket_path"`
}

type AppConfig struct {
	AllowRules                       []string
	LogLevel                         string
	LogDir                           string
	ProxyPort                        int64
	PprofEnabled                     bool
	PprofPort                        int64
	ConfigureDNSForLocalStubResolver bool
	JailType                         JailType
	TargetCMD                        []string
	UserInfo                         *UserInfo
	DisableAuditLogs                 bool
	LogProxySocketPath               string
}

func NewAppConfigFromCliConfig(cfg CliConfig, targetCMD []string) (AppConfig, error) {
	// Merge allowlist from config file with allow from CLI flags
	allowListStrings := cfg.AllowListStrings.Value()
	allowStrings := cfg.AllowStrings.Value()

	// Combine allowlist (config file) with allow (CLI flags)
	allAllowStrings := append(allowListStrings, allowStrings...)

	jailType, err := NewJailTypeFromString(cfg.JailType.Value())
	if err != nil {
		return AppConfig{}, err
	}

	userInfo := GetUserInfo()

	return AppConfig{
		AllowRules:                       allAllowStrings,
		LogLevel:                         cfg.LogLevel.Value(),
		LogDir:                           cfg.LogDir.Value(),
		ProxyPort:                        cfg.ProxyPort.Value(),
		PprofEnabled:                     cfg.PprofEnabled.Value(),
		PprofPort:                        cfg.PprofPort.Value(),
		ConfigureDNSForLocalStubResolver: cfg.ConfigureDNSForLocalStubResolver.Value(),
		JailType:                         jailType,
		TargetCMD:                        targetCMD,
		UserInfo:                         userInfo,
		DisableAuditLogs:                 cfg.DisableAuditLogs.Value(),
		LogProxySocketPath:               cfg.LogProxySocketPath.Value(),
	}, nil
}
