package coderdtest_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
)

func TestDeterministicUUIDGenerator(t *testing.T) {
	t.Parallel()

	ids := coderdtest.NewDeterministicUUIDGenerator()
	require.Equal(t, ids.ID("g1"), ids.ID("g1"))
	require.NotEqual(t, ids.ID("g1"), ids.ID("g2"))
}
