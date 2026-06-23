package codersdk_test

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

type exclusion struct {
	flag bool
	env  bool
	yaml bool
}

func TestDeploymentValues_HighlyConfigurable(t *testing.T) {
	t.Parallel()

	// This test ensures that every deployment option has
	// a corresponding Flag, Env, and YAML name, unless explicitly excluded.

	excludes := map[string]exclusion{
		// These are used to configure YAML support itself, so
		// they make no sense within the YAML file.
		"Config Path": {
			yaml: true,
		},
		"Write Config": {
			yaml: true,
			env:  true,
		},
		// Dangerous values? Not sure we should help users
		// persistent their configuration.
		"DANGEROUS: Allow Path App Sharing": {
			yaml: true,
		},
		"DANGEROUS: Allow Site Owners to Access Path Apps": {
			yaml: true,
		},
		// Secrets
		"Trace Honeycomb API Key": {
			yaml: true,
		},
		"OAuth2 GitHub Client Secret": {
			yaml: true,
		},
		"OIDC Client Secret": {
			yaml: true,
		},
		"Postgres Connection URL": {
			yaml: true,
		},
		"SCIM API Key": {
			yaml: true,
		},
		"External Token Encryption Keys": {
			yaml: true,
		},
		"External Auth Providers": {
			// Technically External Auth Providers can be provided through the env,
			// but bypassing serpent. See cli.ReadExternalAuthProvidersFromEnv.
			flag: true,
			env:  true,
		},
		"Provisioner Daemon Pre-shared Key (PSK)": {
			yaml: true,
		},
		"Email Auth: Password": {
			yaml: true,
		},
		"Notifications: Email Auth: Password": {
			yaml: true,
		},
		// We don't want these to be configurable via YAML because they are secrets.
		// However, we do want to allow them to be shown in documentation.
		"AI Gateway OpenAI Key": {
			yaml: true,
		},
		"AI Gateway Anthropic Key": {
			yaml: true,
		},
		"AI Gateway Bedrock Access Key": {
			yaml: true,
		},
		"AI Gateway Bedrock Access Key Secret": {
			yaml: true,
		},
	}

	set := (&codersdk.DeploymentValues{}).Options()
	for _, opt := range set {
		// These are generally for development, so their configurability is
		// not relevant.
		if opt.Hidden {
			delete(excludes, opt.Name)
			continue
		}

		if codersdk.IsSecretDeploymentOption(opt) && opt.YAML != "" {
			// Secrets should not be written to YAML and instead should continue
			// to be read from the environment.
			//
			// Unfortunately, secrets are still accepted through flags for
			// legacy purposes. Eventually, we should prevent that.
			t.Errorf("Option %q is a secret but has a YAML name", opt.Name)
		}

		excluded := excludes[opt.Name]
		switch {
		case opt.YAML == "" && !excluded.yaml:
			t.Errorf("Option %q should have a YAML name", opt.Name)
		case opt.YAML != "" && excluded.yaml:
			t.Errorf("Option %q is excluded but has a YAML name", opt.Name)
		case opt.Flag == "" && !excluded.flag:
			t.Errorf("Option %q should have a flag name", opt.Name)
		case opt.Flag != "" && excluded.flag:
			t.Errorf("Option %q is excluded but has a flag name", opt.Name)
		case opt.Env == "" && !excluded.env:
			t.Errorf("Option %q should have an env name", opt.Name)
		case opt.Env != "" && excluded.env:
			t.Errorf("Option %q is excluded but has an env name", opt.Name)
		}

		// Also check all env vars are prefixed with CODER_
		const prefix = "CODER_"
		if opt.Env != "" && !strings.HasPrefix(opt.Env, prefix) {
			t.Errorf("Option %q has an env name (%q) that is not prefixed with %s", opt.Name, opt.Env, prefix)
		}

		delete(excludes, opt.Name)
	}

	for opt := range excludes {
		t.Errorf("Excluded option %q is not in the deployment config. Remove it?", opt)
	}
}

func TestParseSSHConfigOption(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		option    string
		wantKey   string
		wantValue string
		wantErr   bool
	}{
		{
			name:      "ProxyCommandWithSpaces",
			option:    "ProxyCommand=ssh -W %h:%p bastion",
			wantKey:   "ProxyCommand",
			wantValue: "ssh -W %h:%p bastion",
		},
		{
			name:      "SetEnvWithEquals",
			option:    "SetEnv=FOO=bar BAZ=qux",
			wantKey:   "SetEnv",
			wantValue: "FOO=bar BAZ=qux",
		},
		{
			name:      "SetEnvWithSpaceSeparator",
			option:    "SetEnv FOO=bar BAZ=qux",
			wantKey:   "SetEnv",
			wantValue: "FOO=bar BAZ=qux",
		},
		{
			name:      "HostName",
			option:    "HostName example.com",
			wantKey:   "HostName",
			wantValue: "example.com",
		},
		{
			name:    "NewlineInValue",
			option:  "ProxyCommand=echo hi\nHost *",
			wantErr: true,
		},
		{
			name:    "CarriageReturnInValue",
			option:  "ProxyCommand=echo hi\rHost *",
			wantErr: true,
		},
		{
			name:    "NULInValue",
			option:  "ProxyCommand=echo hi\x00Host *",
			wantErr: true,
		},
		{
			name:    "NewlineInKey",
			option:  "Proxy\nCommand=value",
			wantErr: true,
		},
		{
			name:    "CarriageReturnInKey",
			option:  "Proxy\rCommand=value",
			wantErr: true,
		},
		{
			name:    "NULInKey",
			option:  "Proxy\x00Command=value",
			wantErr: true,
		},
		{
			name:    "MissingSeparator",
			option:  "JustAKeyNoValue",
			wantErr: true,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			key, value, err := codersdk.ParseSSHConfigOption(tt.option)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantKey, key)
			require.Equal(t, tt.wantValue, value)
		})
	}
}

func TestValidateWorkspaceHostnameSuffix(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		suffix  string
		wantErr bool
	}{
		{name: "Coder", suffix: "coder"},
		{name: "Example", suffix: "example"},
		{name: "Dotted", suffix: "coder.example.com"},
		{name: "Empty", suffix: ""},
		{name: "LeadingDot", suffix: ".coder", wantErr: true},
		{name: "Newline", suffix: "coder\nHost *\n\tProxyCommand evil", wantErr: true},
		{name: "CarriageReturn", suffix: "coder\r\nHost *", wantErr: true},
		{name: "Space", suffix: "coder Host *", wantErr: true},
		{name: "Tab", suffix: "coder\t*", wantErr: true},
		{name: "NUL", suffix: "coder\x00", wantErr: true},
		{name: "NonBreakingSpace", suffix: "coder\u00A0suffix", wantErr: true},
		{name: "Glob", suffix: "*", wantErr: true},
		{name: "GlobPrefix", suffix: "*.*", wantErr: true},
		{name: "QuestionMark", suffix: "code?", wantErr: true},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := codersdk.ValidateWorkspaceHostnameSuffix(tt.suffix)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateWorkspaceHostnamePrefix(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		prefix  string
		wantErr bool
	}{
		{name: "Default", prefix: "coder."},
		{name: "NoDot", prefix: "coder"},
		{name: "Empty", prefix: ""},
		{name: "LeadingDot", prefix: ".coder"},
		{name: "Newline", prefix: "coder.\nHost *\n\tProxyCommand evil", wantErr: true},
		{name: "CarriageReturn", prefix: "coder.\r\nHost *", wantErr: true},
		{name: "Space", prefix: "coder. Host *", wantErr: true},
		{name: "Tab", prefix: "coder.\t*", wantErr: true},
		{name: "NUL", prefix: "coder.\x00", wantErr: true},
		{name: "NonBreakingSpace", prefix: "coder.\u00A0x", wantErr: true},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := codersdk.ValidateWorkspaceHostnamePrefix(tt.prefix)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateSSHConfigOptions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		options map[string]string
		wantErr bool
	}{
		{name: "HostName", options: map[string]string{"HostName": "example.com"}},
		{name: "User", options: map[string]string{"User": "coder"}},
		{name: "Port", options: map[string]string{"Port": "22"}},
		{name: "SetEnv", options: map[string]string{"SetEnv": "FOO=bar BAZ=qux"}},
		{name: "UserKnownHostsFile", options: map[string]string{"UserKnownHostsFile": "/tmp/coder_known_hosts"}},
		{name: "EmptyKey", options: map[string]string{"": "value"}, wantErr: true},
		{name: "NewlineInKey", options: map[string]string{"User\nProxyCommand": "evil"}, wantErr: true},
		{name: "CarriageReturnInKey", options: map[string]string{"User\rProxyCommand": "evil"}, wantErr: true},
		{name: "NULInKey", options: map[string]string{"User\x00ProxyCommand": "evil"}, wantErr: true},
		{name: "SpaceInKey", options: map[string]string{"User ProxyCommand": "evil"}, wantErr: true},
		{name: "EqualsInKey", options: map[string]string{"User=ProxyCommand": "evil"}, wantErr: true},
		{name: "Host", options: map[string]string{"Host": "*"}, wantErr: true},
		{name: "HostCaseInsensitive", options: map[string]string{"hOsT": "*"}, wantErr: true},
		{name: "Match", options: map[string]string{"Match": "all"}, wantErr: true},
		{name: "Include", options: map[string]string{"Include": "~/.ssh/config.d/*"}, wantErr: true},
		{name: "ProxyCommand", options: map[string]string{"ProxyCommand": "ssh -W %h:%p bastion"}, wantErr: true},
		{name: "ProxyCommandCaseInsensitive", options: map[string]string{"proxycommand": "ssh -W %h:%p bastion"}, wantErr: true},
		{name: "LocalCommand", options: map[string]string{"LocalCommand": "echo pwned"}, wantErr: true},
		{name: "PermitLocalCommand", options: map[string]string{"PermitLocalCommand": "yes"}, wantErr: true},
		{name: "RemoteCommand", options: map[string]string{"RemoteCommand": "some-command"}, wantErr: true},
		{name: "KnownHostsCommand", options: map[string]string{"KnownHostsCommand": "echo key"}, wantErr: true},
		{name: "PKCS11Provider", options: map[string]string{"PKCS11Provider": "/tmp/evil.so"}, wantErr: true},
		{name: "PKCS11ProviderCaseInsensitive", options: map[string]string{"pkcs11provider": "/tmp/evil.so"}, wantErr: true},
		{name: "SecurityKeyProvider", options: map[string]string{"SecurityKeyProvider": "/tmp/evil.so"}, wantErr: true},
		{name: "NewlineInValue", options: map[string]string{"UserKnownHostsFile": "/tmp/known_hosts\nHost *\nProxyCommand evil"}, wantErr: true},
		{name: "CarriageReturnInValue", options: map[string]string{"UserKnownHostsFile": "/tmp/known_hosts\r\nHost *"}, wantErr: true},
		{name: "NULInValue", options: map[string]string{"UserKnownHostsFile": "/tmp/known_hosts\x00suffix"}, wantErr: true},
		{name: "SmartcardDevice", options: map[string]string{"SmartcardDevice": "/path/to/lib"}, wantErr: true},
		{name: "XAuthLocation", options: map[string]string{"XAuthLocation": "/usr/bin/xauth"}, wantErr: true},
		{name: "ProxyJump", options: map[string]string{"ProxyJump": "bastion.example.com"}, wantErr: true},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := codersdk.ValidateSSHConfigOptions(tt.options)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestSSHConfigResponse_Validate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		response codersdk.SSHConfigResponse
		wantErr  string
	}{
		{
			name: "Valid",
			response: codersdk.SSHConfigResponse{
				HostnamePrefix:   "coder.",
				HostnameSuffix:   "coder",
				SSHConfigOptions: map[string]string{"HostName": "example.com"},
			},
		},
		{
			name:     "Empty",
			response: codersdk.SSHConfigResponse{},
		},
		{
			name:     "PrefixUnsafe",
			response: codersdk.SSHConfigResponse{HostnamePrefix: "coder.\nHost *"},
			wantErr:  "workspace hostname prefix",
		},
		{
			name:     "SuffixUnsafe",
			response: codersdk.SSHConfigResponse{HostnameSuffix: "coder\nHost *"},
			wantErr:  "workspace hostname suffix",
		},
		{
			name:     "OptionUnsafe",
			response: codersdk.SSHConfigResponse{SSHConfigOptions: map[string]string{"ProxyCommand": "ssh -W %h:%p bastion"}},
			wantErr:  `ssh config option "ProxyCommand" is not allowed`,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.response.Validate()
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestSSHConfig_ParseOptions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Name          string
		ConfigOptions serpent.StringArray
		ExpectError   bool
		Expect        map[string]string
	}{
		{
			Name:          "Empty",
			ConfigOptions: []string{},
			Expect:        map[string]string{},
		},
		{
			Name: "Whitespace",
			ConfigOptions: []string{
				"test value",
			},
			Expect: map[string]string{
				"test": "value",
			},
		},
		{
			Name: "SimpleValueEqual",
			ConfigOptions: []string{
				"test=value",
			},
			Expect: map[string]string{
				"test": "value",
			},
		},
		{
			Name: "SimpleValues",
			ConfigOptions: []string{
				"test=value",
				"foo=bar",
			},
			Expect: map[string]string{
				"test": "value",
				"foo":  "bar",
			},
		},
		{
			Name: "ValueWithQuote",
			ConfigOptions: []string{
				"bar=buzz=bazz",
			},
			Expect: map[string]string{
				"bar": "buzz=bazz",
			},
		},
		{
			Name: "NoEquals",
			ConfigOptions: []string{
				"foobar",
			},
			ExpectError: true,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			c := codersdk.SSHConfig{
				SSHConfigOptions: tt.ConfigOptions,
			}
			got, err := c.ParseOptions()
			if tt.ExpectError {
				require.Error(t, err, tt.ConfigOptions.String())
			} else {
				require.NoError(t, err, tt.ConfigOptions.String())
				require.Equalf(t, tt.Expect, got, tt.ConfigOptions.String())
			}
		})
	}
}

func TestTimezoneOffsets(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Name           string
		Now            time.Time
		Loc            *time.Location
		ExpectedOffset int
	}{
		{
			Name:           "UTC",
			Loc:            time.UTC,
			ExpectedOffset: 0,
		},

		{
			Name:           "Eastern",
			Now:            time.Date(2021, 2, 1, 0, 0, 0, 0, time.UTC),
			Loc:            must(time.LoadLocation("America/New_York")),
			ExpectedOffset: 5,
		},
		{
			// Daylight savings is on the 14th of March to Nov 7 in 2021
			Name:           "EasternDaylightSavings",
			Now:            time.Date(2021, 3, 16, 0, 0, 0, 0, time.UTC),
			Loc:            must(time.LoadLocation("America/New_York")),
			ExpectedOffset: 4,
		},
		{
			Name:           "Central",
			Now:            time.Date(2021, 2, 1, 0, 0, 0, 0, time.UTC),
			Loc:            must(time.LoadLocation("America/Chicago")),
			ExpectedOffset: 6,
		},
		{
			Name:           "CentralDaylightSavings",
			Now:            time.Date(2021, 3, 16, 0, 0, 0, 0, time.UTC),
			Loc:            must(time.LoadLocation("America/Chicago")),
			ExpectedOffset: 5,
		},
		{
			Name:           "Ireland",
			Now:            time.Date(2021, 2, 1, 0, 0, 0, 0, time.UTC),
			Loc:            must(time.LoadLocation("Europe/Dublin")),
			ExpectedOffset: 0,
		},
		{
			Name:           "IrelandDaylightSavings",
			Now:            time.Date(2021, 4, 3, 0, 0, 0, 0, time.UTC),
			Loc:            must(time.LoadLocation("Europe/Dublin")),
			ExpectedOffset: -1,
		},
		{
			Name: "HalfHourTz",
			Now:  time.Date(2024, 1, 20, 6, 0, 0, 0, must(time.LoadLocation("Asia/Yangon"))),
			// This timezone is +6:30, but the function rounds to the nearest hour.
			// This is intentional because our DAUs endpoint only covers 1-hour offsets.
			// If the user is in a non-hour timezone, they get the closest hour bucket.
			Loc:            must(time.LoadLocation("Asia/Yangon")),
			ExpectedOffset: -6,
		},
	}

	for _, c := range testCases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			offset := codersdk.TimezoneOffsetHourWithTime(c.Now, c.Loc)
			require.Equal(t, c.ExpectedOffset, offset)
		})
	}
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

func TestAIGatewayCompatibilityAliases(t *testing.T) {
	t.Parallel()

	options := (&codersdk.DeploymentValues{}).Options()
	byFlag := map[string]serpent.Option{}
	for _, opt := range options {
		if opt.Flag != "" {
			byFlag[opt.Flag] = opt
		}
	}

	type alias struct {
		old serpent.Option
		new serpent.Option
	}
	var aliases []alias
	for _, opt := range options {
		if !strings.HasPrefix(opt.Flag, "aibridge-") {
			continue
		}
		require.True(t, strings.HasPrefix(opt.Description, "Deprecated:"), "aibridge option %s should have a 'Deprecated:' description", opt.Flag)
		require.Len(t, opt.UseInstead, 1, "aibridge option %s should point to a single replacement", opt.Flag)

		newOpt, ok := byFlag[opt.UseInstead[0].Flag]
		require.True(t, ok, "aibridge option %s points to unknown flag %s", opt.Flag, opt.UseInstead[0].Flag)
		require.NotEqual(t, opt.Flag, newOpt.Flag, "flag %s shares its flag with the new alias option", opt.Flag)
		require.NotEqual(t, opt.Env, newOpt.Env, "flag %s shares its env with the new alias option", opt.Flag)
		if oldYAML := opt.YAMLPath(); oldYAML != "" {
			require.NotEqual(t, oldYAML, newOpt.YAMLPath(), "flag %s shares its YAML path with the new alias option", opt.Flag)
		} else {
			require.Empty(t, newOpt.YAMLPath(), "flag %s has no YAML path but the new alias option %s does", opt.Flag, newOpt.Flag)
		}
		aliases = append(aliases, alias{old: opt, new: newOpt})
	}
	// Update this count when adding or removing aibridge alias options.
	require.Len(t, aliases, 34, "unexpected number of aibridge alias options")

	sampleVal := func(opt serpent.Option) any {
		switch opt.Value.Type() {
		case "bool":
			return opt.Default != "true"
		case "int":
			return 7
		case "duration":
			return "2h"
		case "string-array":
			return []string{"10.0.0.0/8", "172.16.0.0/12"}
		default:
			return "alias-value"
		}
	}
	sampleArg := func(opt serpent.Option) string {
		v := sampleVal(opt)
		if arr, ok := v.([]string); ok {
			return strings.Join(arr, ",")
		}
		return fmt.Sprint(v)
	}

	aiConfFromOpts := func(t *testing.T, apply func(opts serpent.OptionSet) error) codersdk.AIConfig {
		t.Helper()
		dv := &codersdk.DeploymentValues{}
		opts := dv.Options()
		require.NoError(t, opts.SetDefaults())
		require.NoError(t, apply(opts))
		return dv.AI
	}

	t.Run("FlagParity", func(t *testing.T) {
		t.Parallel()

		var oldArgs, newArgs []string
		for _, a := range aliases {
			value := sampleArg(a.old)
			oldArgs = append(oldArgs, "--"+a.old.Flag, value)
			newArgs = append(newArgs, "--"+a.new.Flag, value)
		}
		oldAI := aiConfFromOpts(t, func(opts serpent.OptionSet) error {
			return opts.FlagSet().Parse(oldArgs)
		})
		newAI := aiConfFromOpts(t, func(opts serpent.OptionSet) error {
			return opts.FlagSet().Parse(newArgs)
		})
		require.Equal(t, newAI, oldAI)
	})

	t.Run("EnvParity", func(t *testing.T) {
		t.Parallel()

		var oldEnv, newEnv []serpent.EnvVar
		for _, a := range aliases {
			value := sampleArg(a.old)
			oldEnv = append(oldEnv, serpent.EnvVar{Name: a.old.Env, Value: value})
			newEnv = append(newEnv, serpent.EnvVar{Name: a.new.Env, Value: value})
		}
		oldAI := aiConfFromOpts(t, func(opts serpent.OptionSet) error {
			return opts.ParseEnv(oldEnv)
		})
		newAI := aiConfFromOpts(t, func(opts serpent.OptionSet) error {
			return opts.ParseEnv(newEnv)
		})
		require.Equal(t, newAI, oldAI)
	})

	t.Run("YAMLParity", func(t *testing.T) {
		t.Parallel()

		setPath := func(doc map[string]any, path string, value any) {
			parts := strings.Split(path, ".")
			for _, field := range parts[:len(parts)-1] {
				next, ok := doc[field].(map[string]any)
				if !ok {
					next = map[string]any{}
					doc[field] = next
				}
				doc = next
			}
			doc[parts[len(parts)-1]] = value
		}

		oldYAML := map[string]any{}
		newYAML := map[string]any{}
		for _, a := range aliases {
			oldPath := a.old.YAMLPath()
			newPath := a.new.YAMLPath()
			if oldPath == "" {
				require.Empty(t, newPath)
				continue
			}
			require.NotEmpty(t, newPath, "new flag %s has no YAML path", a.old.Flag)

			value := sampleVal(a.old)
			setPath(oldYAML, oldPath, value)
			setPath(newYAML, newPath, value)
		}

		parse := func(doc map[string]any) codersdk.AIConfig {
			var node yaml.Node
			require.NoError(t, node.Encode(doc))
			return aiConfFromOpts(t, func(opts serpent.OptionSet) error {
				return opts.UnmarshalYAML(&node)
			})
		}

		require.Equal(t, parse(newYAML), parse(oldYAML))
	})
}

func TestDeploymentValues_Validate_RefreshLifetime(t *testing.T) {
	t.Parallel()

	mk := func(access, refresh time.Duration) *codersdk.DeploymentValues {
		dv := &codersdk.DeploymentValues{}
		dv.Sessions.DefaultDuration = serpent.Duration(access)
		dv.Sessions.RefreshDefaultDuration = serpent.Duration(refresh)
		return dv
	}

	t.Run("EqualDurations_Error", func(t *testing.T) {
		t.Parallel()
		dv := mk(1*time.Hour, 1*time.Hour)
		err := dv.Validate()
		require.Error(t, err)
		require.ErrorContains(t, err, "must be strictly greater")
	})

	t.Run("RefreshShorter_Error", func(t *testing.T) {
		t.Parallel()
		dv := mk(2*time.Hour, 1*time.Hour)
		err := dv.Validate()
		require.Error(t, err)
		require.ErrorContains(t, err, "must be strictly greater")
	})

	t.Run("RefreshZero_Error", func(t *testing.T) {
		t.Parallel()
		dv := mk(1*time.Hour, 0)
		err := dv.Validate()
		require.Error(t, err)
		require.ErrorContains(t, err, "must be strictly greater")
	})

	t.Run("AccessUninitialized_Error", func(t *testing.T) {
		t.Parallel()
		// Access duration is zero (uninitialized); refresh is valid.
		dv := mk(0, 48*time.Hour)
		err := dv.Validate()
		require.Error(t, err)
		require.ErrorContains(t, err, "developer error: sessions configuration appears uninitialized")
	})

	t.Run("RefreshLonger_OK", func(t *testing.T) {
		t.Parallel()
		dv := mk(1*time.Hour, 48*time.Hour)
		err := dv.Validate()
		require.NoError(t, err)
	})
}

func TestDeploymentValues_DurationFormatNanoseconds(t *testing.T) {
	t.Parallel()

	set := (&codersdk.DeploymentValues{}).Options()
	for _, s := range set {
		if s.Value.Type() != "duration" {
			continue
		}
		// Just make sure the annotation is set.
		// If someone wants to not format a duration, they can
		// explicitly set the annotation to false.
		if s.Annotations.IsSet("format_duration") {
			continue
		}
		t.Logf("Option %q is a duration but does not have the format_duration annotation.", s.Name)
		t.Log("To fix this, add the following to the option declaration:")
		t.Log(`Annotations: serpent.Annotations{}.Mark(annotationFormatDurationNS, "true"),`)
		t.FailNow()
	}
}

//go:embed testdata/*
var testData embed.FS

func TestExternalAuthYAMLConfig(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		// The windows marshal function uses different line endings.
		// Not worth the effort getting this to work on windows.
		t.SkipNow()
	}

	file := func(t *testing.T, name string) string {
		data, err := testData.ReadFile(fmt.Sprintf("testdata/%s", name))
		require.NoError(t, err, "read testdata file %q", name)
		return string(data)
	}
	githubCfg := codersdk.ExternalAuthConfig{
		Type:                          "github",
		ClientID:                      "client_id",
		ClientSecret:                  "client_secret",
		ID:                            "id",
		AuthURL:                       "https://example.com/auth",
		TokenURL:                      "https://example.com/token",
		ValidateURL:                   "https://example.com/validate",
		RevokeURL:                     "https://example.com/revoke",
		AppInstallURL:                 "https://example.com/install",
		AppInstallationsURL:           "https://example.com/installations",
		NoRefresh:                     true,
		Scopes:                        []string{"user:email", "read:org"},
		ExtraTokenKeys:                []string{"extra", "token"},
		DeviceFlow:                    true,
		DeviceCodeURL:                 "https://example.com/device",
		Regex:                         "^https://example.com/.*$",
		DisplayName:                   "GitHub",
		DisplayIcon:                   "/static/icons/github.svg",
		MCPURL:                        "https://api.githubcopilot.com/mcp/",
		MCPToolAllowRegex:             ".*",
		MCPToolDenyRegex:              "create_gist",
		CodeChallengeMethodsSupported: []string{"S256"},
	}

	// Input the github section twice for testing a slice of configs.
	inputYAML := func() string {
		f := file(t, "githubcfg.yaml")
		lines := strings.SplitN(f, "\n", 2)
		// Append github config twice
		return f + lines[1]
	}()

	expected := []codersdk.ExternalAuthConfig{
		githubCfg, githubCfg,
	}

	dv := codersdk.DeploymentValues{}
	opts := dv.Options()
	// replace any tabs with the proper space indentation
	inputYAML = strings.ReplaceAll(inputYAML, "\t", "  ")

	// This is the order things are done in the cli, so just
	// keep it the same.
	var n yaml.Node
	err := yaml.Unmarshal([]byte(inputYAML), &n)
	require.NoError(t, err)

	err = n.Decode(&opts)
	require.NoError(t, err)
	require.ElementsMatchf(t, expected, dv.ExternalAuthConfigs.Value, "from yaml")

	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	err = enc.Encode(dv.ExternalAuthConfigs)
	require.NoError(t, err)

	// Because we only marshal the 1 section, the correct section name is not applied.
	output := strings.Replace(out.String(), "value:", "externalAuthProviders:", 1)
	require.Equal(t, inputYAML, output, "re-marshaled is the same as input")
}

func TestFeatureComparison(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Name     string
		A        codersdk.Feature
		B        codersdk.Feature
		Expected int
	}{
		{
			Name:     "Empty",
			Expected: 0,
		},
		// Entitlement check
		//		Entitled
		{
			Name:     "EntitledVsGracePeriod",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementGracePeriod},
			Expected: 1,
		},
		{
			Name: "EntitledVsGracePeriodLimits",
			A:    codersdk.Feature{Entitlement: codersdk.EntitlementEntitled},
			// Entitled should still win here
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementGracePeriod, Limit: ptr.Ref[int64](100), Actual: ptr.Ref[int64](50)},
			Expected: 1,
		},
		{
			Name:     "EntitledVsNotEntitled",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementNotEntitled},
			Expected: 3,
		},
		{
			Name:     "EntitledVsUnknown",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled},
			B:        codersdk.Feature{Entitlement: ""},
			Expected: 4,
		},
		//		GracePeriod
		{
			Name:     "GracefulVsNotEntitled",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementGracePeriod},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementNotEntitled},
			Expected: 2,
		},
		{
			Name:     "GracefulVsUnknown",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementGracePeriod},
			B:        codersdk.Feature{Entitlement: ""},
			Expected: 3,
		},
		//		NotEntitled
		{
			Name:     "NotEntitledVsUnknown",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementNotEntitled},
			B:        codersdk.Feature{Entitlement: ""},
			Expected: 1,
		},
		// --
		{
			Name:     "EntitledVsGracePeriodCapable",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref[int64](100), Actual: ptr.Ref[int64](200)},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementGracePeriod, Limit: ptr.Ref[int64](300), Actual: ptr.Ref[int64](200)},
			Expected: -1,
		},
		// UserLimits
		{
			// Tests an exceeded limit that is entitled vs a graceful limit that
			// is not exceeded. This is the edge case that we should use the graceful period
			// instead of the entitled.
			Name:     "UserLimitExceeded",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(100)), Actual: ptr.Ref(int64(200))},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementGracePeriod, Limit: ptr.Ref(int64(300)), Actual: ptr.Ref(int64(200))},
			Expected: -1,
		},
		{
			Name:     "UserLimitExceededNoEntitled",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(100)), Actual: ptr.Ref(int64(200))},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementNotEntitled, Limit: ptr.Ref(int64(300)), Actual: ptr.Ref(int64(200))},
			Expected: 3,
		},
		{
			Name:     "HigherLimit",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(110)), Actual: ptr.Ref(int64(200))},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(100)), Actual: ptr.Ref(int64(200))},
			Expected: 10, // Diff in the limit #
		},
		{
			Name:     "HigherActual",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(100)), Actual: ptr.Ref(int64(300))},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(100)), Actual: ptr.Ref(int64(200))},
			Expected: 100, // Diff in the actual #
		},
		{
			Name:     "LimitExists",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(100)), Actual: ptr.Ref(int64(50))},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: nil, Actual: ptr.Ref(int64(200))},
			Expected: 1,
		},
		{
			Name:     "LimitExistsGrace",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementGracePeriod, Limit: ptr.Ref(int64(100)), Actual: ptr.Ref(int64(50))},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementGracePeriod, Limit: nil, Actual: ptr.Ref(int64(200))},
			Expected: 1,
		},
		{
			Name:     "ActualExists",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(100)), Actual: ptr.Ref(int64(50))},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(100)), Actual: nil},
			Expected: 1,
		},
		{
			Name:     "NotNils",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(100)), Actual: ptr.Ref(int64(50))},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: nil, Actual: nil},
			Expected: 1,
		},
		{
			Name:     "EnabledVsDisabled",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Enabled: true, Limit: ptr.Ref(int64(300)), Actual: ptr.Ref(int64(200))},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(300)), Actual: ptr.Ref(int64(200))},
			Expected: 1,
		},
		{
			Name:     "NotNils",
			A:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: ptr.Ref(int64(100)), Actual: ptr.Ref(int64(50))},
			B:        codersdk.Feature{Entitlement: codersdk.EntitlementEntitled, Limit: nil, Actual: nil},
			Expected: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			r := tc.A.Compare(tc.B)
			logIt := !assert.Equal(t, tc.Expected, r)

			// Comparisons should be like addition. A - B = -1 * (B - A)
			r = tc.B.Compare(tc.A)
			logIt = logIt || !assert.Equalf(t, tc.Expected*-1, r, "the inverse comparison should also be true")
			if logIt {
				ad, _ := json.Marshal(tc.A)
				bd, _ := json.Marshal(tc.B)
				t.Logf("a = %s\nb = %s", ad, bd)
			}
		})
	}
}

// TestPremiumSuperSet tests that the "premium" feature set is a superset of the
// "enterprise" feature set.
func TestPremiumSuperSet(t *testing.T) {
	t.Parallel()

	enterprise := codersdk.FeatureSetEnterprise
	premium := codersdk.FeatureSetPremium

	// Premium > Enterprise
	require.Greater(t, len(premium.Features()), len(enterprise.Features()), "premium should have more features than enterprise")

	// Premium ⊃ Enterprise
	require.Subset(t, premium.Features(), enterprise.Features(), "premium should be a superset of enterprise. If this fails, update the premium feature set to include all enterprise features.")

	// Premium = All Features EXCEPT limit-based features.
	// TODO: In future release, also exclude addon features (f.IsAddonFeature()).
	expectedPremiumFeatures := []codersdk.FeatureName{}
	for _, feature := range codersdk.FeatureNames {
		if feature.UsesLimit() {
			continue
		}
		expectedPremiumFeatures = append(expectedPremiumFeatures, feature)
	}
	require.NotEmpty(t, expectedPremiumFeatures, "expectedPremiumFeatures should not be empty")
	require.ElementsMatch(t, premium.Features(), expectedPremiumFeatures, "premium should contain all features except usage limit features")

	// This check exists because if you misuse the slices.Delete, you can end up
	// with zero'd values.
	require.NotContains(t, enterprise.Features(), "", "enterprise should not contain empty string")
	require.NotContains(t, premium.Features(), "", "premium should not contain empty string")
}

func TestNotificationsCanBeDisabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                       string
		expectNotificationsEnabled bool
		environment                []serpent.EnvVar
	}{
		{
			name:                       "NoDeliveryMethodSet",
			environment:                []serpent.EnvVar{},
			expectNotificationsEnabled: false,
		},
		{
			name: "SMTP_DeliveryMethodSet",
			environment: []serpent.EnvVar{
				{
					Name:  "CODER_EMAIL_SMARTHOST",
					Value: "localhost:587",
				},
			},
			expectNotificationsEnabled: true,
		},
		{
			name: "Webhook_DeliveryMethodSet",
			environment: []serpent.EnvVar{
				{
					Name:  "CODER_NOTIFICATIONS_WEBHOOK_ENDPOINT",
					Value: "https://example.com/webhook",
				},
			},
			expectNotificationsEnabled: true,
		},
		{
			name: "WebhookAndSMTP_DeliveryMethodSet",
			environment: []serpent.EnvVar{
				{
					Name:  "CODER_NOTIFICATIONS_WEBHOOK_ENDPOINT",
					Value: "https://example.com/webhook",
				},
				{
					Name:  "CODER_EMAIL_SMARTHOST",
					Value: "localhost:587",
				},
			},
			expectNotificationsEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dv := codersdk.DeploymentValues{}
			opts := dv.Options()

			err := opts.ParseEnv(tt.environment)
			require.NoError(t, err)

			require.Equal(t, tt.expectNotificationsEnabled, dv.Notifications.Enabled())
		})
	}
}

func TestRetentionConfigParsing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		environment            []serpent.EnvVar
		expectedAuditLogs      time.Duration
		expectedConnectionLogs time.Duration
		expectedAPIKeys        time.Duration
	}{
		{
			name:                   "Defaults",
			environment:            []serpent.EnvVar{},
			expectedAuditLogs:      0,
			expectedConnectionLogs: 0,
			expectedAPIKeys:        7 * 24 * time.Hour, // 7 days default
		},
		{
			name: "IndividualRetentionSet",
			environment: []serpent.EnvVar{
				{Name: "CODER_AUDIT_LOGS_RETENTION", Value: "30d"},
				{Name: "CODER_CONNECTION_LOGS_RETENTION", Value: "60d"},
				{Name: "CODER_API_KEYS_RETENTION", Value: "14d"},
			},
			expectedAuditLogs:      30 * 24 * time.Hour,
			expectedConnectionLogs: 60 * 24 * time.Hour,
			expectedAPIKeys:        14 * 24 * time.Hour,
		},
		{
			name: "AllRetentionSet",
			environment: []serpent.EnvVar{
				{Name: "CODER_AUDIT_LOGS_RETENTION", Value: "365d"},
				{Name: "CODER_CONNECTION_LOGS_RETENTION", Value: "30d"},
				{Name: "CODER_API_KEYS_RETENTION", Value: "0"},
			},
			expectedAuditLogs:      365 * 24 * time.Hour,
			expectedConnectionLogs: 30 * 24 * time.Hour,
			expectedAPIKeys:        0, // Explicitly disabled
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dv := codersdk.DeploymentValues{}
			opts := dv.Options()

			err := opts.SetDefaults()
			require.NoError(t, err)

			err = opts.ParseEnv(tt.environment)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedAuditLogs, dv.Retention.AuditLogs.Value(), "audit logs retention mismatch")
			assert.Equal(t, tt.expectedConnectionLogs, dv.Retention.ConnectionLogs.Value(), "connection logs retention mismatch")
			assert.Equal(t, tt.expectedAPIKeys, dv.Retention.APIKeys.Value(), "api keys retention mismatch")
		})
	}
}

func TestChatAIGatewayRoutingEnabledDefault(t *testing.T) {
	t.Parallel()

	dv := codersdk.DeploymentValues{}
	opts := dv.Options()
	require.NoError(t, opts.SetDefaults())
	require.True(t, dv.AI.Chat.AIGatewayRoutingEnabled.Value())
}

func TestAIBudgetConfigParsing(t *testing.T) {
	t.Parallel()

	t.Run("Defaults", func(t *testing.T) {
		t.Parallel()

		dv := codersdk.DeploymentValues{}
		opts := dv.Options()

		require.NoError(t, opts.SetDefaults())

		assert.Equal(t, string(codersdk.AIBudgetPolicyHighest), dv.AI.BridgeConfig.BudgetPolicy)
		assert.Equal(t, string(codersdk.AIBudgetPeriodMonth), dv.AI.BridgeConfig.BudgetPeriod)
	})

	t.Run("AcceptsSupportedValues", func(t *testing.T) {
		t.Parallel()

		dv := codersdk.DeploymentValues{}
		opts := dv.Options()

		require.NoError(t, opts.SetDefaults())
		require.NoError(t, opts.ParseEnv([]serpent.EnvVar{
			{Name: "CODER_AI_BUDGET_POLICY", Value: string(codersdk.AIBudgetPolicyHighest)},
			{Name: "CODER_AI_BUDGET_PERIOD", Value: string(codersdk.AIBudgetPeriodMonth)},
		}))

		assert.Equal(t, string(codersdk.AIBudgetPolicyHighest), dv.AI.BridgeConfig.BudgetPolicy)
		assert.Equal(t, string(codersdk.AIBudgetPeriodMonth), dv.AI.BridgeConfig.BudgetPeriod)
	})

	t.Run("RejectsUnsupportedPolicy", func(t *testing.T) {
		t.Parallel()

		dv := codersdk.DeploymentValues{}
		opts := dv.Options()

		require.NoError(t, opts.SetDefaults())
		err := opts.ParseEnv([]serpent.EnvVar{
			{Name: "CODER_AI_BUDGET_POLICY", Value: "invalid"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid choice")
	})

	t.Run("RejectsUnsupportedPeriod", func(t *testing.T) {
		t.Parallel()

		dv := codersdk.DeploymentValues{}
		opts := dv.Options()

		require.NoError(t, opts.SetDefaults())
		err := opts.ParseEnv([]serpent.EnvVar{
			{Name: "CODER_AI_BUDGET_PERIOD", Value: "invalid"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid choice")
	})
}

func TestNewAIBudgetPolicyFromString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want codersdk.AIBudgetPolicy
	}{
		{name: "supported", in: "highest", want: codersdk.AIBudgetPolicyHighest},
		{name: "empty falls back to highest", in: "", want: codersdk.AIBudgetPolicyHighest},
		{name: "unknown falls back to highest", in: "unsupported", want: codersdk.AIBudgetPolicyHighest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, codersdk.NewAIBudgetPolicyFromString(tt.in))
		})
	}
}

func TestComputeMaxIdleConns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		maxOpen        int
		configuredIdle string
		expectedIdle   int
		expectError    bool
		errorContains  string
	}{
		{
			name:           "auto_default_10_open",
			maxOpen:        10,
			configuredIdle: "auto",
			expectedIdle:   3, // 10/3 = 3
		},
		{
			name:           "auto_with_whitespace",
			maxOpen:        10,
			configuredIdle: " auto ",
			expectedIdle:   3, // 10/3 = 3
		},
		{
			name:           "auto_30_open",
			maxOpen:        30,
			configuredIdle: "auto",
			expectedIdle:   10, // 30/3 = 10
		},
		{
			name:           "auto_minimum_1",
			maxOpen:        1,
			configuredIdle: "auto",
			expectedIdle:   1, // 1/3 = 0, but minimum is 1
		},
		{
			name:           "auto_minimum_2_open",
			maxOpen:        2,
			configuredIdle: "auto",
			expectedIdle:   1, // 2/3 = 0, but minimum is 1
		},
		{
			name:           "auto_3_open",
			maxOpen:        3,
			configuredIdle: "auto",
			expectedIdle:   1, // 3/3 = 1
		},
		{
			name:           "explicit_equal_to_max",
			maxOpen:        10,
			configuredIdle: "10",
			expectedIdle:   10,
		},
		{
			name:           "explicit_less_than_max",
			maxOpen:        10,
			configuredIdle: "5",
			expectedIdle:   5,
		},
		{
			name:           "explicit_with_whitespace",
			maxOpen:        10,
			configuredIdle: " 5 ",
			expectedIdle:   5,
		},
		{
			name:           "explicit_0",
			maxOpen:        10,
			configuredIdle: "0",
			expectedIdle:   0,
		},
		{
			name:           "error_exceeds_max",
			maxOpen:        10,
			configuredIdle: "15",
			expectError:    true,
			errorContains:  "cannot exceed",
		},
		{
			name:           "error_exceeds_max_by_1",
			maxOpen:        10,
			configuredIdle: "11",
			expectError:    true,
			errorContains:  "cannot exceed",
		},
		{
			name:           "error_invalid_string",
			maxOpen:        10,
			configuredIdle: "invalid",
			expectError:    true,
			errorContains:  "must be \"auto\" or >= 0",
		},
		{
			name:           "error_negative",
			maxOpen:        10,
			configuredIdle: "-1",
			expectError:    true,
			errorContains:  "must be \"auto\" or >= 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := codersdk.ComputeMaxIdleConns(tt.maxOpen, tt.configuredIdle)
			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedIdle, result)
			}
		})
	}
}

func TestHTTPCookieConfigMiddleware(t *testing.T) {
	t.Parallel()

	// Realistic cookies that are always present in production.
	// These cookies are added to every test.
	baseCookies := []*http.Cookie{
		{Name: "_ga", Value: "GA1.1.661026807.1770083336"},
		{Name: "_ga_G0Q1B9GRC0", Value: "GS2.1.s1771343727$o49$g1$t1771343993$j48$l0$h0"},
		{Name: "csrf_token", Value: "gDiKk8GjTM2iCUHAPfN9GlC+DGjzAprlLi2vJ+5TBU0="},
	}

	cases := []struct {
		name            string
		cfg             codersdk.HTTPCookieConfig
		extraCookies    []*http.Cookie
		expectedCookies map[string]string // cookie name -> value that handler should see
		expectedDeleted []string          // if any cookies are supposed to be deleted via Set-Cookie
	}{
		{
			name: "Disabled_PassesThrough",
			cfg:  codersdk.HTTPCookieConfig{},
			extraCookies: []*http.Cookie{
				{Name: codersdk.SessionTokenCookie, Value: "token123"},
			},
			expectedCookies: map[string]string{
				codersdk.SessionTokenCookie: "token123",
			},
		},
		{
			name: "Enabled_StripsPrefixFromCookie",
			cfg:  codersdk.HTTPCookieConfig{EnableHostPrefix: true},
			extraCookies: []*http.Cookie{
				{Name: "__Host-" + codersdk.SessionTokenCookie, Value: "token123"},
			},
			expectedCookies: map[string]string{
				codersdk.SessionTokenCookie: "token123",
			},
		},
		{
			name: "Enabled_DeletesUnprefixedCookie",
			cfg:  codersdk.HTTPCookieConfig{EnableHostPrefix: true},
			extraCookies: []*http.Cookie{
				// Unprefixed cookie that should be in the "to prefix" list.
				{Name: codersdk.SessionTokenCookie, Value: "unprefixed-token"},
			},
			expectedCookies: map[string]string{
				// Session token should NOT be present - it was deleted.
			},
			expectedDeleted: []string{codersdk.SessionTokenCookie},
		},
		{
			name: "Enabled_BothPrefixedAndUnprefixed",
			cfg:  codersdk.HTTPCookieConfig{EnableHostPrefix: true},
			extraCookies: []*http.Cookie{
				// Browser might send both during migration.
				{Name: codersdk.SessionTokenCookie, Value: "unprefixed-token"},
				{Name: "__Host-" + codersdk.SessionTokenCookie, Value: "prefixed-token"},
			},
			expectedCookies: map[string]string{
				codersdk.SessionTokenCookie: "prefixed-token", // Prefixed wins.
			},
			expectedDeleted: []string{codersdk.SessionTokenCookie},
		},
		{
			name: "Enabled_MultiplePrefixedCookies",
			cfg:  codersdk.HTTPCookieConfig{EnableHostPrefix: true},
			extraCookies: []*http.Cookie{
				{Name: "__Host-" + codersdk.SessionTokenCookie, Value: "session"},
				{Name: "__Host-SomeOtherCookie", Value: "other-cookie"},
				{Name: "__Host-Santa", Value: "santa"},
			},
			expectedCookies: map[string]string{
				codersdk.SessionTokenCookie: "session",
				"__Host-SomeOtherCookie":    "other-cookie",
				"__Host-Santa":              "santa",
			},
		},
		{
			name: "Enabled_UnrelatedCookiesUnchanged",
			cfg:  codersdk.HTTPCookieConfig{EnableHostPrefix: true},
			extraCookies: []*http.Cookie{
				{Name: "custom_cookie", Value: "custom-value"},
				{Name: "__Host-" + codersdk.SessionTokenCookie, Value: "session"},
				{Name: "__Host-foobar", Value: "do-not-change-me"},
			},
			expectedCookies: map[string]string{
				"custom_cookie":             "custom-value",
				codersdk.SessionTokenCookie: "session",
				"__Host-foobar":             "do-not-change-me",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var handlerCookies []*http.Cookie
			handler := tc.cfg.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCookies = r.Cookies()
			}))

			req := httptest.NewRequest("GET", "/", nil)
			for _, c := range baseCookies {
				req.AddCookie(c)
			}
			for _, c := range tc.extraCookies {
				req.AddCookie(c)
			}

			rw := httptest.NewRecorder()
			handler.ServeHTTP(rw, req)

			// Verify cookies seen by handler.
			gotCookies := make(map[string]string)
			for _, c := range handlerCookies {
				gotCookies[c.Name] = c.Value
			}

			for _, v := range baseCookies {
				tc.expectedCookies[v.Name] = v.Value
			}
			assert.Equal(t, tc.expectedCookies, gotCookies)

			// Verify Set-Cookie header for deletion.
			setCookies := rw.Result().Cookies()
			if len(tc.expectedDeleted) > 0 {
				assert.NotEmpty(t, setCookies, "expected Set-Cookie header for cookie deletion")
				expDel := make(map[string]struct{})
				for _, name := range tc.expectedDeleted {
					expDel[name] = struct{}{}
				}
				// Verify it's a deletion (MaxAge < 0).
				for _, c := range setCookies {
					assert.Less(t, c.MaxAge, 0, "Set-Cookie should have MaxAge < 0 for deletion")
					delete(expDel, c.Name)
				}
				require.Empty(t, expDel, "expected Set-Cookie header for deletion")
			} else {
				assert.Empty(t, setCookies, "did not expect Set-Cookie header")
			}
		})
	}
}

func BenchmarkHTTPCookieConfigMiddleware(b *testing.B) {
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	// Realistic cookies that are always present in production.
	baseCookies := []*http.Cookie{
		{Name: "_ga", Value: "GA1.1.661026807.1770083336"},
		{Name: "_ga_G0Q1B9GRC0", Value: "GS2.1.s1771343727$o49$g1$t1771343993$j48$l0$h0"},
		{Name: "csrf_token", Value: "gDiKk8GjTM2iCUHAPfN9GlC+DGjzAprlLi2vJ+5TBU0="},
	}

	cases := []struct {
		name         string
		cfg          codersdk.HTTPCookieConfig
		extraCookies []*http.Cookie
	}{
		{
			name: "Disabled",
			cfg:  codersdk.HTTPCookieConfig{},
			extraCookies: []*http.Cookie{
				{Name: codersdk.SessionTokenCookie, Value: "KybJV9fNul-u11vlll9wiF6eLQDxBVucD"},
			},
		},
		{
			name: "Enabled_NoPrefixedCookies",
			cfg:  codersdk.HTTPCookieConfig{EnableHostPrefix: true},
			extraCookies: []*http.Cookie{
				{Name: codersdk.SessionTokenCookie, Value: "KybJV9fNul-u11vlll9wiF6eLQDxBVucD"},
			},
		},
		{
			name: "Enabled_WithPrefixedCookie",
			cfg:  codersdk.HTTPCookieConfig{EnableHostPrefix: true},
			extraCookies: []*http.Cookie{
				{Name: "__Host-" + codersdk.SessionTokenCookie, Value: "KybJV9fNul-u11vlll9wiF6eLQDxBVucD"},
			},
		},
		{
			name: "Enabled_MultiplePrefixedCookies",
			cfg:  codersdk.HTTPCookieConfig{EnableHostPrefix: true},
			extraCookies: []*http.Cookie{
				{Name: "__Host-" + codersdk.SessionTokenCookie, Value: "KybJV9fNul-u11vlll9wiF6eLQDxBVucD"},
				{Name: "__Host-" + codersdk.PathAppSessionTokenCookie, Value: "xyz123"},
				{Name: "__Host-" + codersdk.SubdomainAppSessionTokenCookie, Value: "abc456"},
				{Name: "__Host-" + "foobar", Value: "do-not-change-me"},
			},
		},
		{
			name: "Enabled_NonSessionPrefixedCookies",
			cfg:  codersdk.HTTPCookieConfig{EnableHostPrefix: true},
			extraCookies: []*http.Cookie{
				{Name: "__Host-" + codersdk.SessionTokenCookie, Value: "KybJV9fNul-u11vlll9wiF6eLQDxBVucD"},
			},
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			handler := tc.cfg.Middleware(noop)
			rw := httptest.NewRecorder()

			allCookies := make([]*http.Cookie, 1, len(baseCookies))
			copy(allCookies, baseCookies)
			// Combine base cookies with test-specific cookies.
			allCookies = append(allCookies, tc.extraCookies...)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest("GET", "/", nil)
				for _, c := range allCookies {
					req.AddCookie(c)
				}
				handler.ServeHTTP(rw, req)
			}
		})
	}
}
