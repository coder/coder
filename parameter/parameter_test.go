package parameter_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/parameter"
)

func TestParse(t *testing.T) {
	t.Parallel()
	t.Run("ParseEnvironment", func(t *testing.T) {
		t.Parallel()
		uri, err := parameter.Parse("env://WOW")
		require.NoError(t, err)
		require.Equal(t, uri.Scheme, parameter.SchemeEnvironment)
		require.Equal(t, uri.Value, "WOW")
	})

	t.Run("ParseVariable", func(t *testing.T) {
		t.Parallel()
		uri, err := parameter.Parse("var://ok")
		require.NoError(t, err)
		require.Equal(t, uri.Scheme, parameter.SchemeVariable)
		require.Equal(t, uri.Value, "ok")
	})

	t.Run("Unrecognized", func(t *testing.T) {
		t.Parallel()
		_, err := parameter.Parse("tomato://ok")
		require.Error(t, err)
	})

	t.Run("String", func(t *testing.T) {
		t.Parallel()
		uri, err := parameter.Parse("var://test")
		require.NoError(t, err)
		require.Equal(t, "var://test", uri.String())
	})
}
