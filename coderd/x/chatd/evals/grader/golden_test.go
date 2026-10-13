package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

// TestGoldenCases checks every golden case offline: its hidden tests must
// fail on the untouched seed and pass once the reference solution is
// applied. A case that passes on the seed grades nothing, and one that fails
// with its solution can never be solved.
func TestGoldenCases(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("golden commands use sh")
	}
	files, err := filepath.Glob(filepath.Join("..", "*", "testdata", "golden", "*", "golden.json"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, file := range files {
		caseDir := filepath.Dir(file)
		suiteDir := filepath.Dir(filepath.Dir(filepath.Dir(caseDir)))
		t.Run(filepath.Base(suiteDir)+"/"+filepath.Base(caseDir), func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(file)
			require.NoError(t, err)
			var golden struct {
				Seed    string `json:"seed"`
				Command string `json:"command"`
			}
			require.NoError(t, json.Unmarshal(data, &golden))
			require.NotEmpty(t, golden.Command)
			tool := strings.Fields(golden.Command)[0]
			if _, err := exec.LookPath(tool); err != nil {
				t.Skipf("%s is not installed", tool)
			}
			seed := filepath.Join(suiteDir, "testdata", "seeds", golden.Seed)
			hidden := filepath.Join(caseDir, "hidden")
			solution := filepath.Join(caseDir, "solution")
			require.DirExists(t, hidden)
			require.DirExists(t, solution)

			out, err := runGolden(t, golden.Command, seed, hidden)
			require.Error(t, err, "hidden tests pass on the unmodified seed:\n%s", out)
			out, err = runGolden(t, golden.Command, seed, solution, hidden)
			require.NoError(t, err, "hidden tests fail with the reference solution:\n%s", out)
		})
	}
}

// runGolden copies the layers into a fresh directory, later layers winning,
// and runs command there the way the hidden_tests grader does.
func runGolden(t *testing.T, command string, layers ...string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	for _, layer := range layers {
		require.NoError(t, copyTree(layer, dir))
	}
	tmp := t.TempDir()
	ctx := testutil.Context(t, testutil.WaitSuperLong)
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + tmp,
		"TMPDIR=" + tmp,
		"GOCACHE=" + goEnv(t, "GOCACHE"),
		"GOPATH=" + filepath.Join(tmp, "gopath"),
		"GOFLAGS=-mod=mod",
		"GOPROXY=off",
		"GOTOOLCHAIN=local",
		"PYTHONDONTWRITEBYTECODE=1",
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func goEnv(t *testing.T, key string) string {
	t.Helper()
	out, err := exec.Command("go", "env", key).Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}
