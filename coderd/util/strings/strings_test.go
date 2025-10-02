package strings_test

import (
	"fmt"
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
		options  []strings.TruncateOption
	}{
		{"foo", 4, "foo", nil},
		{"foo", 3, "foo", nil},
		{"foo", 2, "fo", nil},
		{"foo", 1, "f", nil},
		{"foo", 0, "", nil},
		{"foo", -1, "", nil},
		{"foo bar", 7, "foo bar", []strings.TruncateOption{strings.TruncateWithEllipsis}},
		{"foo bar", 6, "foo b…", []strings.TruncateOption{strings.TruncateWithEllipsis}},
		{"foo bar", 5, "foo …", []strings.TruncateOption{strings.TruncateWithEllipsis}},
		{"foo bar", 4, "foo…", []strings.TruncateOption{strings.TruncateWithEllipsis}},
		{"foo bar", 3, "fo…", []strings.TruncateOption{strings.TruncateWithEllipsis}},
		{"foo bar", 2, "f…", []strings.TruncateOption{strings.TruncateWithEllipsis}},
		{"foo bar", 1, "…", []strings.TruncateOption{strings.TruncateWithEllipsis}},
		{"foo bar", 0, "", []strings.TruncateOption{strings.TruncateWithEllipsis}},
		{"foo bar", 7, "foo bar", []strings.TruncateOption{strings.TruncateWithFullWords}},
		{"foo bar", 6, "foo", []strings.TruncateOption{strings.TruncateWithFullWords}},
		{"foo bar", 5, "foo", []strings.TruncateOption{strings.TruncateWithFullWords}},
		{"foo bar", 4, "foo", []strings.TruncateOption{strings.TruncateWithFullWords}},
		{"foo bar", 3, "foo", []strings.TruncateOption{strings.TruncateWithFullWords}},
		{"foo bar", 2, "fo", []strings.TruncateOption{strings.TruncateWithFullWords}},
		{"foo bar", 1, "f", []strings.TruncateOption{strings.TruncateWithFullWords}},
		{"foo bar", 0, "", []strings.TruncateOption{strings.TruncateWithFullWords}},
		{"foo bar", 7, "foo bar", []strings.TruncateOption{strings.TruncateWithFullWords, strings.TruncateWithEllipsis}},
		{"foo bar", 6, "foo…", []strings.TruncateOption{strings.TruncateWithFullWords, strings.TruncateWithEllipsis}},
		{"foo bar", 5, "foo…", []strings.TruncateOption{strings.TruncateWithFullWords, strings.TruncateWithEllipsis}},
		{"foo bar", 4, "foo…", []strings.TruncateOption{strings.TruncateWithFullWords, strings.TruncateWithEllipsis}},
		{"foo bar", 3, "fo…", []strings.TruncateOption{strings.TruncateWithFullWords, strings.TruncateWithEllipsis}},
		{"foo bar", 2, "f…", []strings.TruncateOption{strings.TruncateWithFullWords, strings.TruncateWithEllipsis}},
		{"foo bar", 1, "…", []strings.TruncateOption{strings.TruncateWithFullWords, strings.TruncateWithEllipsis}},
		{"foo bar", 0, "", []strings.TruncateOption{strings.TruncateWithFullWords, strings.TruncateWithEllipsis}},
		{"This is a very long task prompt that should be truncated to 160 characters. Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.", 160, "This is a very long task prompt that should be truncated to 160 characters. Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor…", []strings.TruncateOption{strings.TruncateWithFullWords, strings.TruncateWithEllipsis}},
	} {
		tName := fmt.Sprintf("%s_%d", tt.s, tt.n)
		for _, opt := range tt.options {
			tName += fmt.Sprintf("_%v", opt)
		}
		t.Run(tName, func(t *testing.T) {
			t.Parallel()
			actual := strings.Truncate(tt.s, tt.n, tt.options...)
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
