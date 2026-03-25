package xjson_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/xjson"
)

func TestParseUUIDList(t *testing.T) {
	t.Parallel()

	t.Run("EmptyString", func(t *testing.T) {
		t.Parallel()
		ids, err := xjson.ParseUUIDList("")
		require.NoError(t, err)
		require.NotNil(t, ids)
		require.Empty(t, ids)
	})

	t.Run("JSONNull", func(t *testing.T) {
		t.Parallel()
		ids, err := xjson.ParseUUIDList("null")
		require.NoError(t, err)
		require.NotNil(t, ids)
		require.Empty(t, ids)
	})

	t.Run("ValidUUIDs", func(t *testing.T) {
		t.Parallel()
		a := uuid.MustParse("c7c6686d-a93c-4df2-bef9-5f837e9a33d5")
		b := uuid.MustParse("8f3b3e0b-2c3f-46a5-a365-fd5b62bd8818")
		ids, err := xjson.ParseUUIDList(`["c7c6686d-a93c-4df2-bef9-5f837e9a33d5","8f3b3e0b-2c3f-46a5-a365-fd5b62bd8818"]`)
		require.NoError(t, err)
		require.Equal(t, []uuid.UUID{a, b}, ids)
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		t.Parallel()
		_, err := xjson.ParseUUIDList("not json at all")
		require.Error(t, err)
		require.Contains(t, err.Error(), "unmarshal uuid list")
	})

	t.Run("InvalidUUID", func(t *testing.T) {
		t.Parallel()
		_, err := xjson.ParseUUIDList(`["not-a-uuid"]`)
		require.Error(t, err)
		require.Contains(t, err.Error(), "parse uuid")
	})
}
