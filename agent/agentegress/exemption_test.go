package agentegress_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentegress"
)

func TestParseExemption(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		value string
		want  agentegress.Exemption
	}{
		{
			name:  "TCP hostname",
			value: "tcp/coder.example.com:443",
			want:  agentegress.Exemption{Proto: "tcp", Host: "coder.example.com", Port: 443},
		},
		{
			name:  "UDP IPv6",
			value: "udp/[2001:db8::1]:41641",
			want:  agentegress.Exemption{Proto: "udp", Host: "2001:db8::1", Port: 41641},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := agentegress.ParseExemption(tc.value)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}

	for _, value := range []string{
		"coder.example.com:443",
		"tcp/coder.example.com",
		"icmp/coder.example.com:443",
		"tcp/coder.example.com:0",
		"tcp/:443",
	} {
		t.Run("Invalid/"+value, func(t *testing.T) {
			t.Parallel()
			_, err := agentegress.ParseExemption(value)
			require.Error(t, err)
		})
	}
}
