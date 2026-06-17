package cli

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestParseChatID(t *testing.T) {
	t.Parallel()

	t.Run("EmptyIsNil", func(t *testing.T) {
		t.Parallel()
		got, err := parseChatID("")
		require.NoError(t, err)
		require.Equal(t, uuid.Nil, got)
	})

	t.Run("ValidUUID", func(t *testing.T) {
		t.Parallel()
		want := uuid.MustParse("11111111-1111-4111-8111-111111111111")
		got, err := parseChatID(want.String())
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("InvalidErrors", func(t *testing.T) {
		t.Parallel()
		_, err := parseChatID("not-a-uuid")
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid chat ID")
	})
}
