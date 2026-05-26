package coderd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNATSRouteAddressFromRelayAddress(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{
			name: "HTTPS",
			in:   "https://replica.example.com:8443",
			want: "nats://replica.example.com:6222",
			ok:   true,
		},
		{
			name: "HTTP",
			in:   "http://replica.example.com",
			want: "nats://replica.example.com:6222",
			ok:   true,
		},
		{
			name: "IPv6",
			in:   "https://[2001:db8::1]:8443",
			want: "nats://[2001:db8::1]:6222",
			ok:   true,
		},
		{
			name: "Empty",
			in:   "",
			ok:   false,
		},
		{
			name: "Garbage",
			in:   "not a url",
			ok:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := natsRouteAddressFromRelayAddress(tc.in)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.want, got)
		})
	}
}
