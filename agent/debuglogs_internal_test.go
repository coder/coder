package agent

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSafeHomeRelPath(t *testing.T) {
	t.Parallel()

	home := filepath.Clean(string(filepath.Separator) + filepath.Join("home", "coder"))
	tests := []struct {
		name string
		path string
		want string
		ok   bool
	}{
		{
			name: "inside home",
			path: filepath.Join(home, ".vscode-server", "data", "logs", "main.log"),
			want: ".vscode-server/data/logs/main.log",
			ok:   true,
		},
		{
			name: "home itself",
			path: home,
		},
		{
			name: "sibling prefix",
			path: home + "-evil" + string(filepath.Separator) + "main.log",
		},
		{
			name: "outside home",
			path: filepath.Clean(string(filepath.Separator) + filepath.Join("var", "log", "main.log")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := safeHomeRelPath(home, tt.path)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.want, got)
		})
	}
}
