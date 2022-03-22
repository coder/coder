package examples_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/examples"
)

func TestTemplate(t *testing.T) {
	t.Parallel()
	list, err := examples.List()
	require.NoError(t, err)
	require.Greater(t, len(list), 0)

	_, err = examples.Archive(list[0].ID)
	require.NoError(t, err)
}
