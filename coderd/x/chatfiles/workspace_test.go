package chatfiles_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatfiles"
)

func TestSanitizeWorkspaceUploadName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "plain", input: "foo.zip", want: "foo.zip"},
		{name: "trim_spaces", input: "  foo.zip  ", want: "foo.zip"},
		{name: "replace_whitespace", input: "my file.zip", want: "my_file.zip"},
		{name: "strip_path_components", input: "/etc/passwd", want: "passwd"},
		{name: "windows_path_components", input: `C:\Windows\System32\evil.dll`, want: "evil.dll"},
		{name: "parent_dir_only", input: "../../etc/passwd", want: "passwd"},
		{name: "strip_control_chars", input: "foo\x00\x01\x1fbar.zip", want: "foo_bar.zip"},
		{name: "trim_leading_dots", input: "....bashrc", want: "bashrc"},
		{name: "unicode_passes_through", input: "résumé.pdf", want: "résumé.pdf"},
		{name: "cjk_passes_through", input: "测试.zip", want: "测试.zip"},
		{name: "empty_after_trim", input: "   ", wantErr: true},
		{name: "all_unsafe", input: "/ ", wantErr: true},
		{name: "only_dots", input: "....", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := chatfiles.SanitizeWorkspaceUploadName(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestSanitizeWorkspaceUploadName_Truncates(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", chatfiles.MaxWorkspaceUploadFileNameBytes+50)
	got, err := chatfiles.SanitizeWorkspaceUploadName(long)
	require.NoError(t, err)
	require.LessOrEqual(t, len(got), chatfiles.MaxWorkspaceUploadFileNameBytes)
}

func TestWorkspaceUploadDir(t *testing.T) {
	t.Parallel()

	require.Equal(t, "/home/coder/.coder/chats/abcd1234/files",
		chatfiles.WorkspaceUploadDir("/home/coder", "abcd1234"))
	require.Equal(t, "~/.coder/chats/abcd1234/files",
		chatfiles.WorkspaceUploadDir("", "abcd1234"))
}

func TestWorkspaceChatDir(t *testing.T) {
	t.Parallel()

	require.Equal(t, "/home/coder/.coder/chats/abcd1234",
		chatfiles.WorkspaceChatDir("/home/coder", "abcd1234"))
	require.Equal(t, "~/.coder/chats/abcd1234",
		chatfiles.WorkspaceChatDir("", "abcd1234"))
}

func TestAddCollisionSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{name: "no_suffix_when_first", in: "foo.zip", n: 1, want: "foo.zip"},
		{name: "no_suffix_when_zero", in: "foo.zip", n: 0, want: "foo.zip"},
		{name: "second", in: "foo.zip", n: 2, want: "foo_2.zip"},
		{name: "tenth", in: "foo.zip", n: 10, want: "foo_10.zip"},
		{name: "no_extension", in: "Dockerfile", n: 3, want: "Dockerfile_3"},
		{name: "multi_dot", in: "archive.tar.gz", n: 2, want: "archive.tar_2.gz"},
		{name: "only_extension", in: ".env", n: 2, want: ".env_2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, chatfiles.AddCollisionSuffix(tt.in, tt.n))
		})
	}
}
