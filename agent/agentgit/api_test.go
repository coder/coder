package agentgit_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentgit"
)

func TestGitShow_ReturnsFileAtHEAD(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)
	targetFile := filepath.Join(repoDir, "hello.txt")

	// Write and commit a file with known content.
	require.NoError(t, os.WriteFile(targetFile, []byte("committed content\n"), 0o600))
	gitCmd(t, repoDir, "add", "hello.txt")
	gitCmd(t, repoDir, "commit", "-m", "add hello")

	// Modify the working tree version so it differs from HEAD.
	require.NoError(t, os.WriteFile(targetFile, []byte("working tree content\n"), 0o600))

	logger := slogtest.Make(t, nil)
	api := agentgit.NewAPI(logger, nil)

	req := httptest.NewRequest(http.MethodGet, "/show?repo_root="+repoDir+"&path=hello.txt&ref=HEAD", nil)
	rec := httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp agentgit.GitShowResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Equal(t, "committed content\n", resp.Contents)
}

func TestGitShow_FileNotFound(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)
	logger := slogtest.Make(t, nil)
	api := agentgit.NewAPI(logger, nil)

	req := httptest.NewRequest(http.MethodGet, "/show?repo_root="+repoDir+"&path=nonexistent.txt&ref=HEAD", nil)
	rec := httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGitShow_InvalidRepoRoot(t *testing.T) {
	t.Parallel()

	notARepo := t.TempDir()
	logger := slogtest.Make(t, nil)
	api := agentgit.NewAPI(logger, nil)

	req := httptest.NewRequest(http.MethodGet, "/show?repo_root="+notARepo+"&path=file.txt&ref=HEAD", nil)
	rec := httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGitShow_BinaryFile(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)

	// Create a file with null bytes to simulate binary content.
	binPath := filepath.Join(repoDir, "binary.dat")
	require.NoError(t, os.WriteFile(binPath, []byte("hello\x00world"), 0o600))
	gitCmd(t, repoDir, "add", "binary.dat")
	gitCmd(t, repoDir, "commit", "-m", "add binary")

	logger := slogtest.Make(t, nil)
	api := agentgit.NewAPI(logger, nil)

	req := httptest.NewRequest(http.MethodGet, "/show?repo_root="+repoDir+"&path=binary.dat&ref=HEAD", nil)
	rec := httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, rec.Body.String(), "binary file")
}

func TestGitShow_FileTooLarge(t *testing.T) {
	t.Parallel()

	repoDir := initTestRepo(t)

	// Create a file exceeding 512 KB.
	largePath := filepath.Join(repoDir, "large.txt")
	content := strings.Repeat("x", 512*1024+1)
	require.NoError(t, os.WriteFile(largePath, []byte(content), 0o600))
	gitCmd(t, repoDir, "add", "large.txt")
	gitCmd(t, repoDir, "commit", "-m", "add large file")

	logger := slogtest.Make(t, nil)
	api := agentgit.NewAPI(logger, nil)

	req := httptest.NewRequest(http.MethodGet, "/show?repo_root="+repoDir+"&path=large.txt&ref=HEAD", nil)
	rec := httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, rec.Body.String(), "file too large")
}
