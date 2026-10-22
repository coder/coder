package sessionstore

import (
	"encoding/json"
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

// newSeededKeyring returns a Keyring holding one entry for testHost whose
// coder_url is storedURL.
func newSeededKeyring(t *testing.T, storedURL string) (Keyring, *fakeKeyring) {
	t.Helper()
	raw, err := json.Marshal(credentialsMap{testHost: {CoderURL: storedURL, APIToken: testToken}})
	require.NoError(t, err)
	provider := &fakeKeyring{data: map[string]string{testServiceName: string(raw)}}
	return Keyring{provider: provider, serviceName: testServiceName}, provider
}

func TestKeyringSchemeScoping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		storedURL string
		op        string
		url       string
		wantErr   error
	}{
		{name: "ReadSameScheme", storedURL: "https://coder.example.com", op: "read", url: "https://coder.example.com"},
		{name: "ReadHTTPSEntryOverHTTP", storedURL: "https://coder.example.com", op: "read", url: "http://coder.example.com", wantErr: errSchemeMismatch},
		{name: "ReadHTTPEntryOverHTTPS", storedURL: "http://coder.example.com", op: "read", url: "https://coder.example.com", wantErr: errSchemeMismatch},
		{name: "ReadLegacyEntryOverHTTPS", storedURL: "coder.example.com", op: "read", url: "https://coder.example.com"},
		{name: "ReadLegacyEntryOverHTTP", storedURL: "coder.example.com", op: "read", url: "http://coder.example.com", wantErr: errSchemeMismatch},
		{name: "DeleteSameScheme", storedURL: "https://coder.example.com", op: "delete", url: "https://coder.example.com"},
		{name: "DeleteOtherScheme", storedURL: "https://coder.example.com", op: "delete", url: "http://coder.example.com", wantErr: errSchemeMismatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			keyring, provider := newSeededKeyring(t, tt.storedURL)
			before := provider.data[testServiceName]
			u, err := url.Parse(tt.url)
			require.NoError(t, err)

			var got string
			switch tt.op {
			case "read":
				got, err = keyring.Read(u)
			case "delete":
				err = keyring.Delete(u)
			default:
				t.Fatalf("unknown op %q", tt.op)
			}

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.NotContains(t, err.Error(), testToken, "error must not leak the token")
				require.Equal(t, before, provider.data[testServiceName], "stored entry must be left intact")
				return
			}
			require.NoError(t, err)
			if tt.op == "read" {
				require.Equal(t, testToken, got)
			} else {
				require.NotContains(t, provider.data, testServiceName)
			}
		})
	}
}

func TestKeyringWriteUpgradesLegacyEntry(t *testing.T) {
	t.Parallel()

	keyring, provider := newSeededKeyring(t, testHost)
	u, err := url.Parse("http://coder.example.com")
	require.NoError(t, err)

	require.NoError(t, keyring.Write(u, testToken))

	creds, err := parseCredentialsJSON([]byte(provider.data[testServiceName]))
	require.NoError(t, err)
	require.Equal(t, "http://coder.example.com", creds[testHost].CoderURL)

	got, err := keyring.Read(u)
	require.NoError(t, err)
	require.Equal(t, testToken, got)
}
