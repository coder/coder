package exitnode_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/enterprise/exitnode"
)

func TestNormalizeHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{"Example.COM", "example.com"},
		{"example.com.", "example.com"},
		{"example.com:8443", "example.com"},
		{"[::1]:443", "::1"},
		{"::1", "::1"},
		{"  foo.bar ", "foo.bar"},
		{"", ""},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, exitnode.NormalizeHost(tt.in), "input %q", tt.in)
	}
}
