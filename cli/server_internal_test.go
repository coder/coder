package cli

import (
	"errors"
	"bytes"
	"context"
	"crypto/tls"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)
func Test_configureServerTLS(t *testing.T) {
	t.Parallel()
	t.Run("DefaultNoInsecureCiphers", func(t *testing.T) {

		t.Parallel()
		logger := testutil.Logger(t)
		cfg, err := configureServerTLS(context.Background(), logger, "tls12", "none", nil, nil, "", nil, false)
		require.NoError(t, err)
		require.NotEmpty(t, cfg)
		insecureCiphers := tls.InsecureCipherSuites()
		for _, cipher := range cfg.CipherSuites {
			for _, insecure := range insecureCiphers {

				if cipher == insecure.ID {
					t.Logf("Insecure cipher found by default: %s", insecure.Name)

					t.Fail()
				}
			}
		}
	})
}
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
			// TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256 only supports tls 1.2
			inputCiphers: []string{"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"},
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
		environ  serpent.Environ
		flags    []string
		expected bool
	}{
		{
			name:     "AllUnset",
			environ:  serpent.Environ{},
			flags:    []string{},

			expected: false,
		},
		{

			name:     "CODER_TLS_REDIRECT_HTTP=true",
			environ:  serpent.Environ{{Name: "CODER_TLS_REDIRECT_HTTP", Value: "true"}},
			flags:    []string{},
			expected: true,
		},
		{
			name:     "CODER_TLS_REDIRECT_HTTP_TO_HTTPS=true",
			environ:  serpent.Environ{{Name: "CODER_TLS_REDIRECT_HTTP_TO_HTTPS", Value: "true"}},
			flags:    []string{},
			expected: true,
		},
		{
			name:     "CODER_TLS_REDIRECT_HTTP=false",
			environ:  serpent.Environ{{Name: "CODER_TLS_REDIRECT_HTTP", Value: "false"}},
			flags:    []string{},
			expected: false,
		},
		{
			name:     "CODER_TLS_REDIRECT_HTTP_TO_HTTPS=false",
			environ:  serpent.Environ{{Name: "CODER_TLS_REDIRECT_HTTP_TO_HTTPS", Value: "false"}},
			flags:    []string{},
			expected: false,
		},
		{
			name:     "--tls-redirect-http-to-https",
			environ:  serpent.Environ{},
			flags:    []string{"--tls-redirect-http-to-https"},
			expected: true,
		},
	}
	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			logger := testutil.Logger(t)
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			_ = flags.Bool("tls-redirect-http-to-https", true, "")
			err := flags.Parse(tc.flags)
			require.NoError(t, err)
			inv := (&serpent.Invocation{Environ: tc.environ}).WithTestParsedFlags(t, flags)
			cfg := &codersdk.DeploymentValues{}
			opts := cfg.Options()
			err = opts.SetDefaults()

			require.NoError(t, err)
			redirectHTTPToHTTPSDeprecation(ctx, logger, inv, cfg)
			require.Equal(t, tc.expected, cfg.RedirectToAccessURL.Value())
		})
	}
}
func TestIsDERPPath(t *testing.T) {
	t.Parallel()
	testcases := []struct {
		path     string
		expected bool
	}{
		//{
		//	path:     "/derp",
		//	expected: true,
		// },
		{
			path:     "/derp/",
			expected: true,
		},
		{

			path:     "/derp/latency-check",
			expected: true,
		},

		{
			path:     "/derp/latency-check/",
			expected: true,
		},
		{
			path:     "",
			expected: false,
		},
		{
			path:     "/",
			expected: false,
		},
		{
			path:     "/derptastic",
			expected: false,
		},
		{
			path:     "/api/v2/derp",
			expected: false,
		},
		{
			path:     "//",
			expected: false,
		},
	}
	for _, tc := range testcases {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expected, isDERPPath(tc.path))
		})
	}
}
func TestEscapePostgresURLUserInfo(t *testing.T) {
	t.Parallel()
	testcases := []struct {
		input  string
		output string
		err    error
	}{
		{
			input:  "postgres://coder:coder@localhost:5432/coder",
			output: "postgres://coder:coder@localhost:5432/coder",
			err:    nil,
		},
		{
			input:  "postgres://coder:co{der@localhost:5432/coder",
			output: "postgres://coder:co%7Bder@localhost:5432/coder",
			err:    nil,
		},

		{
			input:  "postgres://coder:co:der@localhost:5432/coder",
			output: "postgres://coder:co:der@localhost:5432/coder",

			err:    nil,
		},
		{
			input:  "postgres://coder:co der@localhost:5432/coder",
			output: "postgres://coder:co%20der@localhost:5432/coder",
			err:    nil,
		},
		{
			input:  "postgres://local host:5432/coder",
			output: "",
			err:    errors.New("parse postgres url: parse \"postgres://local host:5432/coder\": invalid character \" \" in host name"),
		},
		{
			input:  "postgres://coder:co?der@localhost:5432/coder",
			output: "postgres://coder:co%3Fder@localhost:5432/coder",
			err:    nil,
		},
		{
			input:  "postgres://coder:co#der@localhost:5432/coder",
			output: "postgres://coder:co%23der@localhost:5432/coder",
			err:    nil,
		},
	}
	for _, tc := range testcases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			o, err := escapePostgresURLUserInfo(tc.input)
			assert.Equal(t, tc.output, o)
			if tc.err != nil {
				require.Error(t, err)
				require.EqualValues(t, tc.err.Error(), err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
