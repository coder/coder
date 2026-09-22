package chatfiles_test

import (
	"path/filepath"
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
		{name: "windows_path_components", input: "C:\\Windows\\System32\\evil.dll", want: "evil.dll"},
		{name: "parent_dir_only", input: "../../etc/passwd", want: "passwd"},
		{name: "strip_control_chars", input: "foo\x00\x01\x1fbar.zip", want: "foo_bar.zip"},
		{name: "strip_unicode_control_chars", input: "foo\u202ebar.zip", want: "foo_bar.zip"},
		{name: "replace_windows_invalid_chars", input: `bad:name*?.zip`, want: "bad_name_.zip"},
		{name: "trim_leading_dots", input: "....bashrc", want: "bashrc"},
		{name: "trim_trailing_dots", input: "report...", want: "report"},
		{name: "unicode_passes_through", input: "résumé.pdf", want: "résumé.pdf"},
		{name: "cjk_passes_through", input: "测试.zip", want: "测试.zip"},
		{name: "reserved_windows_name", input: "CON", wantErr: true},
		{name: "reserved_windows_name_with_extension", input: "com1.txt", wantErr: true},
		{name: "reserved_windows_name_multiple_extensions", input: "COM1.txt.jpg", wantErr: true},
		{name: "reserved_windows_name_archive_extensions", input: "NUL.tar.gz", wantErr: true},
		{name: "reserved_windows_superscript_name", input: "COM¹.txt", wantErr: true},
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

func TestSanitizeWorkspaceUploadName_TrimsAfterTruncate(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", chatfiles.MaxWorkspaceUploadFileNameBytes-1) + "." + strings.Repeat("b", 10)
	got, err := chatfiles.SanitizeWorkspaceUploadName(long)
	require.NoError(t, err)
	require.Equal(t, strings.Repeat("a", chatfiles.MaxWorkspaceUploadFileNameBytes-1), got)
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

	const chatID = "00000000-0000-0000-0000-000000000001"
	homeDir := filepath.Join("home", "coder")
	require.Equal(t,
		filepath.Join(homeDir, chatfiles.WorkspaceChatsDir, chatID, chatfiles.WorkspaceUploadFilesSubdir),
		chatfiles.WorkspaceUploadDir(homeDir, chatID),
	)
}

func TestWorkspaceChatDir(t *testing.T) {
	t.Parallel()

	const chatID = "00000000-0000-0000-0000-000000000001"
	homeDir := filepath.Join("home", "coder")
	require.Equal(t,
		filepath.Join(homeDir, chatfiles.WorkspaceChatsDir, chatID),
		chatfiles.WorkspaceChatDir(homeDir, chatID),
	)
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

	longName := strings.Repeat("a", chatfiles.MaxWorkspaceUploadFileNameBytes)
	got := chatfiles.AddCollisionSuffix(longName, 2)
	require.LessOrEqual(t, len(got), chatfiles.MaxWorkspaceUploadFileNameBytes)
	require.True(t, strings.HasSuffix(got, "_2"))

	longStem := strings.Repeat("a", chatfiles.MaxWorkspaceUploadFileNameBytes-len(".zip")) + ".zip"
	got = chatfiles.AddCollisionSuffix(longStem, 2)
	require.LessOrEqual(t, len(got), chatfiles.MaxWorkspaceUploadFileNameBytes)
	require.True(t, strings.HasSuffix(got, "_2.zip"))
}
