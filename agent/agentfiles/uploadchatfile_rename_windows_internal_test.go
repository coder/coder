//go:build windows

package agentfiles

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtendedLengthPath(t *testing.T) {
	t.Parallel()

	long := strings.Repeat(`segment\`, 40) + "archive.zip"
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "Short", path: `C:\Users\coder\archive.zip`, want: `C:\Users\coder\archive.zip`},
		{name: "LongDrive", path: `C:\` + long, want: `\\?\C:\` + long},
		{name: "LongUNC", path: `\\server\share\` + long, want: `\\?\UNC\server\share\` + long},
		{name: "LongForwardSlashUNC", path: `//server/share/` + strings.ReplaceAll(long, `\`, "/"), want: `\\?\UNC\server\share\` + long},
		{name: "AlreadyExtended", path: `\\?\C:\` + long, want: `\\?\C:\` + long},
		{name: "AlreadyExtendedUNC", path: `\\?\UNC\server\share\` + long, want: `\\?\UNC\server\share\` + long},
		{name: "Device", path: `\\.\C:\` + long, want: `\\.\C:\` + long},
		{name: "Relative", path: long, want: long},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, extendedLengthPath(tt.path))
		})
	}
}
