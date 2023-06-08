package strings_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/util/strings"
)

func TestJoinWithConjunction(t *testing.T) {
	t.Parallel()
	require.Equal(t, "foo", strings.JoinWithConjunction([]string{"foo"}))
	require.Equal(t, "foo and bar", strings.JoinWithConjunction([]string{"foo", "bar"}))
	require.Equal(t, "foo, bar and baz", strings.JoinWithConjunction([]string{"foo", "bar", "baz"}))
}
