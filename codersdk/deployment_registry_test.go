package codersdk_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func TestNormalizeTemplateBuilderRegistryURL(t *testing.T) {
	t.Parallel()

	t.Run("Accepts", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			in   string
			want string
		}{
			{"Empty", "", codersdk.DefaultTemplateBuilderRegistryURL},
			{"Whitespaceonly", "   ", codersdk.DefaultTemplateBuilderRegistryURL},
			{"BareHost", "registry.coder.com", "registry.coder.com"},
			{"BareHostSurroundingSpace", "  mirror.internal.example  ", "mirror.internal.example"},
			{"HostPort", "mirror.example.com:8443", "mirror.example.com:8443"},
			{"HTTPSStripped", "https://mirror.example.com", "mirror.example.com"},
			{"HTTPStripped", "http://mirror.example.com", "mirror.example.com"},
			{"UppercaseSchemeStripped", "HTTPS://mirror.example.com", "mirror.example.com"},
			{"TrailingSlashStripped", "https://mirror.example.com/", "mirror.example.com"},
			{"TrailingSlashesStripped", "mirror.example.com///", "mirror.example.com"},
			// svchost.ForComparison, the parser Terraform uses, defines these:
			// an uppercase host is lowercased, a Unicode host is normalized to
			// punycode, a trailing dot is a valid FQDN, and :443 is the default
			// port and is dropped. The old hand-rolled regex rejected the middle
			// two and preserved :443, diverging from what `terraform init` does.
			{"HostLowercased", "Registry.Coder.Com", "registry.coder.com"},
			{"UnicodeHostPunycoded", "m\u00fcnchen.example", "xn--mnchen-3ya.example"},
			{"TrailingDotFQDN", "registry.coder.com.", "registry.coder.com."},
			{"DefaultPortDropped", "mirror.example.com:443", "mirror.example.com"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				got, err := codersdk.NormalizeTemplateBuilderRegistryURL(tc.in)
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
			})
		}
	})

	t.Run("Rejects", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			in   string
		}{
			{"DoubledScheme", "https://https://mirror.example.com"},
			{"Path", "mirror.example.com/coder"},
			{"Interpolation", "mirror.example.com/${var.evil}"},
			{"InteriorInterpolation", "mirror${var.evil}.example.com"},
			{"Directive", "mirror.example.com/%{if true}"},
			{"Backslash", `mirror.example.com\x`},
			{"Quote", `mirror.example.com/"`},
			{"InteriorSpace", "mirror .example.com"},
			{"NonBreakingSpace", "mirror\u00a0.example.com"},
			{"VerticalTab", "mirror\v.example.com"},
			{"SchemeOnly", "https://"},
			{"LeadingDot", ".mirror.example.com"},
			// New coverage for the over-rejection/under-rejection class: the
			// value is validated by svchost, so it now agrees with Terraform on
			// each of these (raw punycode and underscores are rejected, a port
			// over 65535 is rejected, and an IPv6 literal is not a registry host).
			{"RawPunycode", "xn--mnchen-3ya.example"},
			{"Underscore", "under_score.example"},
			{"PortTooHigh", "mirror.example.com:99999"},
			{"IPv6", "[::1]:8443"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				_, err := codersdk.NormalizeTemplateBuilderRegistryURL(tc.in)
				require.Error(t, err)
				require.ErrorContains(t, err, "bare host")
			})
		}
	})

	t.Run("ErrorNamesBrokenRuleWithoutEchoingInput", func(t *testing.T) {
		t.Parallel()
		// The rejection names the specific broken rule (so an operator can fix
		// it) but never echoes the input, so a credential is not disclosed, and
		// it no longer claims "no scheme" as a cause, since a scheme is stripped
		// before validation and can never be the reason.
		cases := []struct {
			name, in, reason string
		}{
			{"Credentials", "https://user:s3cr3t-token@mirror.example.com", "credentials"},
			{"Path", "mirror.example.com/coder", "path"},
			{"IPv6", "[::1]:8443", "IPv6"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				_, err := codersdk.NormalizeTemplateBuilderRegistryURL(tc.in)
				require.Error(t, err)
				require.ErrorContains(t, err, "bare host")
				require.ErrorContains(t, err, tc.reason)
				require.NotContains(t, err.Error(), "scheme")
			})
		}

		_, err := codersdk.NormalizeTemplateBuilderRegistryURL("https://user:s3cr3t-token@mirror.example.com")
		require.Error(t, err)
		require.NotContains(t, err.Error(), "s3cr3t-token")
	})
}

// TestDeploymentValues_Validate_TemplateBuilderRegistryURL covers the config
// boundary. A malformed CODER_TEMPLATE_BUILDER_REGISTRY_URL must fail at server
// start naming the option, regardless of whether it was set via flag/env or
// YAML, and must not block boot when the template builder is disabled.
func TestDeploymentValues_Validate_TemplateBuilderRegistryURL(t *testing.T) {
	t.Parallel()

	// mkValid returns a DeploymentValues that passes Validate() except for the
	// template builder registry URL, so that check is what each case exercises.
	mkValid := func() *codersdk.DeploymentValues {
		dv := &codersdk.DeploymentValues{}
		dv.Sessions.DefaultDuration = serpent.Duration(time.Hour)
		dv.Sessions.RefreshDefaultDuration = serpent.Duration(48 * time.Hour)
		return dv
	}

	t.Run("Cases", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name     string
			url      string
			disabled bool
			wantErr  string
		}{
			{name: "EmptyOK"},
			{name: "BareHostOK", url: "mirror.internal.example"},
			{name: "HostPortOK", url: "mirror.internal.example:8443"},
			{name: "SchemeStrippedOK", url: "https://mirror.internal.example"},
			{name: "PathRejected", url: "mirror.internal.example/coder", wantErr: "bare host"},
			{name: "CredentialsRejected", url: "https://user:tok@mirror.example.com", wantErr: "bare host"},
			// A disabled template builder must not block boot on an inert value,
			// so an upgrade that tightens the shape check cannot take down a
			// deployment that has the feature turned off.
			{name: "DisabledSkipsValidation", url: "https://user:tok@mirror.example.com/bad", disabled: true},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				dv := mkValid()
				dv.TemplateBuilder.Disabled = serpent.Bool(tc.disabled)
				dv.TemplateBuilder.RegistryURL = serpent.String(tc.url)
				err := dv.Validate()
				if tc.wantErr == "" {
					require.NoError(t, err)
					return
				}
				require.ErrorContains(t, err, tc.wantErr)
			})
		}
	})

	t.Run("YAMLPathValidated", func(t *testing.T) {
		t.Parallel()
		// A serpent validator only runs in Set, which the YAML path bypasses, so
		// validating in Set would let a YAML-configured value skip the check
		// entirely. Validate() runs after all sources merge, so the value set
		// here through YAML is caught. Unmarshal before applying any defaults, the
		// order the server uses, so the YAML value is the option's source (a
		// default-sourced option is left untouched by UnmarshalYAML).
		dv := &codersdk.DeploymentValues{}
		dv.Sessions.DefaultDuration = serpent.Duration(time.Hour)
		dv.Sessions.RefreshDefaultDuration = serpent.Duration(48 * time.Hour)
		opts := dv.Options()

		const credURL = "https://user:s3cr3t-token@mirror.example.com"
		doc := map[string]any{"templateBuilder": map[string]any{"registryURL": credURL}}
		var node yaml.Node
		require.NoError(t, node.Encode(doc))
		require.NoError(t, opts.UnmarshalYAML(&node))
		require.Equal(t, credURL, dv.TemplateBuilder.RegistryURL.Value(),
			"YAML unmarshal stores the value verbatim; validation happens in Validate()")

		err := dv.Validate()
		require.ErrorContains(t, err, "bare host")
		require.NotContains(t, err.Error(), "s3cr3t-token")
	})
}
