package cli_test

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codeclysm/extract/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/pty/ptytest"
)

// dirSum calculates a checksum of the files in a directory.
func dirSum(t *testing.T, dir string) string {
	ents, err := os.ReadDir(dir)
	require.NoError(t, err)
	sum := sha256.New()
	for _, e := range ents {
		path := filepath.Join(dir, e.Name())

		stat, err := os.Stat(path)
		require.NoError(t, err)

		byt, err := os.ReadFile(
			path,
		)
		require.NoError(t, err, "mode: %+v", stat.Mode())
		_, _ = sum.Write(byt)
	}
	return hex.EncodeToString(sum.Sum(nil))
}

func TestTemplatePull_NoName(t *testing.T) {
	t.Parallel()

	inv, _ := clitest.New(t, "templates", "pull")
	err := inv.Run()
	require.Error(t, err)
}

// Stdout tests that 'templates pull' pulls down the active template
// and writes it to stdout.
func TestTemplatePull_Stdout(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	owner := coderdtest.CreateFirstUser(t, client)
	templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

	// Create an initial template bundle.
	source1 := genTemplateVersionSource()
	// Create an updated template bundle. This will be used to ensure
	// that templates are correctly returned in order from latest to oldest.
	source2 := genTemplateVersionSource()

	expected, err := echo.Tar(source2)
	require.NoError(t, err)

	version1 := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, source1)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version1.ID)

	template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version1.ID)

	// Update the template version so that we can assert that templates
	// are being sorted correctly.
	updatedVersion := coderdtest.UpdateTemplateVersion(t, client, owner.OrganizationID, source2, template.ID)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, updatedVersion.ID)
	coderdtest.UpdateActiveTemplateVersion(t, client, template.ID, updatedVersion.ID)

	// Verify .tar format
	inv, root := clitest.New(t, "templates", "pull", "--tar", template.Name)
	clitest.SetupConfig(t, templateAdmin, root)

	var buf bytes.Buffer
	inv.Stdout = &buf

	err = inv.Run()
	require.NoError(t, err)
	require.True(t, bytes.Equal(expected, buf.Bytes()), "tar files differ")

	// Verify .zip format
	tarReader := tar.NewReader(bytes.NewReader(expected))
	expectedZip, err := coderd.CreateZipFromTar(tarReader)
	require.NoError(t, err)

	inv, root = clitest.New(t, "templates", "pull", "--zip", template.Name)
	clitest.SetupConfig(t, templateAdmin, root)
	buf.Reset()
	inv.Stdout = &buf

	err = inv.Run()
	require.NoError(t, err)
	require.True(t, bytes.Equal(expectedZip, buf.Bytes()), "zip files differ")
}

// Stdout tests that 'templates pull' pulls down the non-latest active template
// and writes it to stdout.
func TestTemplatePull_ActiveOldStdout(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	owner := coderdtest.CreateFirstUser(t, client)
	templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

	source1 := genTemplateVersionSource()
	source2 := genTemplateVersionSource()

	expected, err := echo.Tar(source1)
	require.NoError(t, err)

	version1 := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, source1)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version1.ID)

	template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version1.ID)

	updatedVersion := coderdtest.UpdateTemplateVersion(t, client, owner.OrganizationID, source2, template.ID)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, updatedVersion.ID)

	inv, root := clitest.New(t, "templates", "pull", "--tar", template.Name)
	clitest.SetupConfig(t, templateAdmin, root)

	var buf bytes.Buffer
	inv.Stdout = &buf
	var stderr strings.Builder
	inv.Stderr = &stderr

	err = inv.Run()
	require.NoError(t, err)

	require.True(t, bytes.Equal(expected, buf.Bytes()), "tar files differ")
	require.Contains(t, stderr.String(), "A newer template version than the active version exists.")
}

// Stdout tests that 'templates pull' pulls down the specified template and
// writes it to stdout.
func TestTemplatePull_SpecifiedStdout(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	owner := coderdtest.CreateFirstUser(t, client)
	templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

	source1 := genTemplateVersionSource()
	source2 := genTemplateVersionSource()
	source3 := genTemplateVersionSource()

	expected, err := echo.Tar(source1)
	require.NoError(t, err)

	version1 := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, source1)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version1.ID)

	template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version1.ID)

	updatedVersion := coderdtest.UpdateTemplateVersion(t, client, owner.OrganizationID, source2, template.ID)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, updatedVersion.ID)

	updatedVersion2 := coderdtest.UpdateTemplateVersion(t, client, owner.OrganizationID, source3, template.ID)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, updatedVersion2.ID)
	coderdtest.UpdateActiveTemplateVersion(t, client, template.ID, updatedVersion2.ID)

	inv, root := clitest.New(t, "templates", "pull", "--tar", template.Name, "--version", version1.Name)
	clitest.SetupConfig(t, templateAdmin, root)

	var buf bytes.Buffer
	inv.Stdout = &buf

	err = inv.Run()
	require.NoError(t, err)

	require.True(t, bytes.Equal(expected, buf.Bytes()), "tar files differ")
}

// Stdout tests that 'templates pull' pulls down the latest template
// and writes it to stdout.
func TestTemplatePull_LatestStdout(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	owner := coderdtest.CreateFirstUser(t, client)
	templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

	source1 := genTemplateVersionSource()
	source2 := genTemplateVersionSource()

	expected, err := echo.Tar(source1)
	require.NoError(t, err)

	version1 := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, source1)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version1.ID)

	template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version1.ID)

	updatedVersion := coderdtest.UpdateTemplateVersion(t, client, owner.OrganizationID, source2, template.ID)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, updatedVersion.ID)

	inv, root := clitest.New(t, "templates", "pull", "--tar", template.Name, "latest")
	clitest.SetupConfig(t, templateAdmin, root)

	var buf bytes.Buffer
	inv.Stdout = &buf

	err = inv.Run()
	require.NoError(t, err)

	require.True(t, bytes.Equal(expected, buf.Bytes()), "tar files differ")
}

// ToDir tests that 'templates pull' pulls down the active template
// and writes it to the correct directory.
//
// nolint: paralleltest // The subtests cannot be run in parallel; see the inner loop.
func TestTemplatePull_ToDir(t *testing.T) {
	tests := []struct {
		name           string
		destPath       string
		useDefaultDest bool
	}{
		{
			name:           "absolute path works",
			useDefaultDest: true,
		},
		{
			name:     "relative path to specific dir is sanitized",
			destPath: "./pulltmp",
		},
		{
			name:     "relative path to current dir is sanitized",
			destPath: ".",
		},
		{
			name:     "directory traversal is acceptable",
			destPath: "../mytmpl",
		},
		{
			name:     "empty path falls back to using template name",
			destPath: "",
		},
	}

	// nolint: paralleltest // These tests change the current working dir, and is therefore unsuitable for parallelisation.
	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()

			cwd, err := os.Getwd()
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, os.Chdir(cwd))
			})

			// Change working directory so that relative path tests don't affect the original working directory.
			newWd := filepath.Join(dir, "new-cwd")
			require.NoError(t, os.MkdirAll(newWd, 0o750))
			require.NoError(t, os.Chdir(newWd))

			expectedDest := filepath.Join(dir, "expected")
			actualDest := tc.destPath
			if tc.useDefaultDest {
				actualDest = filepath.Join(dir, "actual")
			}

			client := coderdtest.New(t, &coderdtest.Options{
				IncludeProvisionerDaemon: true,
			})
			owner := coderdtest.CreateFirstUser(t, client)
			templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

			// Create an initial template bundle.
			source1 := genTemplateVersionSource()
			// Create an updated template bundle. This will be used to ensure
			// that templates are correctly returned in order from latest to oldest.
			source2 := genTemplateVersionSource()

			expected, err := echo.Tar(source2)
			require.NoError(t, err)

			version1 := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, source1)
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version1.ID)

			template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version1.ID)

			// Update the template version so that we can assert that templates
			// are being sorted correctly.
			updatedVersion := coderdtest.UpdateTemplateVersion(t, client, owner.OrganizationID, source2, template.ID)
			_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, updatedVersion.ID)
			coderdtest.UpdateActiveTemplateVersion(t, client, template.ID, updatedVersion.ID)

			ctx := context.Background()

			err = extract.Tar(ctx, bytes.NewReader(expected), expectedDest, nil)
			require.NoError(t, err)

			ents, _ := os.ReadDir(actualDest)
			if len(ents) > 0 {
				t.Logf("%s is not empty", actualDest)
				t.FailNow()
			}

			inv, root := clitest.New(t, "templates", "pull", template.Name, actualDest)
			clitest.SetupConfig(t, templateAdmin, root)

			ptytest.New(t).Attach(inv)

			require.NoError(t, inv.Run())

			// Validate behaviour of choosing template name in the absence of an output path argument.
			destPath := actualDest
			if destPath == "" {
				destPath = template.Name
			}

			require.Equal(t,
				dirSum(t, expectedDest),
				dirSum(t, destPath),
			)
		})
	}
}

// FolderConflict tests that 'templates pull' fails when a folder with has
// existing
func TestTemplatePull_FolderConflict(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	owner := coderdtest.CreateFirstUser(t, client)
	templateAdmin, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID, rbac.RoleTemplateAdmin())

	// Create an initial template bundle.
	source1 := genTemplateVersionSource()
	// Create an updated template bundle. This will be used to ensure
	// that templates are correctly returned in order from latest to oldest.
	source2 := genTemplateVersionSource()

	expected, err := echo.Tar(source2)
	require.NoError(t, err)

	version1 := coderdtest.CreateTemplateVersion(t, client, owner.OrganizationID, source1)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version1.ID)

	template := coderdtest.CreateTemplate(t, client, owner.OrganizationID, version1.ID)

	// Update the template version so that we can assert that templates
	// are being sorted correctly.
	updatedVersion := coderdtest.UpdateTemplateVersion(t, client, owner.OrganizationID, source2, template.ID)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, updatedVersion.ID)
	coderdtest.UpdateActiveTemplateVersion(t, client, template.ID, updatedVersion.ID)

	dir := t.TempDir()

	expectedDest := filepath.Join(dir, "expected")
	conflictDest := filepath.Join(dir, "conflict")

	err = os.MkdirAll(conflictDest, 0o700)
	require.NoError(t, err)

	err = os.WriteFile(
		filepath.Join(conflictDest, "conflict-file"),
		[]byte("conflict"), 0o600,
	)
	require.NoError(t, err)

	ctx := context.Background()

	err = extract.Tar(ctx, bytes.NewReader(expected), expectedDest, nil)
	require.NoError(t, err)

	inv, root := clitest.New(t, "templates", "pull", template.Name, conflictDest)
	clitest.SetupConfig(t, templateAdmin, root)

	pty := ptytest.New(t).Attach(inv)

	waiter := clitest.StartWithWaiter(t, inv)

	pty.ExpectMatch("not empty")
	pty.WriteLine("no")

	waiter.RequireError()

	ents, err := os.ReadDir(conflictDest)
	require.NoError(t, err)

	require.Len(t, ents, 1, "conflict folder should have single conflict file")
}

// genTemplateVersionSource returns a unique bundle that can be used to create
// a template version source.
func genTemplateVersionSource() *echo.Responses {
	return &echo.Responses{
		Parse: []*proto.Response{
			{
				Type: &proto.Response_Log{
					Log: &proto.Log{
						Output: uuid.NewString(),
					},
				},
			},

			{
				Type: &proto.Response_Parse{
					Parse: &proto.ParseComplete{},
				},
			},
		},
		ProvisionApply: echo.ApplyComplete,
	}
}
