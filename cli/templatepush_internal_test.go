package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisioner/sandbox"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestTemplatePushProvisionerOption(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		args        []string
		provisioner codersdk.ProvisionerType
		warn        bool
	}{
		{name: "Default", provisioner: codersdk.ProvisionerTypeTerraform, warn: true},
		{name: "Sandbox", args: []string{"--provisioner", "sandbox"}, provisioner: codersdk.ProvisionerTypeSandbox},
		{name: "EchoAlias", args: []string{"--test.provisioner", "echo"}, provisioner: codersdk.ProvisionerTypeEcho, warn: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			flags := templateUploadFlags{directory: t.TempDir()}
			cmd := (&RootCmd{}).templatePush()
			cmd.Handler = func(inv *serpent.Invocation) error {
				var provisioner codersdk.ProvisionerType
				for _, opt := range cmd.Options {
					if opt.Flag == "provisioner" {
						provisioner = codersdk.ProvisionerType(opt.Value.String())
					}
				}
				require.Equal(t, tc.provisioner, provisioner)
				return flags.checkForLockfile(inv, provisioner)
			}
			args := append([]string{"--directory", flags.directory}, tc.args...)
			inv := cmd.Invoke(args...).WithContext(t.Context())
			var output bytes.Buffer
			inv.Stdout = &output
			inv.Stderr = io.Discard
			require.NoError(t, inv.Run())
			if tc.warn {
				require.Contains(t, output.String(), "No .terraform.lock.hcl file found")
			} else {
				require.Empty(t, output.String())
			}
		})
	}
}

func TestTemplateUploadSandboxDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sandbox.yaml"), []byte(archiveSandboxManifest), 0o600))
	fileID := uuid.New()
	uploaded := false
	client := codersdk.New(&url.URL{Scheme: "http", Host: "template-upload.invalid"})
	client.HTTPClient = &http.Client{Transport: testutil.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, req.Method)
		require.Equal(t, "/api/v2/files", req.URL.Path)
		require.Equal(t, codersdk.ContentTypeTar, req.Header.Get("Content-Type"))
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		manifest, err := sandbox.ReadManifestArchive(body)
		require.NoError(t, err)
		require.Equal(t, int32(1), manifest.DailyCost)
		uploaded = true
		response, err := json.Marshal(codersdk.UploadResponse{ID: fileID})
		require.NoError(t, err)
		return &http.Response{StatusCode: http.StatusCreated, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(response)), Request: req}, nil
	})}
	flags := templateUploadFlags{}
	cmd := &serpent.Command{
		Use:     "upload",
		Options: append(flags.options(), cliui.SkipPromptOption()),
		Handler: func(inv *serpent.Invocation) error {
			response, err := flags.upload(inv, client, codersdk.ProvisionerTypeSandbox)
			if err != nil {
				return err
			}
			require.Equal(t, fileID, response.ID)
			return nil
		},
	}
	inv := cmd.Invoke("--directory", dir, "--yes").WithContext(t.Context())
	inv.Stdout, inv.Stderr = io.Discard, io.Discard
	require.NoError(t, inv.Run())
	require.True(t, uploaded, "the upload must include a validated native archive without Terraform files")
}
