package agentpreview

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

// A path element swapped for a symlink after its checks and before its open
// must not serve the symlink's target. The beforeOpen hook makes the swap
// land in that window on every run.
func TestOpenServedRefusesSwappedElement(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin privileges on Windows runners")
	}

	for _, tc := range []struct {
		element string
		target  string
	}{
		{"assets", ".git"},
		{"assets/config", ".secret"},
	} {
		t.Run(tc.element, func(t *testing.T) {
			t.Parallel()

			root := testutil.TempDirResolved(t)
			for name, body := range map[string]string{
				"index.html":     "index",
				"assets/config":  "public",
				"assets/.secret": "secret",
				".git/config":    "secret",
			} {
				name = filepath.Join(root, filepath.FromSlash(name))
				require.NoError(t, os.MkdirAll(filepath.Dir(name), 0o700))
				require.NoError(t, os.WriteFile(name, []byte(body), 0o600))
			}

			api := NewAPI()
			published, _, err := api.publish(root)
			require.NoError(t, err)
			api.beforeOpen = func(prefix string) {
				if prefix != tc.element {
					return
				}
				element := filepath.Join(root, filepath.FromSlash(tc.element))
				swapped := filepath.Join(filepath.Dir(element), ".swapped")
				require.NoError(t, os.Rename(element, swapped))
				require.NoError(t, os.Symlink(tc.target, element))
			}

			request := httptest.NewRequestWithContext(testutil.Context(t, testutil.WaitShort), http.MethodGet, "/"+string(published.ID)+"/assets/config", nil)
			response := httptest.NewRecorder()
			api.Routes().ServeHTTP(response, request)
			require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
		})
	}
}

// Go reports a Windows junction as ModeIrregular, not ModeSymlink or ModeDir,
// and os.Root follows it.
func TestCheckElementRefusesJunctions(t *testing.T) {
	t.Parallel()

	require.ErrorIs(t, checkElement(fs.ModeIrregular), errNotRegular)
}
