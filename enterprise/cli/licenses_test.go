package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/cli"
	"github.com/coder/coder/enterprise/coderd"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/testutil"
)

const fakeLicenseJWT = "test.jwt.sig"

func TestLicensesAddSuccess(t *testing.T) {
	// We can't check a real license into the git repo, and can't patch out the keys from here,
	// so instead we have to fake the HTTP interaction.	t.Parallel()
	t.Run("LFlag", func(t *testing.T) {
		t.Parallel()
		cmd := setupFakeLicenseServerTest(t, "licenses", "add", "-l", fakeLicenseJWT)
		pty := attachPty(t, cmd)
		errC := make(chan error)
		go func() {
			errC <- cmd.Execute()
		}()
		require.NoError(t, <-errC)
		pty.ExpectMatch("License with ID 1 added")
	})
	t.Run("Prompt", func(t *testing.T) {
		t.Parallel()
		cmd := setupFakeLicenseServerTest(t, "license", "add")
		pty := attachPty(t, cmd)
		errC := make(chan error)
		go func() {
			errC <- cmd.Execute()
		}()
		pty.ExpectMatch("Paste license:")
		pty.WriteLine(fakeLicenseJWT)
		require.NoError(t, <-errC)
		pty.ExpectMatch("License with ID 1 added")
	})
	t.Run("File", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		filename := filepath.Join(dir, "license.jwt")
		err := os.WriteFile(filename, []byte(fakeLicenseJWT), 0600)
		require.NoError(t, err)
		cmd := setupFakeLicenseServerTest(t, "license", "add", "-f", filename)
		pty := attachPty(t, cmd)
		errC := make(chan error)
		go func() {
			errC <- cmd.Execute()
		}()
		require.NoError(t, <-errC)
		pty.ExpectMatch("License with ID 1 added")
	})
	t.Run("StdIn", func(t *testing.T) {
		t.Parallel()
		cmd := setupFakeLicenseServerTest(t, "license", "add", "-f", "-")
		r, w := io.Pipe()
		cmd.SetIn(r)
		stdout := new(bytes.Buffer)
		cmd.SetOut(stdout)
		errC := make(chan error)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		go func() {
			errC <- cmd.Execute()
		}()
		_, err := w.Write([]byte(fakeLicenseJWT))
		require.NoError(t, err)
		err = w.Close()
		require.NoError(t, err)
		select {
		case err = <-errC:
			require.NoError(t, err)
		case <-ctx.Done():
			t.Error("timed out")
		}
		assert.Equal(t, "License with ID 1 added\n", stdout.String())
	})
	t.Run("DebugOutput", func(t *testing.T) {
		t.Parallel()
		cmd := setupFakeLicenseServerTest(t, "licenses", "add", "-l", fakeLicenseJWT, "--debug")
		pty := attachPty(t, cmd)
		errC := make(chan error)
		go func() {
			errC <- cmd.Execute()
		}()
		require.NoError(t, <-errC)
		pty.ExpectMatch("\"f2\":2")
	})
}

func TestLicensesAddFail(t *testing.T) {
	t.Parallel()
	t.Run("LFlag", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{APIBuilder: coderd.NewEnterprise})
		coderdtest.CreateFirstUser(t, client)
		cmd, root := clitest.NewWithSubcommands(t, cli.EnterpriseSubcommands(),
			"licenses", "add", "-l", fakeLicenseJWT)
		clitest.SetupConfig(t, client, root)

		errC := make(chan error)
		go func() {
			errC <- cmd.Execute()
		}()
		err := <-errC
		var coderError *codersdk.Error
		require.True(t, xerrors.As(err, &coderError))
		assert.Equal(t, 400, coderError.StatusCode())
		assert.Contains(t, "Invalid license", coderError.Message)
	})
}

func setupFakeLicenseServerTest(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	s := httptest.NewServer(&fakeAddLicenseServer{t})
	t.Cleanup(s.Close)
	cmd, root := clitest.NewWithSubcommands(t, cli.EnterpriseSubcommands(), args...)
	err := root.URL().Write(s.URL)
	require.NoError(t, err)
	err = root.Session().Write("sessiontoken")
	require.NoError(t, err)
	return cmd
}

func attachPty(t *testing.T, cmd *cobra.Command) *ptytest.PTY {
	pty := ptytest.New(t)
	cmd.SetIn(pty.Input())
	cmd.SetOut(pty.Output())
	return pty
}

type fakeAddLicenseServer struct {
	t *testing.T
}

func (s *fakeAddLicenseServer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/v2/buildinfo" {
		return
	}
	assert.Equal(s.t, http.MethodPost, r.Method)
	assert.Equal(s.t, "/api/v2/licenses", r.URL.Path)
	var req codersdk.AddLicenseRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	require.NoError(s.t, err)
	assert.Equal(s.t, "test.jwt.sig", req.License)

	resp := codersdk.License{
		ID:         1,
		UploadedAt: time.Now(),
		Claims: map[string]interface{}{
			"h1": "claim1",
			"features": map[string]int64{
				"f1": 1,
				"f2": 2,
			},
		},
	}
	rw.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(rw).Encode(resp)
	assert.NoError(s.t, err)
}
