package exitnode_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/enterprise/exitnode"
)

func TestNormalizeHost(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ input, want string }{
		{"example.com:8443", "example.com"},
		{"[::1]:443", "::1"},
		{"::1", "::1"},
		{"  foo.bar ", "foo.bar"},
		{"", ""},
	} {
		require.Equal(t, tt.want, exitnode.NormalizeHost(tt.input), tt.input)
	}
}
