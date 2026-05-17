package support

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSafeDebugLogFileArchiveName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want string
		ok   bool
	}{
		{
			name: "relative path",
			path: ".vscode-server/data/logs/main.log",
			want: ".vscode-server/data/logs/main.log",
			ok:   true,
		},
		{
			name: "cleans path",
			path: "logs/../logs/main.log",
			want: "logs/main.log",
			ok:   true,
		},
		{
			name: "dot",
			path: ".",
		},
		{
			name: "parent",
			path: "../../etc/passwd",
		},
		{
			name: "windows separators parent",
			path: `..\..\windows\system32`,
		},
		{
			name: "absolute path",
			path: "/etc/passwd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := safeDebugLogFileArchiveName(tt.path)
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.want, got)
		})
	}
}
