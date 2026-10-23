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

// fakeKeyring is an in-memory keyringProvider so that the scheme scoping rules can
// be exercised without depending on an operating system keyring.
type fakeKeyring struct {
	data map[string]string
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

const (
	testServiceName = "coder-v2-credentials-test"
	testHost        = "coder.example.com"
	testToken       = "test-token-12345"
)

// newSeededKeyring returns a Keyring holding one entry under key whose
// coder_url is storedURL.
func newSeededKeyring(t *testing.T, key, storedURL string) (Keyring, *fakeKeyring) {
	t.Helper()
	raw, err := json.Marshal(credentialsMap{key: {CoderURL: storedURL, APIToken: testToken}})
	require.NoError(t, err)
	provider := &fakeKeyring{data: map[string]string{testServiceName: string(raw)}}
	return Keyring{provider: provider, serviceName: testServiceName}, provider
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
		storedURL string
		delete    bool
		url       string
		wantErr   error
	}{
		{name: "ReadSameScheme", storedKey: testHost, storedURL: httpsURL, url: httpsURL},
		{name: "ReadHTTPEntryOverHTTP", storedKey: httpURL, storedURL: httpURL, url: httpURL},
		{name: "ReadHTTPSEntryOverHTTP", storedKey: testHost, storedURL: httpsURL, url: httpURL, wantErr: os.ErrNotExist},
		{name: "ReadHTTPEntryOverHTTPS", storedKey: httpURL, storedURL: httpURL, url: httpsURL, wantErr: os.ErrNotExist},
		{name: "ReadLegacyEntryOverHTTPS", storedKey: testHost, storedURL: testHost, url: httpsURL},
		{name: "ReadLegacyEntryOverHTTP", storedKey: testHost, storedURL: testHost, url: httpURL, wantErr: os.ErrNotExist},
		{name: "DeleteSameScheme", storedKey: testHost, storedURL: httpsURL, delete: true, url: httpsURL},
		{name: "DeleteOtherScheme", storedKey: testHost, storedURL: httpsURL, delete: true, url: httpURL, wantErr: os.ErrNotExist},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			keyring, provider := newSeededKeyring(t, tt.storedKey, tt.storedURL)
			before := provider.data[testServiceName]
			u, err := url.Parse(tt.url)
			require.NoError(t, err)

			if tt.delete {
				err = keyring.Delete(u)
				if tt.wantErr != nil {
					require.ErrorIs(t, err, tt.wantErr)
					require.Equal(t, before, provider.data[testServiceName], "stored entry must be left intact")
					return
				}
				require.NoError(t, err)
				require.NotContains(t, provider.data, testServiceName)
				return
			}

			got, err := keyring.Read(u)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testToken, got)
		})
	}
}

func TestKeyringHTTPAndHTTPSCoexist(t *testing.T) {
	t.Parallel()

	keyring, provider := newSeededKeyring(t, testHost, testHost)
	httpURL, err := url.Parse("http://" + testHost)
	require.NoError(t, err)
	httpsURL, err := url.Parse("https://" + testHost)
	require.NoError(t, err)

	const httpToken = "http-token"
	require.NoError(t, keyring.Write(httpURL, httpToken))

	creds, err := parseCredentialsJSON([]byte(provider.data[testServiceName]))
	require.NoError(t, err)
	require.Len(t, creds, 2)
	require.Equal(t, "http://"+testHost, creds["http://"+testHost].CoderURL)

	got, err := keyring.Read(httpURL)
	require.NoError(t, err)
	require.Equal(t, httpToken, got)
	got, err = keyring.Read(httpsURL)
	require.NoError(t, err)
	require.Equal(t, testToken, got)
}
