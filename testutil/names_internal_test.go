package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIncSuffix(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		s      string
		num    int64
		maxLen int
		want   string
	}{
		{
			s:      "foo",
			num:    1,
			maxLen: 4,
			want:   "foo1",
		},
		{
			s:      "foo",
			num:    42,
			maxLen: 3,
			want:   "f42",
		},
		{
			s:      "foo",
			num:    3,
			maxLen: 2,
			want:   "f3",
		},
		{
			s:      "foo",
			num:    4,
			maxLen: 1,
			want:   "4",
		},
		{
			s:      "foo",
			num:    0,
			maxLen: 0,
			want:   "",
		},
	} {
		actual := incSuffix(tt.s, tt.num, tt.maxLen)
		assert.Equal(t, tt.want, actual)
	}
}
