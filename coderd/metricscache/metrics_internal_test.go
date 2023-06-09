package metricscache

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClosest(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Name     string
		Keys     []int
		Input    int
		Expected int
		NotFound bool
	}{
		{
			Name:     "Empty",
			Input:    10,
			NotFound: true,
		},
		{
			Name:     "Equal",
			Keys:     []int{1, 2, 3, 4, 5, 6, 10, 12, 15},
			Input:    10,
			Expected: 10,
		},
		{
			Name:     "ZeroOnly",
			Keys:     []int{0},
			Input:    10,
			Expected: 0,
		},
		{
			Name:     "NegativeOnly",
			Keys:     []int{-10, -5},
			Input:    10,
			Expected: -5,
		},
		{
			Name:     "CloseBothSides",
			Keys:     []int{-10, -5, 0, 5, 8, 12},
			Input:    10,
			Expected: 8,
		},
		{
			Name:     "CloseNoZero",
			Keys:     []int{-10, -5, 5, 8, 12},
			Input:    0,
			Expected: -5,
		},
		{
			Name:     "CloseLeft",
			Keys:     []int{-10, -5, 0, 5, 8, 12},
			Input:    20,
			Expected: 12,
		},
		{
			Name:     "CloseRight",
			Keys:     []int{-10, -5, 0, 5, 8, 12},
			Input:    -20,
			Expected: -10,
		},
		{
			Name:     "ChooseZero",
			Keys:     []int{-10, -5, 0, 5, 8, 12},
			Input:    2,
			Expected: 0,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			m := make(map[int]int)
			for _, k := range tc.Keys {
				m[k] = k
			}

			found, _, ok := closest(m, tc.Input)
			if tc.NotFound {
				require.False(t, ok, "should not be found")
			} else {
				require.True(t, ok)
				require.Equal(t, tc.Expected, found, "closest")
			}
		})
	}
}
