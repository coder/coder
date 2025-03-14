package coderd_test
import (
	"errors"
	"context"
	"fmt"
	"net/http"
	"testing"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/testutil"
)
func createOpts(t *testing.T) *coderdenttest.Options {
	t.Helper()
	dt := coderdtest.DeploymentValues(t)
	return &coderdenttest.Options{
		Options: &coderdtest.Options{
			DeploymentValues: dt,
		},
	}
}
func TestUpdateNotificationTemplateMethod(t *testing.T) {
	t.Parallel()
	t.Run("Happy path", func(t *testing.T) {
		t.Parallel()
		if !dbtestutil.WillUsePostgres() {
			t.Skip("This test requires postgres; it relies on read from and writing to the notification_templates table")
		}
		ctx := testutil.Context(t, testutil.WaitSuperLong)
		api, _ := coderdenttest.New(t, createOpts(t))
		var (
			method     = string(database.NotificationMethodSmtp)
			templateID = notifications.TemplateWorkspaceDeleted
		)
		// Given: a template whose method is initially empty (i.e. deferring to the global method value).
		template, err := getTemplateByID(t, ctx, api, templateID)
		require.NoError(t, err)
		require.NotNil(t, template)
		require.Empty(t, template.Method)
		// When: calling the API to update the method.
		require.NoError(t, api.UpdateNotificationTemplateMethod(ctx, notifications.TemplateWorkspaceDeleted, method), "initial request to set the method failed")
		// Then: the method should be set.
		template, err = getTemplateByID(t, ctx, api, templateID)
		require.NoError(t, err)
		require.NotNil(t, template)
		require.Equal(t, method, template.Method)
	})
	t.Run("Insufficient permissions", func(t *testing.T) {
		t.Parallel()
		if !dbtestutil.WillUsePostgres() {
			t.Skip("This test requires postgres; it relies on read from and writing to the notification_templates table")
		}
		ctx := testutil.Context(t, testutil.WaitSuperLong)
		// Given: the first user which has an "owner" role, and another user which does not.
		api, firstUser := coderdenttest.New(t, createOpts(t))
		anotherClient, _ := coderdtest.CreateAnotherUser(t, api, firstUser.OrganizationID)
		// When: calling the API as an unprivileged user.
		err := anotherClient.UpdateNotificationTemplateMethod(ctx, notifications.TemplateWorkspaceDeleted, string(database.NotificationMethodWebhook))
		// Then: the request is denied because of insufficient permissions.
		var sdkError *codersdk.Error
		require.Error(t, err)
		require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
		require.Equal(t, http.StatusNotFound, sdkError.StatusCode())
		require.Equal(t, "Resource not found or you do not have access to this resource", sdkError.Response.Message)
	})
	t.Run("Invalid notification method", func(t *testing.T) {
		t.Parallel()
		if !dbtestutil.WillUsePostgres() {
			t.Skip("This test requires postgres; it relies on read from and writing to the notification_templates table")
		}
		ctx := testutil.Context(t, testutil.WaitSuperLong)
		// Given: the first user which has an "owner" role
		api, _ := coderdenttest.New(t, createOpts(t))
		// When: calling the API with an invalid method.
		const method = "nope"
		// nolint:gocritic // Using an owner-scope user is kinda the point.
		err := api.UpdateNotificationTemplateMethod(ctx, notifications.TemplateWorkspaceDeleted, method)
		// Then: the request is invalid because of the unacceptable method.
		var sdkError *codersdk.Error
		require.Error(t, err)
		require.ErrorAsf(t, err, &sdkError, "error should be of type *codersdk.Error")
		require.Equal(t, http.StatusBadRequest, sdkError.StatusCode())
		require.Equal(t, "Invalid request to update notification template method", sdkError.Response.Message)
		require.Len(t, sdkError.Response.Validations, 1)
		require.Equal(t, "method", sdkError.Response.Validations[0].Field)
		require.Equal(t, fmt.Sprintf("%q is not a valid method; smtp, webhook, inbox are the available options", method), sdkError.Response.Validations[0].Detail)
	})
	t.Run("Not modified", func(t *testing.T) {
		t.Parallel()
		if !dbtestutil.WillUsePostgres() {
			t.Skip("This test requires postgres; it relies on read from and writing to the notification_templates table")
		}
		ctx := testutil.Context(t, testutil.WaitSuperLong)
		api, _ := coderdenttest.New(t, createOpts(t))
		var (
			method     = string(database.NotificationMethodSmtp)
			templateID = notifications.TemplateWorkspaceDeleted
		)
		template, err := getTemplateByID(t, ctx, api, templateID)
		require.NoError(t, err)
		require.NotNil(t, template)
		// Given: a template whose method is initially empty (i.e. deferring to the global method value).
		require.Empty(t, template.Method)
		// When: calling the API to update the method, it should set it.
		require.NoError(t, api.UpdateNotificationTemplateMethod(ctx, notifications.TemplateWorkspaceDeleted, method), "initial request to set the method failed")
		template, err = getTemplateByID(t, ctx, api, templateID)
		require.NoError(t, err)
		require.NotNil(t, template)
		require.Equal(t, method, template.Method)
		// Then: when calling the API again with the same method, the method will remain unchanged.
		require.NoError(t, api.UpdateNotificationTemplateMethod(ctx, notifications.TemplateWorkspaceDeleted, method), "second request to set the method failed")
		template, err = getTemplateByID(t, ctx, api, templateID)
		require.NoError(t, err)
		require.NotNil(t, template)
		require.Equal(t, method, template.Method)
	})
}
// nolint:revive // t takes precedence.
func getTemplateByID(t *testing.T, ctx context.Context, api *codersdk.Client, id uuid.UUID) (*codersdk.NotificationTemplate, error) {
	t.Helper()
	var template codersdk.NotificationTemplate
	templates, err := api.GetSystemNotificationTemplates(ctx)
	if err != nil {
		return nil, err
	}
	for _, tmpl := range templates {
		if tmpl.ID == id {
			template = tmpl
		}
	}
	if template.ID == uuid.Nil {
		return nil, fmt.Errorf("template not found: %q", id.String())
	}
	return &template, nil
}
