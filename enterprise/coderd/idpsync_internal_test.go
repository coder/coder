package coderd

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestApplyIDPSyncMappingDiff(t *testing.T) {
	t.Parallel()

	t.Run("with UUIDs", func(t *testing.T) {
		t.Parallel()

		id := []uuid.UUID{
			uuid.MustParse("00000000-b8bd-46bb-bb6c-6c2b2c0dd2ea"),
			uuid.MustParse("01000000-fbe8-464c-9429-fe01a03f3644"),
			uuid.MustParse("02000000-0926-407b-9998-39af62e3d0c5"),
			uuid.MustParse("03000000-92f6-4bfd-bba6-0f54667b131c"),
		}

		mapping := applyIDPSyncMappingDiff(map[string][]uuid.UUID{},
			[]codersdk.IDPSyncMapping[uuid.UUID]{
				{Given: "wibble", Gets: id[0]},
				{Given: "wibble", Gets: id[1]},
				{Given: "wobble", Gets: id[0]},
				{Given: "wobble", Gets: id[1]},
				{Given: "wobble", Gets: id[2]},
				{Given: "wobble", Gets: id[3]},
				{Given: "wooble", Gets: id[0]},
			},
			// Remove takes priority over Add, so `3` should not actually be added.
			[]codersdk.IDPSyncMapping[uuid.UUID]{
				{Given: "wobble", Gets: id[3]},
			},
		)

		expected := map[string][]uuid.UUID{
			"wibble": {id[0], id[1]},
			"wobble": {id[0], id[1], id[2]},
			"wooble": {id[0]},
		}

		require.Equal(t, expected, mapping)

		mapping = applyIDPSyncMappingDiff(mapping,
			[]codersdk.IDPSyncMapping[uuid.UUID]{
				{Given: "wibble", Gets: id[2]},
				{Given: "wobble", Gets: id[3]},
				{Given: "wooble", Gets: id[0]},
			},
			[]codersdk.IDPSyncMapping[uuid.UUID]{
				{Given: "wibble", Gets: id[0]},
				{Given: "wobble", Gets: id[1]},
			},
		)

		expected = map[string][]uuid.UUID{
			"wibble": {id[1], id[2]},
			"wobble": {id[0], id[2], id[3]},
			"wooble": {id[0]},
		}

		require.Equal(t, expected, mapping)
	})

	t.Run("with strings", func(t *testing.T) {
		t.Parallel()

		mapping := applyIDPSyncMappingDiff(map[string][]string{},
			[]codersdk.IDPSyncMapping[string]{
				{Given: "wibble", Gets: "group-00"},
				{Given: "wibble", Gets: "group-01"},
				{Given: "wobble", Gets: "group-00"},
				{Given: "wobble", Gets: "group-01"},
				{Given: "wobble", Gets: "group-02"},
				{Given: "wobble", Gets: "group-03"},
				{Given: "wooble", Gets: "group-00"},
			},
			// Remove takes priority over Add, so `3` should not actually be added.
			[]codersdk.IDPSyncMapping[string]{
				{Given: "wobble", Gets: "group-03"},
			},
		)

		expected := map[string][]string{
			"wibble": {"group-00", "group-01"},
			"wobble": {"group-00", "group-01", "group-02"},
			"wooble": {"group-00"},
		}

		require.Equal(t, expected, mapping)

		mapping = applyIDPSyncMappingDiff(mapping,
			[]codersdk.IDPSyncMapping[string]{
				{Given: "wibble", Gets: "group-02"},
				{Given: "wobble", Gets: "group-03"},
				{Given: "wooble", Gets: "group-00"},
			},
			[]codersdk.IDPSyncMapping[string]{
				{Given: "wibble", Gets: "group-00"},
				{Given: "wobble", Gets: "group-01"},
			},
		)

		expected = map[string][]string{
			"wibble": {"group-01", "group-02"},
			"wobble": {"group-00", "group-02", "group-03"},
			"wooble": {"group-00"},
		}

		require.Equal(t, expected, mapping)
	})
}
