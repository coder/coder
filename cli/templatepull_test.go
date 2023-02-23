package cli_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/codeclysm/extract"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/pty/ptytest"
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

func TestTemplatePull(t *testing.T) {
	t.Parallel()

	t.Run("NoName", func(t *testing.T) {
		t.Parallel()

		cmd, _ := clitest.New(t, "templates", "pull")
		err := cmd.Execute()
		require.Error(t, err)
	})

	// Stdout tests that 'templates pull' pulls down the latest template
	// and writes it to stdout.
	t.Run("Stdout", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)

		// Create an initial template bundle.
		source1 := genTemplateVersionSource()
		// Create an updated template bundle. This will be used to ensure
		// that templates are correctly returned in order from latest to oldest.
		source2 := genTemplateVersionSource()

		expected, err := echo.Tar(source2)
		require.NoError(t, err)

		version1 := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, source1)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, version1.ID)

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version1.ID)

		// Update the template version so that we can assert that templates
		// are being sorted correctly.
		_ = coderdtest.UpdateTemplateVersion(t, client, user.OrganizationID, source2, template.ID)

		cmd, root := clitest.New(t, "templates", "pull", "--tar", template.Name)
		clitest.SetupConfig(t, client, root)

		var buf bytes.Buffer
		cmd.SetOut(&buf)

		err = cmd.Execute()
		require.NoError(t, err)

		require.True(t, bytes.Equal(expected, buf.Bytes()), "tar files differ")
	})

	// ToDir tests that 'templates pull' pulls down the latest template
	// and writes it to the correct directory.
	t.Run("ToDir", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)

		// Create an initial template bundle.
		source1 := genTemplateVersionSource()
		// Create an updated template bundle. This will be used to ensure
		// that templates are correctly returned in order from latest to oldest.
		source2 := genTemplateVersionSource()

		expected, err := echo.Tar(source2)
		require.NoError(t, err)

		version1 := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, source1)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, version1.ID)

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version1.ID)

		// Update the template version so that we can assert that templates
		// are being sorted correctly.
		_ = coderdtest.UpdateTemplateVersion(t, client, user.OrganizationID, source2, template.ID)

		dir := t.TempDir()

		expectedDest := filepath.Join(dir, "expected")
		actualDest := filepath.Join(dir, "actual")
		ctx := context.Background()

		err = extract.Tar(ctx, bytes.NewReader(expected), expectedDest, nil)
		require.NoError(t, err)

		cmd, root := clitest.New(t, "templates", "pull", template.Name, actualDest)
		clitest.SetupConfig(t, client, root)

		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())

		errChan := make(chan error)
		go func() {
			defer close(errChan)
			errChan <- cmd.Execute()
		}()

		require.NoError(t, <-errChan)

		require.Equal(t,
			dirSum(t, expectedDest),
			dirSum(t, actualDest),
		)
	})

	// FolderConflict tests that 'templates pull' fails when a folder with has
	// existing
	t.Run("FolderConflict", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)

		// Create an initial template bundle.
		source1 := genTemplateVersionSource()
		// Create an updated template bundle. This will be used to ensure
		// that templates are correctly returned in order from latest to oldest.
		source2 := genTemplateVersionSource()

		expected, err := echo.Tar(source2)
		require.NoError(t, err)

		version1 := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, source1)
		_ = coderdtest.AwaitTemplateVersionJob(t, client, version1.ID)

		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version1.ID)

		// Update the template version so that we can assert that templates
		// are being sorted correctly.
		_ = coderdtest.UpdateTemplateVersion(t, client, user.OrganizationID, source2, template.ID)

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

		cmd, root := clitest.New(t, "templates", "pull", template.Name, conflictDest)
		clitest.SetupConfig(t, client, root)

		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())

		errChan := make(chan error)
		go func() {
			defer close(errChan)
			errChan <- cmd.Execute()
		}()

		pty.ExpectMatch("not empty")
		pty.WriteLine("no")

		require.Error(t, <-errChan)

		ents, err := os.ReadDir(conflictDest)
		require.NoError(t, err)

		require.Len(t, ents, 1, "conflict folder should have single conflict file")
	})
}

// genTemplateVersionSource returns a unique bundle that can be used to create
// a template version source.
func genTemplateVersionSource() *echo.Responses {
	return &echo.Responses{
		Parse: []*proto.Parse_Response{
			{
				Type: &proto.Parse_Response_Log{
					Log: &proto.Log{
						Output: uuid.NewString(),
					},
				},
			},

			{
				Type: &proto.Parse_Response_Complete{
					Complete: &proto.Parse_Complete{},
				},
			},
		},
		ProvisionApply: echo.ProvisionComplete,
	}
}
