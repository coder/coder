package codersdk_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
			{"Empty", "", ""},
			{"WhitespaceOnly", "   ", ""},
			{"BareHost", "registry.coder.com", "registry.coder.com"},
			{"SurroundingSpaceTrimmed", "  mirror.internal  ", "mirror.internal"},
			{"HostPort", "mirror.example.com:8443", "mirror.example.com:8443"},
			{"IPv6HostPort", "[::1]:8443", "[::1]:8443"},
			// An accidental scheme and trailing slash are stripped rather than
			// rejected, so a value pasted as a URL still resolves to a host.
			{"HTTPSStripped", "https://mirror.example.com", "mirror.example.com"},
			{"HTTPStripped", "http://mirror.example.com", "mirror.example.com"},
			{"TrailingSlashStripped", "mirror.example.com/", "mirror.example.com"},
			{"SchemePortAndSlash", "https://mirror.example.com:8443/", "mirror.example.com:8443"},
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
			{"Path", "mirror.example.com/coder"},
			{"NonHTTPScheme", "git://mirror.example.com"},
			{"UppercaseScheme", "HTTPS://mirror.example.com"},
			{"SchemeAndPath", "https://mirror.example.com/coder"},
			{"Query", "mirror.example.com?a=b"},
			{"Fragment", "mirror.example.com#frag"},
			{"DoubledScheme", "https://https://mirror.example.com"},
			{"InteriorSpace", "mirror .example.com"},
			{"PortOnly", ":8443"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				_, err := codersdk.NormalizeTemplateBuilderRegistryURL(tc.in)
				require.ErrorContains(t, err, "bare host")
			})
		}
	})

	t.Run("RejectsCredentialsWithoutEcho", func(t *testing.T) {
		t.Parallel()
		// Userinfo survives the scheme strip and must be rejected, not accepted,
		// and the rejection must not echo the credential.
		_, err := codersdk.NormalizeTemplateBuilderRegistryURL("https://user:s3cr3t-token@mirror.example.com")
		require.ErrorContains(t, err, "bare host")
		require.NotContains(t, err.Error(), "s3cr3t-token")
	})
}

// TestDeploymentValues_Validate_TemplateBuilderRegistryURL covers the config
// boundary: a malformed CODER_TEMPLATE_BUILDER_REGISTRY_URL must fail at server
// start naming the option, but must not block boot when the builder is disabled.
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

	cases := []struct {
		name     string
		url      string
		disabled bool
		wantErr  string
	}{
		{name: "EmptyOK"},
		{name: "BareHostOK", url: "mirror.internal.example"},
		{name: "HostPortOK", url: "mirror.internal.example:8443"},
		// A scheme is stripped, not rejected, so an existing deployment that set
		// a scheme'd value keeps booting after upgrade.
		{name: "SchemeStrippedOK", url: "https://mirror.internal.example"},
		{name: "PathRejected", url: "mirror.internal.example/coder", wantErr: "bare host"},
		{name: "CredentialsRejected", url: "https://user:tok@mirror.example.com", wantErr: "bare host"},
		// A disabled template builder must not block boot on an inert value, so
		// tightening the check in an upgrade cannot take down a disabled deployment.
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
}
