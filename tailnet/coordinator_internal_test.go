package tailnet

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// UpdateGoldenFiles indicates golden files should be updated.
// To update the golden files:
// make update-golden-files
var UpdateGoldenFiles = flag.Bool("update", false, "update .golden files")

func TestDebugTemplate(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("newlines screw up golden files on windows")
	}
	p1 := uuid.MustParse("01000000-2222-2222-2222-222222222222")
	p2 := uuid.MustParse("02000000-2222-2222-2222-222222222222")
	in := HTMLDebug{
		Peers: []HTMLPeer{
			{
				Name:         "Peer 1",
				ID:           p1,
				LastWriteAge: 5 * time.Second,
				Node:         `id:1 preferred_derp:999 endpoints:"192.168.0.49:4449"`,
				CreatedAge:   87 * time.Second,
				Overwrites:   0,
			},
			{
				Name:         "Peer 2",
				ID:           p2,
				LastWriteAge: 7 * time.Second,
				Node:         `id:2 preferred_derp:999 endpoints:"192.168.0.33:4449"`,
				CreatedAge:   time.Hour,
				Overwrites:   2,
			},
		},
		Tunnels: []HTMLTunnel{
			{
				Src: p1,
				Dst: p2,
			},
		},
	}
	buf := new(bytes.Buffer)
	err := debugTempl.Execute(buf, in)
	require.NoError(t, err)
	actual := buf.Bytes()

	goldenPath := filepath.Join("testdata", "debug.golden.html")
	if *UpdateGoldenFiles {
		t.Logf("update golden file %s", goldenPath)
		err := os.WriteFile(goldenPath, actual, 0o600)
		require.NoError(t, err, "update golden file")
	}

	expected, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "read golden file, run \"make update-golden-files\" and commit the changes")

	require.Equal(
		t, string(expected), string(actual),
		"golden file mismatch: %s, run \"make update-golden-files\", verify and commit the changes",
		goldenPath,
	)
}
