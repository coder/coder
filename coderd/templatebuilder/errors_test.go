package templatebuilder_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/templatebuilder"
)

func TestClassifyProvisionerError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		jobError string
		logs     []string
		contains string
		exact    bool
	}{
		{
			name:     "DNSFailure",
			jobError: "init failed",
			logs:     []string{"Error: Failed to query available provider packages", "dial tcp: lookup registry.terraform.io: no such host"},
			contains: "unreachable from your provisioner",
		},
		{
			name:     "ConnectionRefused",
			jobError: "terraform init: connection refused",
			logs:     nil,
			contains: "unreachable from your provisioner",
		},
		{
			name:     "IOTimeout",
			jobError: "context deadline exceeded",
			logs:     []string{"dial tcp 1.2.3.4:443: i/o timeout"},
			contains: "unreachable from your provisioner",
		},
		{
			name:     "TLSTimeout",
			jobError: "init error",
			logs:     []string{"net/http: TLS handshake timeout"},
			contains: "unreachable from your provisioner",
		},
		{
			name:     "UnknownError",
			jobError: "Error: Unsupported block type",
			logs:     []string{"on main.tf line 5"},
			contains: "Unsupported block type",
			exact:    true,
		},
		{
			name:     "EmptyErrorPassthrough",
			jobError: "",
			logs:     nil,
			contains: "",
			exact:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := templatebuilder.ClassifyProvisionerError(tc.jobError, tc.logs)
			if tc.exact {
				require.Equal(t, tc.jobError, result)
			} else {
				require.Contains(t, result, tc.contains)
			}
		})
	}
}
