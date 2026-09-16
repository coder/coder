package sandbox_test

import (
	"archive/tar"
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisioner/sandbox"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	base := "version: 1\nimage: registry.example/agent@sha256:" + strings.Repeat("a", 64) + "\n"
	m, err := sandbox.ParseManifest([]byte(base))
	require.NoError(t, err)
	require.Equal(t, float64(1), m.CPU)
	require.EqualValues(t, 2048, m.MemoryMiB)
	require.Equal(t, "/workspace", m.Workdir)
	require.EqualValues(t, 1, m.DailyCost)
	require.Equal(t, map[string]string{"sandbox_host": "local"}, m.Tags())
	for name, body := range map[string]string{
		"unknown": base + "privileged: true\n", "multidoc": base + "---\nversion: 1\n",
		"relative path": base + "workdir: ../host\n", "unclean path": base + "workdir: /workspace/../etc\n",
		"negative memory": base + "memory_mib: -1\n", "nan cpu": base + "cpu: .nan\n",
		"zero memory": base + "memory_mib: 0\n", "zero cpu": base + "cpu: 0\n",
		"zero cost": base + "daily_cost: 0\n", "empty workdir": base + "workdir: ''\n",
		"mutable image":      "version: 1\nimage: registry.example/agent:latest\n",
		"invalid repository": strings.Replace(base, "registry.example/agent", "registry.example/UPPERCASE", 1),
		"duplicate key":      base + "version: 2\n", "unknown version": strings.Replace(base, "version: 1", "version: 2", 1),
	} {
		t.Run(name, func(t *testing.T) { t.Parallel(); _, err := sandbox.ParseManifest([]byte(body)); require.Error(t, err) })
	}
}

func TestManifestArchive(t *testing.T) {
	t.Parallel()
	body := []byte("version: 1\nimage: registry.example/agent@sha256:" + strings.Repeat("b", 64) + "\n")
	for _, duplicate := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid", true: "duplicate"}[duplicate], func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			tw := tar.NewWriter(&buf)
			n := 1
			if duplicate {
				n = 2
			}
			for range n {
				require.NoError(t, tw.WriteHeader(&tar.Header{Name: "sandbox.yaml", Size: int64(len(body)), Mode: 0o600}))
				_, err := tw.Write(body)
				require.NoError(t, err)
			}
			require.NoError(t, tw.Close())
			_, err := sandbox.ReadManifestArchive(buf.Bytes())
			if duplicate {
				require.ErrorContains(t, err, "duplicate")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
