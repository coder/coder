package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/codersdk"
)

func TestPostParameter(t *testing.T) {
	t.Parallel()
	t.Run("BadScope", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateParameter(ctx, codersdk.ParameterScope("something"), user.OrganizationID, codersdk.CreateParameterRequest{
			Name:              "example",
			SourceValue:       "tomato",
			SourceScheme:      codersdk.ParameterSourceSchemeData,
			DestinationScheme: codersdk.ParameterDestinationSchemeProvisionerVariable,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
	})

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		template := createTemplate(t, client, user)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateParameter(ctx, codersdk.ParameterTemplate, template.ID, codersdk.CreateParameterRequest{
			Name:              "example",
			SourceValue:       "tomato",
			SourceScheme:      codersdk.ParameterSourceSchemeData,
			DestinationScheme: codersdk.ParameterDestinationSchemeProvisionerVariable,
		})
		require.NoError(t, err)
	})

	t.Run("AlreadyExists", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		template := createTemplate(t, client, user)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateParameter(ctx, codersdk.ParameterTemplate, template.ID, codersdk.CreateParameterRequest{
			Name:              "example",
			SourceValue:       "tomato",
			SourceScheme:      codersdk.ParameterSourceSchemeData,
			DestinationScheme: codersdk.ParameterDestinationSchemeProvisionerVariable,
		})
		require.NoError(t, err)

		_, err = client.CreateParameter(ctx, codersdk.ParameterTemplate, template.ID, codersdk.CreateParameterRequest{
			Name:              "example",
			SourceValue:       "tomato",
			SourceScheme:      codersdk.ParameterSourceSchemeData,
			DestinationScheme: codersdk.ParameterDestinationSchemeProvisionerVariable,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})
}

func TestParameters(t *testing.T) {
	t.Parallel()
	t.Run("ListEmpty", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		template := createTemplate(t, client, user)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.Parameters(ctx, codersdk.ParameterTemplate, template.ID)
		require.NoError(t, err)
	})
	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		template := createTemplate(t, client, user)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateParameter(ctx, codersdk.ParameterTemplate, template.ID, codersdk.CreateParameterRequest{
			Name:              "example",
			SourceValue:       "tomato",
			SourceScheme:      codersdk.ParameterSourceSchemeData,
			DestinationScheme: codersdk.ParameterDestinationSchemeProvisionerVariable,
		})
		require.NoError(t, err)
		params, err := client.Parameters(ctx, codersdk.ParameterTemplate, template.ID)
		require.NoError(t, err)
		require.Len(t, params, 1)
	})
}

func TestDeleteParameter(t *testing.T) {
	t.Parallel()
	t.Run("NotExist", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		template := createTemplate(t, client, user)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.DeleteParameter(ctx, codersdk.ParameterTemplate, template.ID, "something")
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})
	t.Run("Delete", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		template := createTemplate(t, client, user)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		param, err := client.CreateParameter(ctx, codersdk.ParameterTemplate, template.ID, codersdk.CreateParameterRequest{
			Name:              "example",
			SourceValue:       "tomato",
			SourceScheme:      codersdk.ParameterSourceSchemeData,
			DestinationScheme: codersdk.ParameterDestinationSchemeProvisionerVariable,
		})
		require.NoError(t, err)
		err = client.DeleteParameter(ctx, codersdk.ParameterTemplate, template.ID, param.Name)
		require.NoError(t, err)
	})
}

func createTemplate(t *testing.T, client *codersdk.Client, user codersdk.CreateFirstUserResponse) codersdk.Template {
	instanceID := "instanceidentifier"
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse: echo.ParseComplete,
		ProvisionApply: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "somename",
						Type: "someinstance",
						Agents: []*proto.Agent{{
							Auth: &proto.Agent_InstanceId{
								InstanceId: instanceID,
							},
						}},
					}},
				},
			},
		}},
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	return template
}
