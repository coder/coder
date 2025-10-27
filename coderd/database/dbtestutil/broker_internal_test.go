package dbtestutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetTestPackageName(t *testing.T) {
	t.Parallel()
	packageName := getTestPackageName(t)
	require.Equal(t, "coderd/database/dbtestutil", packageName)
}
