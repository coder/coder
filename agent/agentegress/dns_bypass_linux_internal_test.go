//go:build linux

package agentegress

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolverDialerUsesBypassControl(t *testing.T) {
	t.Parallel()

	dialer := resolverDialer()
	require.NotNil(t, dialer.Control)
}
