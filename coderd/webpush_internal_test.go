package coderd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateWebpushEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		endpoint  string
		wantErr   bool
		errSubstr string
	}{
		{
			name:     "valid https endpoint",
			endpoint: "https://fcm.googleapis.com/fcm/send/abc123",
			wantErr:  false,
		},
		{
			name:     "valid https endpoint with port",
			endpoint: "https://push.example.com:8443/subscription",
			wantErr:  false,
		},
		{
			name:      "relative URL",
			endpoint:  "/push/subscription",
			wantErr:   true,
			errSubstr: "absolute URL",
		},
		{
			name:      "http scheme rejected",
			endpoint:  "http://push.example.com/subscription",
			wantErr:   true,
			errSubstr: "scheme must be https",
		},
		{
			name:      "custom scheme rejected",
			endpoint:  "ws://push.example.com/subscription",
			wantErr:   true,
			errSubstr: "scheme must be https",
		},
		{
			name:      "empty host",
			endpoint:  "https:///path",
			wantErr:   true,
			errSubstr: "host is required",
		},
		{
			name:      "userinfo rejected",
			endpoint:  "https://user:pass@push.example.com/subscription",
			wantErr:   true,
			errSubstr: "must not include userinfo",
		},
		{
			name:      "localhost rejected",
			endpoint:  "https://localhost/subscription",
			wantErr:   true,
			errSubstr: "must not be localhost",
		},
		{
			name:      "subdomain of localhost rejected",
			endpoint:  "https://foo.localhost/subscription",
			wantErr:   true,
			errSubstr: "must not be localhost",
		},
		{
			name:      "loopback IPv4 rejected",
			endpoint:  "https://127.0.0.1/subscription",
			wantErr:   true,
			errSubstr: "must not be private",
		},
		{
			name:      "private 10.x rejected",
			endpoint:  "https://10.0.0.1/subscription",
			wantErr:   true,
			errSubstr: "must not be private",
		},
		{
			name:      "private 192.168.x rejected",
			endpoint:  "https://192.168.1.1/subscription",
			wantErr:   true,
			errSubstr: "must not be private",
		},
		{
			name:      "private 172.16.x rejected",
			endpoint:  "https://172.16.0.1/subscription",
			wantErr:   true,
			errSubstr: "must not be private",
		},
		{
			name:      "link-local IPv4 rejected",
			endpoint:  "https://169.254.1.1/subscription",
			wantErr:   true,
			errSubstr: "must not be private",
		},
		{
			name:      "unspecified IPv4 rejected",
			endpoint:  "https://0.0.0.0/subscription",
			wantErr:   true,
			errSubstr: "must not be private",
		},
		{
			name:      "loopback IPv6 rejected",
			endpoint:  "https://[::1]/subscription",
			wantErr:   true,
			errSubstr: "must not be private",
		},
		{
			name:      "unspecified IPv6 rejected",
			endpoint:  "https://[::]/subscription",
			wantErr:   true,
			errSubstr: "must not be private",
		},
		{
			name:      "link-local IPv6 rejected",
			endpoint:  "https://[fe80::1]/subscription",
			wantErr:   true,
			errSubstr: "must not be private",
		},
		{
			name:      "multicast IPv4 rejected",
			endpoint:  "https://224.0.0.1/subscription",
			wantErr:   true,
			errSubstr: "must not be private",
		},
		{
			name:     "public IPv4 allowed",
			endpoint: "https://203.0.113.1/subscription",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateWebpushEndpoint(tt.endpoint)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr,
					"error should mention %q", tt.errSubstr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
