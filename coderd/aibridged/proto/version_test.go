package proto_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/apiversion"
	"github.com/coder/coder/v2/coderd/aibridged/proto"
)

func TestModelAuthorizationVersionCompatibility(t *testing.T) {
	t.Parallel()
	// Old Gateways remain supported, but model-aware Gateways must upgrade
	// coderd first so optional authorization fields cannot be ignored.
	require.NoError(t, proto.CurrentVersion.Validate("1.3"))
	require.NoError(t, proto.CurrentVersion.Validate("1.4"))
	require.Error(t, apiversion.New(1, 3).Validate(proto.CurrentVersion.String()))
}
