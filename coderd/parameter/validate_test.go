package parameter_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/parameter"
)

func TestValidate(t *testing.T) {
	t.Parallel()
	t.Run("Contains", func(t *testing.T) {
		t.Parallel()
		values, valid, err := parameter.Contains(`contains(["us-east1-a", "us-central1-a"], var.region)`)
		require.NoError(t, err)
		require.True(t, valid)
		require.Len(t, values, 2)
	})
}
