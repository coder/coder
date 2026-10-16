package sessionstore

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		url     *url.URL
		want    string
		wantErr bool
	}{
		{
			name: "StandardHost",
			url:  &url.URL{Host: "coder.example.com"},
			want: "coder.example.com",
		},
		{
			name: "HostWithPort",
			url:  &url.URL{Host: "coder.example.com:8080"},
			want: "coder.example.com:8080",
		},
		{
			name: "UppercaseHost",
			url:  &url.URL{Host: "CODER.EXAMPLE.COM"},
			want: "coder.example.com",
		},
		{
			name: "HostWithWhitespace",
			url:  &url.URL{Host: "  coder.example.com  "},
			want: "coder.example.com",
		},
		{
			name:    "NilURL",
			url:     nil,
			want:    "",
			wantErr: true,
		},
		{
			name:    "EmptyHost",
			url:     &url.URL{Host: ""},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeHost(tt.url)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestParseCredentialsJSON(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		creds, err := parseCredentialsJSON(nil)
		require.NoError(t, err)
		require.NotNil(t, creds)
		require.Empty(t, creds)
	})

	t.Run("NewFormat", func(t *testing.T) {
		t.Parallel()
		jsonData := []byte(`{
			"coder1.example.com": {"coder_url": "coder1.example.com", "api_token": "token1"},
			"coder2.example.com": {"coder_url": "coder2.example.com", "api_token": "token2"}
		}`)
		creds, err := parseCredentialsJSON(jsonData)
		require.NoError(t, err)
		require.Len(t, creds, 2)
		require.Equal(t, "token1", creds["coder1.example.com"].APIToken)
		require.Equal(t, "token2", creds["coder2.example.com"].APIToken)
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		t.Parallel()
		jsonData := []byte(`{invalid json}`)
		_, err := parseCredentialsJSON(jsonData)
		require.Error(t, err)
	})
}

func TestCredentialsMap_RoundTrip(t *testing.T) {
	t.Parallel()

	creds := credentialsMap{
		"coder1.example.com": {
			CoderURL: "coder1.example.com",
			APIToken: "token1",
		},
		"coder2.example.com:8080": {
			CoderURL: "coder2.example.com:8080",
			APIToken: "token2",
		},
	}

	jsonData, err := json.Marshal(creds)
	require.NoError(t, err)

	parsed, err := parseCredentialsJSON(jsonData)
	require.NoError(t, err)

	require.Equal(t, creds, parsed)
}

func TestNormalizeOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "HTTPS",
			url:  "https://coder.example.com",
			want: "https://coder.example.com",
		},
		{
			name: "HTTPKeepsScheme",
			url:  "http://coder.example.com",
			want: "http://coder.example.com",
		},
		{
			name: "UppercaseIsLowered",
			url:  "HTTPS://CODER.EXAMPLE.COM",
			want: "https://coder.example.com",
		},
		{
			name: "NonDefaultPortIsKept",
			url:  "https://coder.example.com:8080",
			want: "https://coder.example.com:8080",
		},
		{
			name: "DefaultHTTPSPortIsDropped",
			url:  "https://coder.example.com:443",
			want: "https://coder.example.com",
		},
		{
			name: "DefaultHTTPPortIsDropped",
			url:  "http://coder.example.com:80",
			want: "http://coder.example.com",
		},
		{
			name: "HTTPSWithHTTPDefaultPortIsKept",
			url:  "https://coder.example.com:80",
			want: "https://coder.example.com:80",
		},
		{
			name: "PathIsIgnored",
			url:  "https://coder.example.com/some/path",
			want: "https://coder.example.com",
		},
		{
			name:    "NoScheme",
			url:     "//coder.example.com",
			wantErr: true,
		},
		{
			name:    "NoHost",
			url:     "https://",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			u, err := url.Parse(tt.url)
			require.NoError(t, err)
			got, err := normalizeOrigin(u)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeOrigin_NilURL(t *testing.T) {
	t.Parallel()

	_, err := normalizeOrigin(nil)
	require.Error(t, err)
}

func TestCredentialOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cred credential
		host string
		want string
	}{
		{
			name: "RecordedOriginIsUsed",
			cred: credential{CoderURL: "http://coder.example.com"},
			host: "coder.example.com",
			want: "http://coder.example.com",
		},
		{
			name: "RecordedOriginIsLowered",
			cred: credential{CoderURL: "HTTPS://Coder.Example.COM"},
			host: "coder.example.com",
			want: "https://coder.example.com",
		},
		{
			// Entries written by older CLI versions, and by other Coder
			// applications, record only the host.
			name: "LegacyHostOnlyAssumesHTTPS",
			cred: credential{CoderURL: "coder.example.com"},
			host: "coder.example.com",
			want: "https://coder.example.com",
		},
		{
			name: "EmptyAssumesHTTPSOfKeyHost",
			cred: credential{CoderURL: ""},
			host: "coder.example.com",
			want: "https://coder.example.com",
		},
		{
			name: "LegacyHostOnlyDropsDefaultHTTPSPort",
			cred: credential{CoderURL: "coder.example.com:443"},
			host: "coder.example.com:443",
			want: "https://coder.example.com",
		},
		{
			name: "LegacyHostOnlyKeepsNonDefaultPort",
			cred: credential{CoderURL: "coder.example.com:8080"},
			host: "coder.example.com:8080",
			want: "https://coder.example.com:8080",
		},
		{
			name: "RecordedOriginIgnoresTrailingSlash",
			cred: credential{CoderURL: "https://coder.example.com/"},
			host: "coder.example.com",
			want: "https://coder.example.com",
		},
		{
			name: "RecordedOriginDropsDefaultPort",
			cred: credential{CoderURL: "https://coder.example.com:443"},
			host: "coder.example.com",
			want: "https://coder.example.com",
		},
		{
			name: "UnparseableFallsBackToHTTPSOfKeyHost",
			cred: credential{CoderURL: "://not a url"},
			host: "coder.example.com",
			want: "https://coder.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, credentialOrigin(tt.cred, tt.host))
		})
	}
}

// fakeKeyring is an in-memory keyringProvider so that the origin scoping rules can
// be exercised without depending on an operating system keyring.
type fakeKeyring struct {
	data map[string]string
}

func newFakeKeyring() *fakeKeyring {
	return &fakeKeyring{data: make(map[string]string)}
}

func (f *fakeKeyring) Set(service, cred string) error {
	f.data[service] = cred
	return nil
}

func (f *fakeKeyring) Get(service string) ([]byte, error) {
	v, ok := f.data[service]
	if !ok {
		return nil, os.ErrNotExist
	}
	return []byte(v), nil
}

func (f *fakeKeyring) Delete(service string) error {
	if _, ok := f.data[service]; !ok {
		return os.ErrNotExist
	}
	delete(f.data, service)
	return nil
}

func TestKeyringOriginScoping(t *testing.T) {
	t.Parallel()

	const (
		serviceName = "coder-v2-credentials-test"
		httpsURL    = "https://coder.example.com"
		httpURL     = "http://coder.example.com"
		token       = "test-token-12345"
	)

	newKeyring := func() (Keyring, *fakeKeyring) {
		provider := newFakeKeyring()
		return Keyring{provider: provider, serviceName: serviceName}, provider
	}

	mustParse := func(t *testing.T, raw string) *url.URL {
		t.Helper()
		u, err := url.Parse(raw)
		require.NoError(t, err)
		return u
	}

	// seedLegacy writes an entry in the format used by older CLI versions and by
	// other Coder applications, where coder_url holds the bare host.
	seedLegacy := func(t *testing.T, provider *fakeKeyring, host string) {
		t.Helper()
		creds := credentialsMap{host: {CoderURL: host, APIToken: token}}
		raw, err := json.Marshal(creds)
		require.NoError(t, err)
		provider.data[serviceName] = string(raw)
	}

	t.Run("WriteRecordsFullOrigin", func(t *testing.T) {
		t.Parallel()
		keyring, provider := newKeyring()

		require.NoError(t, keyring.Write(mustParse(t, httpsURL), token))

		creds, err := parseCredentialsJSON([]byte(provider.data[serviceName]))
		require.NoError(t, err)
		// The map stays keyed by host so that other Coder applications keep
		// finding the entry.
		cred, ok := creds["coder.example.com"]
		require.True(t, ok, "entry should still be keyed by host")
		require.Equal(t, httpsURL, cred.CoderURL)
		require.Equal(t, token, cred.APIToken)
	})

	t.Run("ReadRefusesOtherScheme", func(t *testing.T) {
		t.Parallel()
		keyring, _ := newKeyring()

		require.NoError(t, keyring.Write(mustParse(t, httpsURL), token))

		got, err := keyring.Read(mustParse(t, httpURL))
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrOriginMismatch), "expected ErrOriginMismatch, got %v", err)
		require.Empty(t, got, "no token should be returned for a mismatched origin")
		require.NotContains(t, err.Error(), token, "error must not leak the token")
	})

	t.Run("ReadAllowsMatchingScheme", func(t *testing.T) {
		t.Parallel()
		keyring, _ := newKeyring()

		require.NoError(t, keyring.Write(mustParse(t, httpsURL), token))

		got, err := keyring.Read(mustParse(t, httpsURL))
		require.NoError(t, err)
		require.Equal(t, token, got)
	})

	t.Run("ReadRefusesOtherSchemeForHTTPEntry", func(t *testing.T) {
		t.Parallel()
		keyring, _ := newKeyring()

		require.NoError(t, keyring.Write(mustParse(t, httpURL), token))

		_, err := keyring.Read(mustParse(t, httpsURL))
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrOriginMismatch), "expected ErrOriginMismatch, got %v", err)
	})

	t.Run("DeleteRefusesOtherScheme", func(t *testing.T) {
		t.Parallel()
		keyring, _ := newKeyring()

		require.NoError(t, keyring.Write(mustParse(t, httpsURL), token))

		err := keyring.Delete(mustParse(t, httpURL))
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrOriginMismatch), "expected ErrOriginMismatch, got %v", err)

		// The https session must survive a logout aimed at the http URL.
		got, err := keyring.Read(mustParse(t, httpsURL))
		require.NoError(t, err)
		require.Equal(t, token, got)
	})

	t.Run("DeleteAllowsMatchingScheme", func(t *testing.T) {
		t.Parallel()
		keyring, _ := newKeyring()

		require.NoError(t, keyring.Write(mustParse(t, httpsURL), token))
		require.NoError(t, keyring.Delete(mustParse(t, httpsURL)))

		_, err := keyring.Read(mustParse(t, httpsURL))
		require.True(t, errors.Is(err, os.ErrNotExist), "expected os.ErrNotExist, got %v", err)
	})

	t.Run("WriteOverwritesOriginForSameHost", func(t *testing.T) {
		t.Parallel()
		keyring, _ := newKeyring()

		require.NoError(t, keyring.Write(mustParse(t, httpsURL), token))
		const httpToken = "http-token-67890"
		require.NoError(t, keyring.Write(mustParse(t, httpURL), httpToken))

		// Logging in to the http deployment replaces the entry, and the
		// recorded origin follows it, so no cross-scheme read is possible.
		got, err := keyring.Read(mustParse(t, httpURL))
		require.NoError(t, err)
		require.Equal(t, httpToken, got)

		_, err = keyring.Read(mustParse(t, httpsURL))
		require.True(t, errors.Is(err, ErrOriginMismatch), "expected ErrOriginMismatch, got %v", err)
	})

	t.Run("LegacyEntryReadableOverHTTPS", func(t *testing.T) {
		t.Parallel()
		keyring, provider := newKeyring()
		seedLegacy(t, provider, "coder.example.com")

		got, err := keyring.Read(mustParse(t, httpsURL))
		require.NoError(t, err, "existing https sessions must survive the upgrade")
		require.Equal(t, token, got)
	})

	t.Run("LegacyEntryRefusedOverHTTP", func(t *testing.T) {
		t.Parallel()
		keyring, provider := newKeyring()
		seedLegacy(t, provider, "coder.example.com")

		// This is the reported leak: a scheme-less entry must not hand its
		// token to a plaintext http client.
		_, err := keyring.Read(mustParse(t, httpURL))
		require.Error(t, err)
		require.True(t, errors.Is(err, ErrOriginMismatch), "expected ErrOriginMismatch, got %v", err)
	})

	t.Run("LegacyEntryUpgradedByWrite", func(t *testing.T) {
		t.Parallel()
		keyring, provider := newKeyring()
		seedLegacy(t, provider, "coder.example.com")

		require.NoError(t, keyring.Write(mustParse(t, httpURL), token))

		creds, err := parseCredentialsJSON([]byte(provider.data[serviceName]))
		require.NoError(t, err)
		require.Equal(t, httpURL, creds["coder.example.com"].CoderURL)

		got, err := keyring.Read(mustParse(t, httpURL))
		require.NoError(t, err)
		require.Equal(t, token, got)
	})

	t.Run("ReadMissingHostIsNotExist", func(t *testing.T) {
		t.Parallel()
		keyring, _ := newKeyring()

		require.NoError(t, keyring.Write(mustParse(t, httpsURL), token))

		_, err := keyring.Read(mustParse(t, "https://other.example.com"))
		require.True(t, errors.Is(err, os.ErrNotExist), "expected os.ErrNotExist, got %v", err)
	})

	t.Run("URLWithoutSchemeIsRejected", func(t *testing.T) {
		t.Parallel()
		keyring, _ := newKeyring()

		u := &url.URL{Host: "coder.example.com"}
		_, err := keyring.Read(u)
		require.Error(t, err)

		err = keyring.Write(u, token)
		require.Error(t, err)

		err = keyring.Delete(u)
		require.Error(t, err)
	})
}
