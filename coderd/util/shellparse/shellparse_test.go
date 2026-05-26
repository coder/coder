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
			want: [][]string{{"find", "/repo"}, {"xargs", "grep"}, {"grep", "bar"}, {"head"}},
		},
		{
			name: "stash-build-pop-exit",
			// "ES=$?" is a pure assignment; not a command.
			in: `cd /repo && git stash && go build ./... 2>&1; ES=$?; git stash pop 2>&1 | tail -1; exit $ES`,
			want: [][]string{
				{"cd", "/repo"},
				{"git", "stash"},
				{"go", "build"},
				{"git", "stash"},
				{"tail"},
				{"exit"},
			},
		},
		{
			name: "command-substitution-and-if",
			in:   `cd /repo && TOKEN=$(cat /tmp/tok || echo "") && if [ -n "$TOKEN" ]; then echo "$TOKEN" | gh auth login --with-token; else echo "missing"; fi`,
			want: [][]string{
				{"cd", "/repo"},
				{"cat", "/tmp/tok"},
				{"echo"},
				{"[", "]"},
				{"echo"},
				{"gh", "auth"},
				{"echo", "missing"},
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
			name: "double-quoted-positional",
			in:   `cd "/repo with spaces"`,
			want: [][]string{{"cd", "/repo with spaces"}},
		},
		{
			name: "single-quoted-positional",
			in:   `grep 'fix bug'`,
			want: [][]string{{"grep", "fix bug"}},
		},
		{
			name: "quoted-program-name",
			in:   `"/usr/bin/git" pull`,
			want: [][]string{{"git", "pull"}},
		},
		{
			name: "absolute-path-binary",
			in:   `/opt/mise/data/installs/go/1.26.2/bin/go test ./...`,
			want: [][]string{{"go", "test"}},
		},
		{
			name: "relative-path-binary",
			in:   `./build.sh --verbose`,
			want: [][]string{{"build.sh"}},
		},
		{
			name: "windows-path-binary",
			in:   `'C:\Program Files\Go\bin\go.exe' test ./...`,
			want: [][]string{{"go.exe", "test"}},
		},
		{
			name: "double-quoted-with-variable-expansion-skipped",
			in:   `echo "hello $name"`,
			// The quoted word contains a parameter expansion, so the
			// parser cannot extract a literal; only the program survives.
			want: [][]string{{"echo"}},
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

	t.Run("unterminated-string-no-results", func(t *testing.T) {
		t.Parallel()
		cmds, err := shellparse.Parse(`echo "unterminated`)
		require.Error(t, err)
		require.Nil(t, cmds)
	})

	t.Run("semicolon-prefix-yields-partial-results-plus-error", func(t *testing.T) {
		t.Parallel()
		// Some malformed inputs (e.g. trailing unterminated tokens after
		// valid semicolon-separated commands) yield partial results
		// alongside a non-nil error. Pin both sides of the contract so
		// future mvdan.cc/sh upgrades that change partial-parse behavior
		// fail this test loudly.
		cmds, err := shellparse.Parse(`ls; cat; echo "unterminated`)
		require.Error(t, err)
		require.Equal(t, [][]string{{"ls"}, {"cat"}}, cmds)
	})
}
