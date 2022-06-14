package cli_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/clitest"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
)

func TestTemplatePull(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerD: true})
	user := coderdtest.CreateFirstUser(t, client)

	// Create an initial template bundle.
	templateSource := &echo.Responses{
		Parse: []*proto.Parse_Response{
			{
				Type: &proto.Parse_Response_Log{
					Log: &proto.Log{Output: "yahoo"},
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

	// Create an updated template bundle. This will be used to ensure
	// that templates are correctly returned in order from latest to oldest.
	templateSource2 := &echo.Responses{
		Parse: []*proto.Parse_Response{
			{
				Type: &proto.Parse_Response_Log{
					Log: &proto.Log{Output: "wahoo"},
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

	expected, err := echo.Tar(templateSource2)
	require.NoError(t, err)

	version1 := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, templateSource)
	_ = coderdtest.AwaitTemplateVersionJob(t, client, version1.ID)
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version1.ID)

	// Update the template version so that we can assert that templates
	// are being sorted correctly.
	_ = coderdtest.UpdateTemplateVersion(t, client, user.OrganizationID, templateSource2, template.ID)

	cmd, root := clitest.New(t, "templates", "pull", template.Name)
	clitest.SetupConfig(t, client, root)

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	err = cmd.Execute()
	require.NoError(t, err)

	require.True(t, bytes.Equal(expected, buf.Bytes()), "Bytes differ")
}
