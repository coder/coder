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

	"github.com/go-chi/chi/v5"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/enterprise/cli"
	"github.com/coder/coder/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/testutil"
)

const (
	fakeLicenseJWT = "test.jwt.sig"
	testWarning    = "This is a test warning"
)

func TestLicensesAddFake(t *testing.T) {
	t.Parallel()
	// We can't check a real license into the git repo, and can't patch out the keys from here,
	// so instead we have to fake the HTTP interaction.
	t.Run("LFlag", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		cmd := setupFakeLicenseServerTest(t, "licenses", "add", "-l", fakeLicenseJWT)
		pty := attachPty(t, cmd)
		errC := make(chan error)
		go func() {
			errC <- cmd.ExecuteContext(ctx)
		}()
		require.NoError(t, <-errC)
		pty.ExpectMatch("License with ID 1 added")
	})
	t.Run("Prompt", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		cmd := setupFakeLicenseServerTest(t, "license", "add")
		pty := attachPty(t, cmd)
		errC := make(chan error)
		go func() {
			errC <- cmd.ExecuteContext(ctx)
		}()
		pty.ExpectMatch("Paste license:")
		pty.WriteLine(fakeLicenseJWT)
		require.NoError(t, <-errC)
		pty.ExpectMatch("License with ID 1 added")
	})
	t.Run("File", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		dir := t.TempDir()
		filename := filepath.Join(dir, "license.jwt")
		err := os.WriteFile(filename, []byte(fakeLicenseJWT), 0o600)
		require.NoError(t, err)
		cmd := setupFakeLicenseServerTest(t, "license", "add", "-f", filename)
		pty := attachPty(t, cmd)
		errC := make(chan error)
		go func() {
			errC <- cmd.ExecuteContext(ctx)
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
			errC <- cmd.ExecuteContext(ctx)
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
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		cmd := setupFakeLicenseServerTest(t, "licenses", "add", "-l", fakeLicenseJWT, "--debug")
		pty := attachPty(t, cmd)
		errC := make(chan error)
		go func() {
			errC <- cmd.ExecuteContext(ctx)
		}()
		require.NoError(t, <-errC)
		pty.ExpectMatch("\"f2\": 2")
	})
}

func TestLicensesAddReal(t *testing.T) {
	t.Parallel()
	t.Run("Fails", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		cmd, root := clitest.NewWithSubcommands(t, cli.EnterpriseSubcommands(),
			"licenses", "add", "-l", fakeLicenseJWT)
		clitest.SetupConfig(t, client, root)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		errC := make(chan error)
		go func() {
			errC <- cmd.ExecuteContext(ctx)
		}()
		err := <-errC
		var coderError *codersdk.Error
		require.True(t, xerrors.As(err, &coderError))
		assert.Equal(t, 400, coderError.StatusCode())
		assert.Contains(t, "Invalid license", coderError.Message)
	})
}

func TestLicensesListFake(t *testing.T) {
	t.Parallel()
	// We can't check a real license into the git repo, and can't patch out the keys from here,
	// so instead we have to fake the HTTP interaction.
	t.Run("Mainline", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		cmd := setupFakeLicenseServerTest(t, "licenses", "list")
		stdout := new(bytes.Buffer)
		cmd.SetOut(stdout)
		errC := make(chan error)
		go func() {
			errC <- cmd.ExecuteContext(ctx)
		}()
		require.NoError(t, <-errC)
		var licenses []codersdk.License
		err := json.Unmarshal(stdout.Bytes(), &licenses)
		require.NoError(t, err)
		require.Len(t, licenses, 2)
		assert.Equal(t, int32(1), licenses[0].ID)
		assert.Equal(t, "claim1", licenses[0].Claims["h1"])
		assert.Equal(t, int32(5), licenses[1].ID)
		assert.Equal(t, "claim2", licenses[1].Claims["h2"])
	})
}

func TestLicensesListReal(t *testing.T) {
	t.Parallel()
	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		cmd, root := clitest.NewWithSubcommands(t, cli.EnterpriseSubcommands(),
			"licenses", "list")
		stdout := new(bytes.Buffer)
		cmd.SetOut(stdout)
		stderr := new(bytes.Buffer)
		cmd.SetErr(stderr)
		clitest.SetupConfig(t, client, root)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		errC := make(chan error)
		go func() {
			errC <- cmd.ExecuteContext(ctx)
		}()
		require.NoError(t, <-errC)
		assert.Equal(t, "[]\n", stdout.String())
		assert.Contains(t, testWarning, stderr.String())
	})
}

func TestLicensesDeleteFake(t *testing.T) {
	t.Parallel()
	// We can't check a real license into the git repo, and can't patch out the keys from here,
	// so instead we have to fake the HTTP interaction.
	t.Run("Mainline", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		cmd := setupFakeLicenseServerTest(t, "licenses", "delete", "55")
		pty := attachPty(t, cmd)
		errC := make(chan error)
		go func() {
			errC <- cmd.ExecuteContext(ctx)
		}()
		require.NoError(t, <-errC)
		pty.ExpectMatch("License with ID 55 deleted")
	})
}

func TestLicensesDeleteReal(t *testing.T) {
	t.Parallel()
	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		client := coderdenttest.New(t, nil)
		coderdtest.CreateFirstUser(t, client)
		cmd, root := clitest.NewWithSubcommands(t, cli.EnterpriseSubcommands(),
			"licenses", "delete", "1")
		clitest.SetupConfig(t, client, root)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		errC := make(chan error)
		go func() {
			errC <- cmd.ExecuteContext(ctx)
		}()
		err := <-errC
		var coderError *codersdk.Error
		require.True(t, xerrors.As(err, &coderError))
		assert.Equal(t, 404, coderError.StatusCode())
		assert.Contains(t, "Unknown license ID", coderError.Message)
	})
}

func setupFakeLicenseServerTest(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	s := httptest.NewServer(newFakeLicenseAPI(t))
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

func newFakeLicenseAPI(t *testing.T) http.Handler {
	r := chi.NewRouter()
	a := &fakeLicenseAPI{t: t, r: r}
	r.NotFound(a.notFound)
	r.Post("/api/v2/licenses", a.postLicense)
	r.Get("/api/v2/licenses", a.licenses)
	r.Get("/api/v2/buildinfo", a.noop)
	r.Delete("/api/v2/licenses/{id}", a.deleteLicense)
	r.Get("/api/v2/entitlements", a.entitlements)
	return r
}

type fakeLicenseAPI struct {
	t *testing.T
	r chi.Router
}

func (s *fakeLicenseAPI) notFound(_ http.ResponseWriter, r *http.Request) {
	s.t.Errorf("unexpected HTTP call: %s", r.URL.Path)
}

func (*fakeLicenseAPI) noop(_ http.ResponseWriter, _ *http.Request) {}

func (s *fakeLicenseAPI) postLicense(rw http.ResponseWriter, r *http.Request) {
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

func (s *fakeLicenseAPI) licenses(rw http.ResponseWriter, _ *http.Request) {
	resp := []codersdk.License{
		{
			ID:         1,
			UploadedAt: time.Now(),
			Claims: map[string]interface{}{
				"h1": "claim1",
				"features": map[string]int64{
					"f1": 1,
					"f2": 2,
				},
			},
		},
		{
			ID:         5,
			UploadedAt: time.Now(),
			Claims: map[string]interface{}{
				"h2": "claim2",
				"features": map[string]int64{
					"f3": 3,
					"f4": 4,
				},
			},
		},
	}
	rw.WriteHeader(http.StatusOK)
	err := json.NewEncoder(rw).Encode(resp)
	assert.NoError(s.t, err)
}

func (s *fakeLicenseAPI) deleteLicense(rw http.ResponseWriter, r *http.Request) {
	assert.Equal(s.t, "55", chi.URLParam(r, "id"))
	rw.WriteHeader(200)
}

func (*fakeLicenseAPI) entitlements(rw http.ResponseWriter, r *http.Request) {
	features := make(map[codersdk.FeatureName]codersdk.Feature)
	for _, f := range codersdk.FeatureNames {
		features[f] = codersdk.Feature{
			Entitlement: codersdk.EntitlementEntitled,
			Enabled:     true,
		}
	}
	httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Entitlements{
		Features:     features,
		Warnings:     []string{testWarning},
		HasLicense:   true,
		Experimental: true,
	})
}
