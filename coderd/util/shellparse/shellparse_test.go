package shellparse_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/shellparse"
)

func TestParse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want [][]string
	}{
		{
			name: "chained-git-workflow",
			in:   `cd /path && git pull && git add . && git commit -m "x"`,
			want: [][]string{{"cd", "/path"}, {"git", "pull"}, {"git", "add"}, {"git", "commit"}},
		},
		{
			name: "single-command-with-flags",
			in:   `ls -la /tmp`,
			want: [][]string{{"ls", "/tmp"}},
		},
		{
			name: "no-arg",
			in:   `pwd`,
			want: [][]string{{"pwd"}},
		},
		{
			name: "find-xargs-grep-pipeline",
			in:   `find /repo -type f | xargs grep "foo" 2>/dev/null | grep -i "bar" | head -30`,
			want: [][]string{{"find", "/repo"}, {"xargs", "grep"}, {"grep"}, {"head"}},
		},
		{
			name: "stash-build-pop-exit",
			// "ES=$?" is a pure assignment; not a command.
			in: `cd /repo && git stash && go build ./... 2>&1; ES=$?; git stash pop 2>&1 | tail -1; exit $ES`,
			want: [][]string{
				{"cd", "/repo"}, {"git", "stash"}, {"go", "build"},
				{"git", "stash"}, {"tail"}, {"exit"},
			},
		},
		{
			name: "command-substitution-and-if",
			in:   `cd /repo && TOKEN=$(cat /tmp/tok || echo "") && if [ -n "$TOKEN" ]; then echo "$TOKEN" | gh auth login --with-token; else echo "missing"; fi`,
			want: [][]string{
				{"cd", "/repo"}, {"cat", "/tmp/tok"}, {"echo"},
				{"[", "]"}, {"echo"}, {"gh", "auth"}, {"echo"},
			},
		},
		{
			name: "for-loop-with-sed",
			in: `cd /repo && for line in 1 2 3; do
  sed -i "${line}s|a|b|" file
done`,
			want: [][]string{{"cd", "/repo"}, {"sed", "file"}},
		},
		{
			name: "subshell-and-brace-group",
			in:   `(cd /tmp && ls) && { echo a; echo b; }`,
			want: [][]string{{"cd", "/tmp"}, {"ls"}, {"echo", "a"}, {"echo", "b"}},
		},
		{
			name: "variable-program-not-literal",
			in:   `$cmd --help && echo done`,
			want: [][]string{{"echo", "done"}},
		},
		{
			name: "empty",
			in:   ``,
			want: nil,
		},
		{
			name: "comment-only",
			in:   `# just a comment`,
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := shellparse.Parse(tc.in)
			require.NoError(t, err, "parse failed for %q", tc.in)
			assert.Equal(t, tc.want, got, "input: %q", tc.in)
		})
	}
}

func TestParse_ParseError(t *testing.T) {
	t.Parallel()

	_, err := shellparse.Parse(`echo "unterminated`)
	require.Error(t, err)
}
