package sessionstore

import (
	"encoding/json"
	"net/url"
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
