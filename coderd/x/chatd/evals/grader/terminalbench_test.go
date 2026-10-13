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
