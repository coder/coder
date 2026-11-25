package sessionstore

import (
	"encoding/json"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCredentialKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		url        *url.URL
		wantKey    string
		wantOrigin string
		wantErr    bool
	}{
		{name: "HTTPS", url: &url.URL{Scheme: "https", Host: "coder.example.com"}, wantKey: "coder.example.com", wantOrigin: "https://coder.example.com"},
		{name: "HTTPSDefaultPort", url: &url.URL{Scheme: "https", Host: "coder.example.com:443"}, wantKey: "coder.example.com", wantOrigin: "https://coder.example.com"},
		{name: "HTTPSOtherPort", url: &url.URL{Scheme: "https", Host: "coder.example.com:8443"}, wantKey: "coder.example.com:8443", wantOrigin: "https://coder.example.com:8443"},
		{name: "HTTPSIPv6DefaultPort", url: &url.URL{Scheme: "https", Host: "[::1]:443"}, wantKey: "[::1]", wantOrigin: "https://[::1]"},
		{name: "UppercaseHost", url: &url.URL{Scheme: "https", Host: "CODER.EXAMPLE.COM"}, wantKey: "coder.example.com", wantOrigin: "https://coder.example.com"},
		{name: "HostWithWhitespace", url: &url.URL{Scheme: "https", Host: "  coder.example.com  "}, wantKey: "coder.example.com", wantOrigin: "https://coder.example.com"},
		{name: "HTTP", url: &url.URL{Scheme: "http", Host: "coder.example.com"}, wantKey: "http://coder.example.com", wantOrigin: "http://coder.example.com"},
		{name: "HTTPDefaultPort", url: &url.URL{Scheme: "http", Host: "coder.example.com:80"}, wantKey: "http://coder.example.com", wantOrigin: "http://coder.example.com"},
		{name: "HTTPOtherPort", url: &url.URL{Scheme: "http", Host: "coder.example.com:8080"}, wantKey: "http://coder.example.com:8080", wantOrigin: "http://coder.example.com:8080"},
		{name: "HTTPSKeepsPort80", url: &url.URL{Scheme: "https", Host: "coder.example.com:80"}, wantKey: "coder.example.com:80", wantOrigin: "https://coder.example.com:80"},
		{name: "HTTPKeepsPort443", url: &url.URL{Scheme: "http", Host: "coder.example.com:443"}, wantKey: "http://coder.example.com:443", wantOrigin: "http://coder.example.com:443"},
		{name: "UnsupportedScheme", url: &url.URL{Scheme: "ftp", Host: "coder.example.com"}, wantErr: true},
		{name: "NilURL", url: nil, wantErr: true},
		{name: "EmptyHost", url: &url.URL{Scheme: "https"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			key, origin, err := credentialKey(tt.url)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantKey, key)
			require.Equal(t, tt.wantOrigin, origin)
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

// fakeKeyring is an in-memory keyringProvider keyed by service name.
type fakeKeyring map[string]string

func (f fakeKeyring) Set(service, cred string) error {
	f[service] = cred
	return nil
}

func (f fakeKeyring) Get(service string) ([]byte, error) {
	v, ok := f[service]
	if !ok {
		return nil, os.ErrNotExist
	}
	return []byte(v), nil
}

func (f fakeKeyring) Delete(service string) error {
	if _, ok := f[service]; !ok {
		return os.ErrNotExist
	}
	delete(f, service)
	return nil
}

const (
	testService = "coder-v2-credentials-test"
	testHost    = "coder.example.com"
	testToken   = "test-token-12345"
)

// newSeededKeyring returns a Keyring holding testToken under key.
func newSeededKeyring(t *testing.T, key string) (Keyring, fakeKeyring) {
	t.Helper()
	raw, err := json.Marshal(credentialsMap{key: {APIToken: testToken}})
	require.NoError(t, err)
	provider := fakeKeyring{testService: string(raw)}
	return Keyring{provider: provider, serviceName: testService}, provider
}

func TestKeyringSchemeScoping(t *testing.T) {
	t.Parallel()

	const (
		httpsURL = "https://" + testHost
		httpURL  = "http://" + testHost
	)

	tests := []struct {
		name      string
		storedKey string
		url       string
		delete    bool
		wantMiss  bool
	}{
		{name: "ReadHTTPS", storedKey: testHost, url: httpsURL},
		{name: "ReadHTTP", storedKey: httpURL, url: httpURL},
		{name: "ReadHTTPSDefaultPortAndPath", storedKey: testHost, url: httpsURL + ":443/"},
		{name: "ReadHTTPSEntryOverHTTP", storedKey: testHost, url: httpURL, wantMiss: true},
		{name: "ReadHTTPEntryOverHTTPS", storedKey: httpURL, url: httpsURL, wantMiss: true},
		{name: "DeleteSameScheme", storedKey: testHost, url: httpsURL, delete: true},
		{name: "DeleteOtherScheme", storedKey: testHost, url: httpURL, delete: true, wantMiss: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			keyring, provider := newSeededKeyring(t, tt.storedKey)
			u, err := url.Parse(tt.url)
			require.NoError(t, err)

			if tt.delete {
				before := provider[testService]
				err = keyring.Delete(u)
				if tt.wantMiss {
					require.ErrorIs(t, err, os.ErrNotExist)
					require.Equal(t, before, provider[testService], "stored entry must be left intact")
					return
				}
				require.NoError(t, err)
				require.NotContains(t, provider, testService)
				return
			}

			got, err := keyring.Read(u)
			if tt.wantMiss {
				require.ErrorIs(t, err, os.ErrNotExist)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testToken, got)
		})
	}
}

func TestKeyringHTTPAndHTTPSCoexist(t *testing.T) {
	t.Parallel()

	keyring, provider := newSeededKeyring(t, testHost)
	require.NoError(t, keyring.Write(&url.URL{Scheme: "http", Host: testHost}, "http-token"))

	creds, err := parseCredentialsJSON([]byte(provider[testService]))
	require.NoError(t, err)
	require.Equal(t, credentialsMap{
		testHost:             {APIToken: testToken},
		"http://" + testHost: {CoderURL: "http://" + testHost, APIToken: "http-token"},
	}, creds)
}
