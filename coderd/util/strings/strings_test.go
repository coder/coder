package strings_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/strings"
)

func TestJoinWithConjunction(t *testing.T) {
	t.Parallel()
	require.Equal(t, "foo", strings.JoinWithConjunction([]string{"foo"}))
	require.Equal(t, "foo and bar", strings.JoinWithConjunction([]string{"foo", "bar"}))
	require.Equal(t, "foo, bar and baz", strings.JoinWithConjunction([]string{"foo", "bar", "baz"}))
}

func TestTruncate(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		s        string
		n        int
		expected string
	}{
		{"foo", 4, "foo"},
		{"foo", 3, "foo"},
		{"foo", 2, "fo"},
		{"foo", 1, "f"},
		{"foo", 0, ""},
		{"foo", -1, ""},
	} {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			actual := strings.Truncate(tt.s, tt.n)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func TestUISanitize(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		s        string
		expected string
	}{
		{"normal text", "normal text"},
		{"\tfoo \r\\nbar  ", "foo bar"},
		{"通常のテキスト", "通常のテキスト"},
		{"foo\nbar", "foo bar"},
		{"foo\tbar", "foo bar"},
		{"foo\rbar", "foo bar"},
		{"foo\x00bar", "foobar"},
		{"\u202Eabc", "abc"},
		{"\u200Bzero width", "zero width"},
		{"foo\x1b[31mred\x1b[0mbar", "fooredbar"},
		{"foo\u0008bar", "foobar"},
		{"foo\x07bar", "foobar"},
		{"foo\uFEFFbar", "foobar"},
		{"<a href='javascript:alert(1)'>link</a>", "link"},
		{"<style>body{display:none}</style>", ""},
		{"<html>HTML</html>", "HTML"},
		{"<br>line break", "line break"},
		{"<link rel='stylesheet' href='evil.css'>", ""},
		{"<img src=1 onerror=alert(1)>", ""},
		{"<!-- comment -->visible", "visible"},
		{"<script>alert('xss')</script>", ""},
		{"<iframe src='evil.com'></iframe>", ""},
	} {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			actual := strings.UISanitize(tt.s)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
