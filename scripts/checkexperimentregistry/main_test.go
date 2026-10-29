package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestCheck(t *testing.T) {
	t.Parallel()

	known := codersdk.Experiments{"alpha", "beta-flag"}

	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "every flag registered",
			src: `experiments:
  - {key: alpha, stage: early-access, since: 2026-01-02, flag: alpha}
  - {key: beta-flag, stage: beta, since: 2026-01-02, flag: beta-flag}
  - {key: unflagged, stage: ga, since: 2026-01-02}
`,
		},
		{
			name: "removed flag still registered",
			src: `experiments:
  - {key: alpha, stage: early-access, since: 2026-01-02, flag: alpha}
  - {key: beta-flag, stage: beta, since: 2026-01-02, flag: beta-flag}
  - {key: gone, stage: early-access, since: 2026-01-02, flag: gone}
`,
			want: []string{`gone: flag "gone" is not in codersdk.ExperimentsKnown`},
		},
		{
			name: "new flag missing from registry",
			src: `experiments:
  - {key: alpha, stage: early-access, since: 2026-01-02, flag: alpha}
`,
			want: []string{`flag "beta-flag" from codersdk.ExperimentsKnown has no registry entry`},
		},
		{
			name: "flag claimed twice",
			src: `experiments:
  - {key: alpha, stage: early-access, since: 2026-01-02, flag: alpha}
  - {key: alpha-again, stage: early-access, since: 2026-01-02, flag: alpha}
  - {key: beta-flag, stage: beta, since: 2026-01-02, flag: beta-flag}
`,
			want: []string{`alpha-again: flag "alpha" already belongs to alpha`},
		},
		{
			name: "invalid entry fields",
			src: `experiments:
  - {key: alpha, stage: early-access, since: 2026-01-02, flag: alpha}
  - {key: alpha, stage: preview, since: soon, flag: beta-flag}
  - {stage: ga, since: 2026-01-02}
`,
			want: []string{
				"alpha: duplicate key",
				`alpha: stage "preview" is not one of [early-access beta ga]`,
				`alpha: since "soon" is not a YYYY-MM-DD date`,
				"experiments[2]: missing key",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := check([]byte(tt.src), known)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestCheckRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	_, err := check([]byte("experiments:\n  - {key: alpha, stage: ga, since: 2026-01-02, flags: alpha}\n"), nil)
	require.ErrorContains(t, err, "field flags not found")
}

func TestRun(t *testing.T) {
	t.Parallel()

	t.Run("mismatch fails", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "registry.yaml")
		require.NoError(t, os.WriteFile(path, []byte("experiments: []\n"), 0o600))
		var stderr bytes.Buffer
		require.Equal(t, 1, run(path, &stderr))
		require.Contains(t, stderr.String(), "has no registry entry")
	})

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()
		var stderr bytes.Buffer
		require.Equal(t, 2, run(filepath.Join(t.TempDir(), "missing.yaml"), &stderr))
	})
}
