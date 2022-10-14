package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/pty/ptytest"
)

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

		cmd, root := clitest.New(t, "templates", "pull", template.Name)
		clitest.SetupConfig(t, client, root)

		var buf bytes.Buffer
		cmd.SetOut(&buf)

		err = cmd.Execute()
		require.NoError(t, err)

		require.True(t, bytes.Equal(expected, buf.Bytes()), "tar files differ")
	})

	// ToFile tests that 'templates pull' pulls down the latest template
	// and writes it to the correct directory.
	t.Run("ToFile", func(t *testing.T) {
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

		dest := filepath.Join(dir, "actual.tar")

		// Create the file so that we can test that the command
		// warns the user before overwriting a preexisting file.
		fi, err := os.OpenFile(dest, os.O_CREATE|os.O_RDONLY, 0600)
		require.NoError(t, err)
		_ = fi.Close()

		cmd, root := clitest.New(t, "templates", "pull", template.Name, dest)
		clitest.SetupConfig(t, client, root)

		pty := ptytest.New(t)
		cmd.SetIn(pty.Input())
		cmd.SetOut(pty.Output())

		errChan := make(chan error)
		go func() {
			defer close(errChan)
			errChan <- cmd.Execute()
		}()

		// We expect to be prompted that a file already exists.
		pty.ExpectMatch("already exists")
		pty.WriteLine("yes")

		require.NoError(t, <-errChan)

		actual, err := os.ReadFile(dest)
		require.NoError(t, err)

		require.True(t, bytes.Equal(actual, expected), "tar files differ")
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
		Provision: echo.ProvisionComplete,
	}
}
