package sessionstore

import (
	"encoding/json"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCredentialValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		url      *url.URL
		token    string
		wantErr  bool
		errMsg   string
		wantCred *credential
	}{
		{
			name: "ValidHTTPURL",
			url: &url.URL{
				Scheme: "http",
				Host:   "coder.example.com",
			},
			token:   "my-secret-token",
			wantErr: false,
			wantCred: &credential{
				CoderURL: "coder.example.com",
				APIToken: "my-secret-token",
			},
		},
		{
			name: "ValidHTTPSURL",
			url: &url.URL{
				Scheme: "https",
				Host:   "coder.example.com",
			},
			token:   "my-secret-token",
			wantErr: false,
			wantCred: &credential{
				CoderURL: "coder.example.com",
				APIToken: "my-secret-token",
			},
		},
		{
			name: "URLWithPort",
			url: &url.URL{
				Scheme: "https",
				Host:   "coder.example.com:8080",
			},
			token:   "token-123",
			wantErr: false,
			wantCred: &credential{
				CoderURL: "coder.example.com:8080",
				APIToken: "token-123",
			},
		},
		{
			name: "EmptyToken",
			url: &url.URL{
				Scheme: "https",
				Host:   "coder.example.com",
			},
			token:   "",
			wantErr: false,
			wantCred: &credential{
				CoderURL: "coder.example.com",
				APIToken: "",
			},
		},
		{
			name: "UppercaseHostNormalized",
			url: &url.URL{
				Scheme: "https",
				Host:   "CODER.EXAMPLE.COM",
			},
			token:   "token-abc",
			wantErr: false,
			wantCred: &credential{
				CoderURL: "coder.example.com",
				APIToken: "token-abc",
			},
		},
		{
			name: "HostWithWhitespace",
			url: &url.URL{
				Scheme: "https",
				Host:   "  coder.example.com  ",
			},
			token:   "token-xyz",
			wantErr: false,
			wantCred: &credential{
				CoderURL: "coder.example.com",
				APIToken: "token-xyz",
			},
		},
		{
			name:    "NilURL",
			url:     nil,
			token:   "token-123",
			wantErr: true,
			errMsg:  "nil URL for credential value",
		},
		{
			name: "EmptyHost",
			url: &url.URL{
				Scheme: "https",
				Host:   "",
			},
			token:   "token-123",
			wantErr: true,
			errMsg:  "empty host for credential value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := generateCredential(tt.url, tt.token)

			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errMsg)
				require.Empty(t, result)
				return
			}

			require.NoError(t, err)
			require.NotEmpty(t, result)

			// Verify the result is valid JSON
			var gotCred credential
			err = json.Unmarshal([]byte(result), &gotCred)
			require.NoError(t, err, "result should be valid JSON")

			// Verify the unmarshaled credential matches expected values
			require.Equal(t, tt.wantCred.CoderURL, gotCred.CoderURL)
			require.Equal(t, tt.wantCred.APIToken, gotCred.APIToken)

			// Verify the JSON structure has the expected fields
			var jsonMap map[string]string
			err = json.Unmarshal([]byte(result), &jsonMap)
			require.NoError(t, err)
			require.Contains(t, jsonMap, "coder_url")
			require.Contains(t, jsonMap, "api_token")
			require.Equal(t, tt.wantCred.CoderURL, jsonMap["coder_url"])
			require.Equal(t, tt.wantCred.APIToken, jsonMap["api_token"])
		})
	}
}

func TestCredentialValue_RoundTrip(t *testing.T) {
	t.Parallel()

	u := &url.URL{
		Scheme: "https",
		Host:   "coder.example.com:8080",
	}
	token := "test-token-123"

	result, err := generateCredential(u, token)
	require.NoError(t, err)

	// Parse the JSON to verify structure
	var cred credential
	err = json.Unmarshal([]byte(result), &cred)
	require.NoError(t, err)

	// Verify fields are correct
	require.Equal(t, "coder.example.com:8080", cred.CoderURL)
	require.Equal(t, "test-token-123", cred.APIToken)

	// Verify we can re-marshal and get consistent results
	remarshaled, err := json.Marshal(cred)
	require.NoError(t, err)

	var cred2 credential
	err = json.Unmarshal(remarshaled, &cred2)
	require.NoError(t, err)
	require.Equal(t, cred, cred2)
}
