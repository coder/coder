package cli_test

import (
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

	inv, root := clitest.New(t, "templates", "pull", "--tar", template.Name)
	clitest.SetupConfig(t, templateAdmin, root)

	var buf bytes.Buffer
	inv.Stdout = &buf

	err = inv.Run()
	require.NoError(t, err)

	require.True(t, bytes.Equal(expected, buf.Bytes()), "tar files differ")
}

// Stdout tests that 'templates pull' pulls down the non-latest active template
// and writes it to stdout.
func TestTemplatePull_ActiveOldStdout(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon: true,
	})
	user := coderdtest.CreateFirstUser(t, client)

	source1 := genTemplateVersionSource()
	source2 := genTemplateVersionSource()

	expected, err := echo.Tar(source1)
	require.NoError(t, err)

	version1 := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, source1)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version1.ID)

	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version1.ID)

	updatedVersion := coderdtest.UpdateTemplateVersion(t, client, user.OrganizationID, source2, template.ID)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, updatedVersion.ID)

	inv, root := clitest.New(t, "templates", "pull", "--tar", template.Name)
	clitest.SetupConfig(t, client, root)

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
	user := coderdtest.CreateFirstUser(t, client)

	source1 := genTemplateVersionSource()
	source2 := genTemplateVersionSource()
	source3 := genTemplateVersionSource()

	expected, err := echo.Tar(source1)
	require.NoError(t, err)

	version1 := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, source1)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version1.ID)

	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version1.ID)

	updatedVersion := coderdtest.UpdateTemplateVersion(t, client, user.OrganizationID, source2, template.ID)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, updatedVersion.ID)

	updatedVersion2 := coderdtest.UpdateTemplateVersion(t, client, user.OrganizationID, source3, template.ID)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, updatedVersion2.ID)
	coderdtest.UpdateActiveTemplateVersion(t, client, template.ID, updatedVersion2.ID)

	inv, root := clitest.New(t, "templates", "pull", "--tar", template.Name, "--version", version1.Name)
	clitest.SetupConfig(t, client, root)

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
	user := coderdtest.CreateFirstUser(t, client)

	source1 := genTemplateVersionSource()
	source2 := genTemplateVersionSource()

	expected, err := echo.Tar(source1)
	require.NoError(t, err)

	version1 := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, source1)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, version1.ID)

	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version1.ID)

	updatedVersion := coderdtest.UpdateTemplateVersion(t, client, user.OrganizationID, source2, template.ID)
	_ = coderdtest.AwaitTemplateVersionJobCompleted(t, client, updatedVersion.ID)

	inv, root := clitest.New(t, "templates", "pull", "--tar", template.Name, "latest")
	clitest.SetupConfig(t, client, root)

	var buf bytes.Buffer
	inv.Stdout = &buf

	err = inv.Run()
	require.NoError(t, err)

	require.True(t, bytes.Equal(expected, buf.Bytes()), "tar files differ")
}

// ToDir tests that 'templates pull' pulls down the active template
// and writes it to the correct directory.
func TestTemplatePull_ToDir(t *testing.T) {
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
	actualDest := filepath.Join(dir, "actual")
	ctx := context.Background()

	err = extract.Tar(ctx, bytes.NewReader(expected), expectedDest, nil)
	require.NoError(t, err)

	inv, root := clitest.New(t, "templates", "pull", template.Name, actualDest)
	clitest.SetupConfig(t, templateAdmin, root)

	ptytest.New(t).Attach(inv)

	require.NoError(t, inv.Run())

	require.Equal(t,
		dirSum(t, expectedDest),
		dirSum(t, actualDest),
	)
}

// ToDir tests that 'templates pull' pulls down the active template and writes
// it to a directory with the name of the template if the path is not implicitly
// supplied.
// nolint: paralleltest
func TestTemplatePull_ToImplicit(t *testing.T) {
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

	// create a tempdir and change the working directory to it for the duration of the test (cannot run in parallel)
	dir := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(dir)
	require.NoError(t, err)
	defer func() {
		err := os.Chdir(wd)
		require.NoError(t, err, "if this fails, it can break other subsequent tests due to wrong working directory")
	}()

	expectedDest := filepath.Join(dir, "expected")
	actualDest := filepath.Join(dir, template.Name)

	ctx := context.Background()

	err = extract.Tar(ctx, bytes.NewReader(expected), expectedDest, nil)
	require.NoError(t, err)

	inv, root := clitest.New(t, "templates", "pull", template.Name)
	clitest.SetupConfig(t, templateAdmin, root)

	ptytest.New(t).Attach(inv)

	require.NoError(t, inv.Run())

	require.Equal(t,
		dirSum(t, expectedDest),
		dirSum(t, actualDest),
	)
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
