package cliui

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func Test_maybeSortList(t *testing.T) {
	t.Parallel()

	type structID struct {
		ID uuid.UUID
	}
	type structName struct {
		Name string
	}
	type structBoth struct {
		ID   uuid.UUID
		Name string
	}
	type structOther struct {
		Other string
	}
	type structBadType struct {
		ID int
	}

	cases := []struct {
		name      string
		in        any // must be a slice
		wantOrder []int
	}{
		{
			name:      "EmptyAny",
			in:        []any{},
			wantOrder: []int{},
		},
		{
			name:      "EmptyListStruct",
			in:        []structID{},
			wantOrder: []int{},
		},
		{
			name: "OtherStructs",
			in: []structOther{
				{Other: "foo"},
				{Other: "bar"},
			},
			// Does not sort because the struct does not have an ID or Name
			// field.
			wantOrder: []int{0, 1},
		},
		{
			name: "StructIDs",
			in: []structID{
				{ID: uuid.MustParse("31b92cb7-45c0-4dd6-8cb3-ce9c86568ebb")},
				{ID: uuid.MustParse("9ff6b55f-09ea-452e-aabb-e85589dd4c37")},
				{ID: uuid.MustParse("fc409006-f160-42cd-ade8-eadce65c42e4")},
				{ID: uuid.MustParse("4c2a1f75-9dd9-4687-bda1-271dec393399")},
			},
			wantOrder: []int{0, 3, 1, 2},
		},
		{
			name: "StructNames",
			in: []structName{
				{Name: "foo"},
				{Name: "bar"},
				{Name: "baz"},
				{Name: "qux"},
			},
			wantOrder: []int{1, 2, 0, 3},
		},
		{
			name: "Both",
			in: []structBoth{
				{
					ID:   uuid.MustParse("31b92cb7-45c0-4dd6-8cb3-ce9c86568ebb"),
					Name: "foo",
				},
				{
					ID:   uuid.MustParse("9ff6b55f-09ea-452e-aabb-e85589dd4c37"),
					Name: "bar",
				},
				{
					ID:   uuid.MustParse("fc409006-f160-42cd-ade8-eadce65c42e4"),
					Name: "baz",
				},
				{
					ID:   uuid.MustParse("4c2a1f75-9dd9-4687-bda1-271dec393399"),
					Name: "qux",
				},
			},
			// Only sorts by ID.
			wantOrder: []int{0, 3, 1, 2},
		},
		{
			name: "Pointers",
			in: []*structID{
				{ID: uuid.MustParse("9ff6b55f-09ea-452e-aabb-e85589dd4c37")},
				{ID: uuid.MustParse("fc409006-f160-42cd-ade8-eadce65c42e4")},
				{ID: uuid.MustParse("4c2a1f75-9dd9-4687-bda1-271dec393399")},
				{ID: uuid.MustParse("31b92cb7-45c0-4dd6-8cb3-ce9c86568ebb")},
			},
			wantOrder: []int{3, 2, 0, 1},
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			in := reflect.Indirect(reflect.ValueOf(c.in))
			if in.Kind() != reflect.Slice {
				t.Fatalf("input must be a slice")
			}

			outRaw := maybeSortList(c.in)

			out := reflect.Indirect(reflect.ValueOf(outRaw))
			require.Equal(t, in.Len(), out.Len())

			inSorted := make([]any, in.Len())
			for i, wantIdx := range c.wantOrder {
				inSorted[i] = in.Index(wantIdx).Interface()
			}

			outInterface := make([]any, out.Len())
			for i := 0; i < out.Len(); i++ {
				outInterface[i] = out.Index(i).Interface()
			}

			require.Equal(t, inSorted, outInterface)
		})
	}

	t.Run("NotSlice", func(t *testing.T) {
		t.Parallel()

		out := maybeSortList("foo")
		require.Equal(t, "foo", out)
	})

	t.Run("PanicBadSortType", func(t *testing.T) {
		t.Parallel()

		require.Panics(t, func() {
			maybeSortList([]structBadType{
				{ID: 1},
				{ID: 2},
			})
		})
	})
}
