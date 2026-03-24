package coderd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPickTokenEndpointAuthMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		supported []string
		want      string
	}{
		{
			name:      "PreferClientSecretPostWhenSupported",
			supported: []string{"client_secret_post", "client_secret_basic", "none"},
			want:      "client_secret_post",
		},
		{
			name:      "FallBackToClientSecretBasic",
			supported: []string{"client_secret_basic", "none"},
			want:      "client_secret_basic",
		},
		{
			name:      "FallBackToNoneWhenOnlyOption",
			supported: []string{"none"},
			want:      "none",
		},
		{
			name:      "DefaultToClientSecretPostWhenEmpty",
			supported: nil,
			want:      "client_secret_post",
		},
		{
			name:      "DefaultToClientSecretPostWhenEmptySlice",
			supported: []string{},
			want:      "client_secret_post",
		},
		{
			name:      "DefaultToClientSecretPostForUnknownMethods",
			supported: []string{"private_key_jwt", "tls_client_auth"},
			want:      "client_secret_post",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := pickTokenEndpointAuthMethod(tt.supported)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCallbackURLIsLoopback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"Localhost", "http://localhost:3000/callback", true},
		{"IPv4Loopback", "http://127.0.0.1:8080/callback", true},
		{"IPv6Loopback", "http://[::1]:8080/callback", true},
		{"PublicHTTPS", "https://coder.example.com/callback", false},
		{"PublicHTTP", "http://coder.example.com/callback", false},
		{"PrivateIP", "http://192.168.1.1:3000/callback", false},
		{"Empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := callbackURLIsLoopback(tt.url)
			assert.Equal(t, tt.want, got)
		})
	}
}
