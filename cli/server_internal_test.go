package cli

import (
	"bytes"
	"context"
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/assert"

	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
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
		// Errors
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
			wantErr:      "tls ciphers cannot be specified when using minimum tls version 1.3",
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
