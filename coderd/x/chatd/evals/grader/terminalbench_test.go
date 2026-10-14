package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// An empty task /app must still produce a seed commit with files, since the
// aegis sidecar refuses an empty baseline, and verification must hand the
// task tests the /app the task defines.
func TestEmptySeedPlaceholder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{name: "empty app", want: nil},
		{name: "task-owned gitkeep survives", files: map[string]string{".gitkeep": "", "data.txt": "x"}, want: []string{".gitkeep", "data.txt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			root := t.TempDir()
			appDir := filepath.Join(root, "app")
			require.NoError(t, os.Mkdir(appDir, 0o755))
			for name, body := range tc.files {
				require.NoError(t, os.WriteFile(filepath.Join(appDir, name), []byte(body), 0o600))
			}

			placeholder, err := addEmptySeedPlaceholder(appDir)
			require.NoError(t, err)
			sha, err := commitSeed(ctx, appDir)
			require.NoError(t, err)
			tree, err := runGit(ctx, appDir, "ls-tree", "--name-only", "HEAD")
			require.NoError(t, err)
			require.NotEmpty(t, strings.TrimSpace(string(tree)), "seed commit has no files")
			seedRepo := filepath.Join(root, "seed.git")
			out, err := runGit(ctx, root, "clone", "-q", "--bare", appDir, seedRepo)
			require.NoError(t, err, string(out))

			seed := tbSeed{SeedRepo: "file://" + seedRepo, SeedRef: sha, Placeholder: placeholder}
			verifyDir := filepath.Join(root, "verify")
			require.NoError(t, materializeSeed(ctx, seed, verifyDir))
			require.NoError(t, os.RemoveAll(filepath.Join(verifyDir, ".git")))
			require.NoError(t, removeEmptySeedPlaceholder(verifyDir, seed))

			entries, err := os.ReadDir(verifyDir)
			require.NoError(t, err)
			var got []string
			for _, e := range entries {
				got = append(got, e.Name())
			}
			require.Equal(t, tc.want, got)
		})
	}
}

// Task success is a full reward, however the verifier spells the number.
func TestRewardVerdict(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		raw    string
		passed bool
		reward float64
	}{
		{"1", true, 1},
		{"1.0", true, 1},
		{"1.00", true, 1},
		{"0", false, 0},
		{"0.5", false, 0.5},
		{"", false, 0},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			t.Parallel()
			v, err := rewardVerdict("task", tc.raw, "/logs")
			require.NoError(t, err)
			require.Equal(t, tc.passed, v.Passed, v.Reason)
			require.InDelta(t, tc.reward, v.Metrics["reward"], 1e-9)
		})
	}
	_, err := rewardVerdict("task", "pass", "/logs")
	require.ErrorContains(t, err, "not a number")
}

// A verifier that logs without end must not grow the grader's memory
// without bound; the kept output is its tail, marked as cut.
func TestTailBuffer(t *testing.T) {
	t.Parallel()
	b := &tailBuffer{limit: 8}
	for _, chunk := range []string{"abc", "defgh", "ijklmnopq", "r", "stuvwxyz"} {
		n, err := b.Write([]byte(chunk))
		require.NoError(t, err)
		require.Equal(t, len(chunk), n)
	}
	require.Equal(t, "[18 earlier bytes dropped]\nstuvwxyz", string(b.Bytes()))

	long := &tailBuffer{limit: 64}
	line := []byte("0123456789\n")
	for range 100000 {
		_, err := long.Write(line)
		require.NoError(t, err)
	}
	require.LessOrEqual(t, cap(long.buf), 4*long.limit, "the buffer stays bounded")
	require.True(t, strings.HasPrefix(string(long.Bytes()), "[1099936 earlier bytes dropped]\n"))

	short := &tailBuffer{limit: 8}
	_, err := short.Write([]byte("ok\n"))
	require.NoError(t, err)
	require.Equal(t, "ok\n", string(short.Bytes()))
}
