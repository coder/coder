package cli

import (
	"bytes"
	"context"
	"crypto/tls"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func Test_configureCipherSuites(t *testing.T) {
	t.Parallel()

	cipherNames := func(ciphers []*tls.CipherSuite) []string {
		var names []string
		for _, c := range ciphers {
			names = append(names, c.Name)
		}
		return names
	}

	cipherIDs := func(ciphers []*tls.CipherSuite) []uint16 {
		var ids []uint16
		for _, c := range ciphers {
			ids = append(ids, c.ID)
		}
		return ids
	}

	cipherByName := func(cipher string) *tls.CipherSuite {
		for _, c := range append(tls.CipherSuites(), tls.InsecureCipherSuites()...) {
			if cipher == c.Name {
				c := c
				return c
			}
		}
		return nil
	}

	tests := []struct {
		name          string
		wantErr       string
		wantWarnings  []string
		inputCiphers  []string
		minTLS        uint16
		maxTLS        uint16
		allowInsecure bool
		expectCiphers []uint16
	}{
		{
			name:          "AllSecure",
			minTLS:        tls.VersionTLS10,
			maxTLS:        tls.VersionTLS13,
			inputCiphers:  cipherNames(tls.CipherSuites()),
			wantWarnings:  []string{},
			expectCiphers: cipherIDs(tls.CipherSuites()),
		},
		{
			name:          "AllowInsecure",
			minTLS:        tls.VersionTLS10,
			maxTLS:        tls.VersionTLS13,
			inputCiphers:  append(cipherNames(tls.CipherSuites()), tls.InsecureCipherSuites()[0].Name),
			allowInsecure: true,
			wantWarnings: []string{
				"insecure tls cipher specified",
			},
			expectCiphers: append(cipherIDs(tls.CipherSuites()), tls.InsecureCipherSuites()[0].ID),
		},
		{
			name:          "AllInsecure",
			minTLS:        tls.VersionTLS10,
			maxTLS:        tls.VersionTLS13,
			inputCiphers:  append(cipherNames(tls.CipherSuites()), cipherNames(tls.InsecureCipherSuites())...),
			allowInsecure: true,
			wantWarnings: []string{
				"insecure tls cipher specified",
			},
			expectCiphers: append(cipherIDs(tls.CipherSuites()), cipherIDs(tls.InsecureCipherSuites())...),
		},
		{
			// Providing ciphers that are not compatible with any tls version
			// enabled should generate a warning.
			name:   "ExcessiveCiphers",
			minTLS: tls.VersionTLS10,
			maxTLS: tls.VersionTLS11,
			inputCiphers: []string{
				"TLS_RSA_WITH_AES_128_CBC_SHA",
				// Only for TLS 1.3
				"TLS_AES_128_GCM_SHA256",
			},
			allowInsecure: true,
			wantWarnings: []string{
				"cipher not supported for tls versions",
			},
			expectCiphers: cipherIDs([]*tls.CipherSuite{
				cipherByName("TLS_RSA_WITH_AES_128_CBC_SHA"),
				cipherByName("TLS_AES_128_GCM_SHA256"),
			}),
		},
		// Errors
		{
			name:         "NotRealCiphers",
			minTLS:       tls.VersionTLS10,
			maxTLS:       tls.VersionTLS13,
			inputCiphers: []string{"RSA-Fake"},
			wantErr:      "unsupported tls ciphers",
		},
		{
			name:    "NoCiphers",
			minTLS:  tls.VersionTLS10,
			maxTLS:  tls.VersionTLS13,
			wantErr: "no tls ciphers supported",
		},
		{
			name:         "InsecureNotAllowed",
			minTLS:       tls.VersionTLS10,
			maxTLS:       tls.VersionTLS13,
			inputCiphers: append(cipherNames(tls.CipherSuites()), tls.InsecureCipherSuites()[0].Name),
			wantErr:      "insecure tls ciphers specified",
		},
		{
			name:         "TLS1.3",
			minTLS:       tls.VersionTLS13,
			maxTLS:       tls.VersionTLS13,
			inputCiphers: cipherNames(tls.CipherSuites()),
			wantErr:      "'--tls-ciphers' cannot be specified when using minimum tls version 1.3",
		},
		{
			name:   "TLSUnsupported",
			minTLS: tls.VersionTLS10,
			maxTLS: tls.VersionTLS13,
			// TLS_RSA_WITH_AES_128_GCM_SHA256 only supports tls 1.2
			inputCiphers: []string{"TLS_RSA_WITH_AES_128_GCM_SHA256"},
			wantErr:      "no tls ciphers supported for tls versions",
		},
		{
			name:    "Min>Max",
			minTLS:  tls.VersionTLS13,
			maxTLS:  tls.VersionTLS12,
			wantErr: "minimum tls version (TLS 1.3) cannot be greater than maximum tls version (TLS 1.2)",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			var out bytes.Buffer
			logger := slog.Make(sloghuman.Sink(&out))

			found, err := configureCipherSuites(ctx, logger, tt.inputCiphers, tt.allowInsecure, tt.minTLS, tt.maxTLS)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err, "no error")
				require.ElementsMatch(t, tt.expectCiphers, found, "expected ciphers")
				if len(tt.wantWarnings) > 0 {
					logger.Sync()
					for _, w := range tt.wantWarnings {
						assert.Contains(t, out.String(), w, "expected warning")
					}
				}
			}
		})
	}
}

func TestRedirectHTTPToHTTPSDeprecation(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name     string
		environ  clibase.Environ
		flags    []string
		expected bool
	}{
		{
			name:     "AllUnset",
			environ:  clibase.Environ{},
			flags:    []string{},
			expected: false,
		},
		{
			name:     "CODER_TLS_REDIRECT_HTTP=true",
			environ:  clibase.Environ{{Name: "CODER_TLS_REDIRECT_HTTP", Value: "true"}},
			flags:    []string{},
			expected: true,
		},
		{
			name:     "CODER_TLS_REDIRECT_HTTP_TO_HTTPS=true",
			environ:  clibase.Environ{{Name: "CODER_TLS_REDIRECT_HTTP_TO_HTTPS", Value: "true"}},
			flags:    []string{},
			expected: true,
		},
		{
			name:     "CODER_TLS_REDIRECT_HTTP=false",
			environ:  clibase.Environ{{Name: "CODER_TLS_REDIRECT_HTTP", Value: "false"}},
			flags:    []string{},
			expected: false,
		},
		{
			name:     "CODER_TLS_REDIRECT_HTTP_TO_HTTPS=false",
			environ:  clibase.Environ{{Name: "CODER_TLS_REDIRECT_HTTP_TO_HTTPS", Value: "false"}},
			flags:    []string{},
			expected: false,
		},
		{
			name:     "--tls-redirect-http-to-https",
			environ:  clibase.Environ{},
			flags:    []string{"--tls-redirect-http-to-https"},
			expected: true,
		},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			logger := slogtest.Make(t, nil)
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			_ = flags.Bool("tls-redirect-http-to-https", true, "")
			err := flags.Parse(tc.flags)
			require.NoError(t, err)
			inv := (&clibase.Invocation{Environ: tc.environ}).WithTestParsedFlags(t, flags)
			cfg := &codersdk.DeploymentValues{}
			opts := cfg.Options()
			err = opts.SetDefaults()
			require.NoError(t, err)
			redirectHTTPToHTTPSDeprecation(ctx, logger, inv, cfg)
			require.Equal(t, tc.expected, cfg.RedirectToAccessURL.Value())
		})
	}
}
