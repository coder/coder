package clibase_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clibase"
)

func TestStrings(t *testing.T) {
	t.Parallel()
	t.Run("CSV", func(t *testing.T) {
		t.Parallel()
		var ss []string

		err := clibase.StringsOf(&ss).Set("a,b,c")
		require.NoError(t, err)

		require.Equal(t, []string{"a", "b", "c"}, ss)
	})
	t.Run("SpaceSeparated", func(t *testing.T) {
		t.Parallel()
		var ss []string

		err := clibase.StringsOf(&ss).Set("a b c")
		require.NoError(t, err)

		require.Equal(t, []string{"a", "b", "c"}, ss)
	})
}
