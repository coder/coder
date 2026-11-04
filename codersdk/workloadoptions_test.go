package codersdk_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestWorkloadInteractiveValidation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		stdin   bool
		tty     bool
		width   uint16
		height  uint16
		wantErr string
	}{
		{name: "Defaults"},
		{name: "PipedInput", stdin: true},
		{name: "Terminal", stdin: true, tty: true, width: 80, height: 24},
		{name: "NoInput", tty: true, width: 80, height: 24, wantErr: "tty requires stdin"},
		{name: "MissingWidth", stdin: true, tty: true, height: 24, wantErr: "dimensions"},
		{name: "MissingHeight", stdin: true, tty: true, width: 80, wantErr: "dimensions"},
		{name: "NoTTY", stdin: true, width: 80, height: 24, wantErr: "dimensions require tty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := codersdk.CreateWorkloadRequest{
				ID: uuid.New(), TemplateID: uuid.New(), CPU: 1, MemoryBytes: 1 << 30, Command: "claude",
				Stdin: tc.stdin, TTY: tc.tty, Width: tc.width, Height: tc.height,
			}
			err := req.Validate()
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
