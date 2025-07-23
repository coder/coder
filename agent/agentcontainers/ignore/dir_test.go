package ignore_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontainers/ignore"
)

func TestFilePathToParts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path     string
		expected []string
	}{
		{"/foo/bar/baz", []string{"foo", "bar", "baz"}},
		{"foo/bar/baz", []string{"foo", "bar", "baz"}},
		{"/foo", []string{"foo"}},
		{"foo", []string{"foo"}},
		{"/", []string{}},
		{"", []string{}},
		{"./foo/bar", []string{"foo", "bar"}},
		{"../foo/bar", []string{"..", "foo", "bar"}},
		{"/foo//bar///baz", []string{"foo", "bar", "baz"}},
		{"foo/bar/", []string{"foo", "bar"}},
		{"/foo/bar/", []string{"foo", "bar"}},
		{"foo/../bar", []string{"bar"}},
		{"./foo/../bar", []string{"bar"}},
		{"/foo/../bar", []string{"bar"}},
		{"foo/./bar", []string{"foo", "bar"}},
		{"/foo/./bar", []string{"foo", "bar"}},
		{"a/b/c/d/e", []string{"a", "b", "c", "d", "e"}},
		{"/a/b/c/d/e", []string{"a", "b", "c", "d", "e"}},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("`%s`", tt.path), func(t *testing.T) {
			t.Parallel()

			parts := ignore.FilePathToParts(tt.path)
			require.Equal(t, tt.expected, parts)
		})
	}
}
