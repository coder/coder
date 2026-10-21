package webpush

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSSRFSafeHTTPClientHasTimeout(t *testing.T) {
	t.Parallel()
	require.Equal(t, pushRequestTimeout, newSSRFSafeHTTPClient().Timeout)
}
