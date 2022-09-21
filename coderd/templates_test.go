package coderd_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/sloggers/slogtest"

	"github.com/coder/coder/agent"
	"github.com/coder/coder/coderd/audit"
	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/provisioner/echo"
	"github.com/coder/coder/provisionersdk/proto"
	"github.com/coder/coder/testutil"
)

func TestTemplate(t *testing.T) {
	t.Parallel()

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.Template(ctx, template.ID)
		require.NoError(t, err)
	})

	t.Run("WorkspaceCount", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		member := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleOwner())
		memberWithDeleted := coderdtest.CreateAnotherUser(t, client, user.OrganizationID, rbac.RoleOwner())
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)

		// Create 3 workspaces with 3 users. 2 workspaces exist, 1 is deleted
		workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

		memberWorkspace := coderdtest.CreateWorkspace(t, member, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, member, memberWorkspace.LatestBuild.ID)

		deletedWorkspace := coderdtest.CreateWorkspace(t, memberWithDeleted, user.OrganizationID, template.ID)
		coderdtest.AwaitWorkspaceBuildJob(t, client, deletedWorkspace.LatestBuild.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		build, err := client.CreateWorkspaceBuild(ctx, deletedWorkspace.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransitionDelete,
		})
		require.NoError(t, err)
		coderdtest.AwaitWorkspaceBuildJob(t, client, build.ID)

		template, err = client.Template(ctx, template.ID)
		require.NoError(t, err)
		require.Equal(t, 2, int(template.WorkspaceOwnerCount), "workspace count")
	})
}

func TestPostTemplateByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, Auditor: auditor})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)

		expected := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		got, err := client.Template(ctx, expected.ID)
		require.NoError(t, err)

		assert.Equal(t, expected.Name, got.Name)
		assert.Equal(t, expected.Description, got.Description)

		require.Len(t, auditor.AuditLogs, 3)
		assert.Equal(t, database.AuditActionCreate, auditor.AuditLogs[0].Action)
		assert.Equal(t, database.AuditActionWrite, auditor.AuditLogs[1].Action)
		assert.Equal(t, database.AuditActionCreate, auditor.AuditLogs[2].Action)
	})

	t.Run("AlreadyExists", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateTemplate(ctx, user.OrganizationID, codersdk.CreateTemplateRequest{
			Name:      template.Name,
			VersionID: version.ID,
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusConflict, apiErr.StatusCode())
	})

	t.Run("MaxTTLTooLow", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateTemplate(ctx, user.OrganizationID, codersdk.CreateTemplateRequest{
			Name:         "testing",
			VersionID:    version.ID,
			MaxTTLMillis: ptr.Ref(int64(-1)),
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
		require.Contains(t, err.Error(), "max_ttl_ms: Must be a positive integer")
	})

	t.Run("MaxTTLTooHigh", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateTemplate(ctx, user.OrganizationID, codersdk.CreateTemplateRequest{
			Name:         "testing",
			VersionID:    version.ID,
			MaxTTLMillis: ptr.Ref(365 * 24 * time.Hour.Milliseconds()),
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusBadRequest, apiErr.StatusCode())
		require.Contains(t, err.Error(), "max_ttl_ms: Cannot be greater than")
	})

	t.Run("NoMaxTTL", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		got, err := client.CreateTemplate(ctx, user.OrganizationID, codersdk.CreateTemplateRequest{
			Name:         "testing",
			VersionID:    version.ID,
			MaxTTLMillis: ptr.Ref(int64(0)),
		})
		require.NoError(t, err)
		require.Zero(t, got.MaxTTLMillis)
	})

	t.Run("Unauthorized", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateTemplate(ctx, uuid.New(), codersdk.CreateTemplateRequest{
			Name:      "test",
			VersionID: uuid.New(),
		})

		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode())
		require.Contains(t, err.Error(), "Try logging in using 'coder login <url>'.")
	})

	t.Run("NoVersion", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.CreateTemplate(ctx, user.OrganizationID, codersdk.CreateTemplateRequest{
			Name:      "test",
			VersionID: uuid.New(),
		})
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})
}

func TestTemplatesByOrganization(t *testing.T) {
	t.Parallel()
	t.Run("ListEmpty", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		templates, err := client.TemplatesByOrganization(ctx, user.OrganizationID)
		require.NoError(t, err)
		require.NotNil(t, templates)
		require.Len(t, templates, 0)
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		templates, err := client.TemplatesByOrganization(ctx, user.OrganizationID)
		require.NoError(t, err)
		require.Len(t, templates, 1)
	})
	t.Run("ListMultiple", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		templates, err := client.TemplatesByOrganization(ctx, user.OrganizationID)
		require.NoError(t, err)
		require.Len(t, templates, 2)
	})
}

func TestTemplateByOrganizationAndName(t *testing.T) {
	t.Parallel()
	t.Run("NotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.TemplateByName(ctx, user.OrganizationID, "something")
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
	})

	t.Run("Found", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.TemplateByName(ctx, user.OrganizationID, template.Name)
		require.NoError(t, err)
	})
}

func TestPatchTemplateMeta(t *testing.T) {
	t.Parallel()

	t.Run("Modified", func(t *testing.T) {
		t.Parallel()

		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, Auditor: auditor})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.Description = "original description"
			ctr.Icon = "/icons/original-icon.png"
			ctr.MaxTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
			ctr.MinAutostartIntervalMillis = ptr.Ref(time.Hour.Milliseconds())
		})
		req := codersdk.UpdateTemplateMeta{
			Name:                       "new-template-name",
			Description:                "lorem ipsum dolor sit amet et cetera",
			Icon:                       "/icons/new-icon.png",
			MaxTTLMillis:               12 * time.Hour.Milliseconds(),
			MinAutostartIntervalMillis: time.Minute.Milliseconds(),
		}
		// It is unfortunate we need to sleep, but the test can fail if the
		// updatedAt is too close together.
		time.Sleep(time.Millisecond * 5)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		updated, err := client.UpdateTemplateMeta(ctx, template.ID, req)
		require.NoError(t, err)
		assert.Greater(t, updated.UpdatedAt, template.UpdatedAt)
		assert.Equal(t, req.Name, updated.Name)
		assert.Equal(t, req.Description, updated.Description)
		assert.Equal(t, req.Icon, updated.Icon)
		assert.Equal(t, req.MaxTTLMillis, updated.MaxTTLMillis)
		assert.Equal(t, req.MinAutostartIntervalMillis, updated.MinAutostartIntervalMillis)

		// Extra paranoid: did it _really_ happen?
		updated, err = client.Template(ctx, template.ID)
		require.NoError(t, err)
		assert.Greater(t, updated.UpdatedAt, template.UpdatedAt)
		assert.Equal(t, req.Name, updated.Name)
		assert.Equal(t, req.Description, updated.Description)
		assert.Equal(t, req.Icon, updated.Icon)
		assert.Equal(t, req.MaxTTLMillis, updated.MaxTTLMillis)
		assert.Equal(t, req.MinAutostartIntervalMillis, updated.MinAutostartIntervalMillis)

		require.Len(t, auditor.AuditLogs, 4)
		assert.Equal(t, database.AuditActionWrite, auditor.AuditLogs[3].Action)
	})

	t.Run("NoMaxTTL", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.MaxTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
		})
		req := codersdk.UpdateTemplateMeta{
			MaxTTLMillis: 0,
		}

		// We're too fast! Sleep so we can be sure that updatedAt is greater
		time.Sleep(time.Millisecond * 5)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.UpdateTemplateMeta(ctx, template.ID, req)
		require.NoError(t, err)

		// Extra paranoid: did it _really_ happen?
		updated, err := client.Template(ctx, template.ID)
		require.NoError(t, err)
		assert.Greater(t, updated.UpdatedAt, template.UpdatedAt)
		assert.Equal(t, req.MaxTTLMillis, updated.MaxTTLMillis)
	})

	t.Run("MaxTTLTooLow", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.MaxTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
		})
		req := codersdk.UpdateTemplateMeta{
			MaxTTLMillis: -1,
		}

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.UpdateTemplateMeta(ctx, template.ID, req)
		require.ErrorContains(t, err, "max_ttl_ms: Must be a positive integer")

		// Ensure no update occurred
		updated, err := client.Template(ctx, template.ID)
		require.NoError(t, err)
		assert.Equal(t, updated.UpdatedAt, template.UpdatedAt)
		assert.Equal(t, updated.MaxTTLMillis, template.MaxTTLMillis)
	})

	t.Run("MaxTTLTooHigh", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.MaxTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
		})
		req := codersdk.UpdateTemplateMeta{
			MaxTTLMillis: 365 * 24 * time.Hour.Milliseconds(),
		}

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		_, err := client.UpdateTemplateMeta(ctx, template.ID, req)
		require.ErrorContains(t, err, "max_ttl_ms: Cannot be greater than")

		// Ensure no update occurred
		updated, err := client.Template(ctx, template.ID)
		require.NoError(t, err)
		assert.Equal(t, updated.UpdatedAt, template.UpdatedAt)
		assert.Equal(t, updated.MaxTTLMillis, template.MaxTTLMillis)
	})

	t.Run("NotModified", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.Description = "original description"
			ctr.Icon = "/icons/original-icon.png"
			ctr.MaxTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
			ctr.MinAutostartIntervalMillis = ptr.Ref(time.Hour.Milliseconds())
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		req := codersdk.UpdateTemplateMeta{
			Name:                       template.Name,
			Description:                template.Description,
			Icon:                       template.Icon,
			MaxTTLMillis:               template.MaxTTLMillis,
			MinAutostartIntervalMillis: template.MinAutostartIntervalMillis,
		}
		_, err := client.UpdateTemplateMeta(ctx, template.ID, req)
		require.ErrorContains(t, err, "not modified")
		updated, err := client.Template(ctx, template.ID)
		require.NoError(t, err)
		assert.Equal(t, updated.UpdatedAt, template.UpdatedAt)
		assert.Equal(t, template.Name, updated.Name)
		assert.Equal(t, template.Description, updated.Description)
		assert.Equal(t, template.Icon, updated.Icon)
		assert.Equal(t, template.MaxTTLMillis, updated.MaxTTLMillis)
		assert.Equal(t, template.MinAutostartIntervalMillis, updated.MinAutostartIntervalMillis)
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.Description = "original description"
			ctr.MaxTTLMillis = ptr.Ref(24 * time.Hour.Milliseconds())
			ctr.MinAutostartIntervalMillis = ptr.Ref(time.Hour.Milliseconds())
		})

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		req := codersdk.UpdateTemplateMeta{
			MaxTTLMillis:               -int64(time.Hour),
			MinAutostartIntervalMillis: -int64(time.Hour),
		}
		_, err := client.UpdateTemplateMeta(ctx, template.ID, req)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Contains(t, apiErr.Message, "Invalid request")
		require.Len(t, apiErr.Validations, 2)
		assert.Equal(t, apiErr.Validations[0].Field, "max_ttl_ms")
		assert.Equal(t, apiErr.Validations[1].Field, "min_autostart_interval_ms")

		updated, err := client.Template(ctx, template.ID)
		require.NoError(t, err)
		assert.WithinDuration(t, template.UpdatedAt, updated.UpdatedAt, time.Minute)
		assert.Equal(t, template.Name, updated.Name)
		assert.Equal(t, template.Description, updated.Description)
		assert.Equal(t, template.Icon, updated.Icon)
		assert.Equal(t, template.MaxTTLMillis, updated.MaxTTLMillis)
		assert.Equal(t, template.MinAutostartIntervalMillis, updated.MinAutostartIntervalMillis)
	})

	t.Run("RemoveIcon", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID, func(ctr *codersdk.CreateTemplateRequest) {
			ctr.Icon = "/icons/code.png"
		})
		req := codersdk.UpdateTemplateMeta{
			Icon: "",
		}

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		updated, err := client.UpdateTemplateMeta(ctx, template.ID, req)
		require.NoError(t, err)
		assert.Equal(t, updated.Icon, "")
	})
}

func TestDeleteTemplate(t *testing.T) {
	t.Parallel()

	t.Run("NoWorkspaces", func(t *testing.T) {
		t.Parallel()
		auditor := audit.NewMock()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true, Auditor: auditor})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.DeleteTemplate(ctx, template.ID)
		require.NoError(t, err)

		require.Len(t, auditor.AuditLogs, 4)
		assert.Equal(t, database.AuditActionDelete, auditor.AuditLogs[3].Action)
	})

	t.Run("Workspaces", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, &coderdtest.Options{IncludeProvisionerDaemon: true})
		user := coderdtest.CreateFirstUser(t, client)
		version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, nil)
		template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
		coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
		coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()

		err := client.DeleteTemplate(ctx, template.ID)
		var apiErr *codersdk.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusPreconditionFailed, apiErr.StatusCode())
	})
}

func TestTemplateDAUs(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, &coderdtest.Options{
		IncludeProvisionerDaemon:    true,
		AgentStatsRefreshInterval:   time.Millisecond * 100,
		MetricsCacheRefreshInterval: time.Millisecond * 100,
	})

	user := coderdtest.CreateFirstUser(t, client)
	authToken := uuid.NewString()
	version := coderdtest.CreateTemplateVersion(t, client, user.OrganizationID, &echo.Responses{
		Parse:           echo.ParseComplete,
		ProvisionDryRun: echo.ProvisionComplete,
		Provision: []*proto.Provision_Response{{
			Type: &proto.Provision_Response_Complete{
				Complete: &proto.Provision_Complete{
					Resources: []*proto.Resource{{
						Name: "example",
						Type: "aws_instance",
						Agents: []*proto.Agent{{
							Id: uuid.NewString(),
							Auth: &proto.Agent_Token{
								Token: authToken,
							},
						}},
					}},
				},
			},
		}},
	})
	template := coderdtest.CreateTemplate(t, client, user.OrganizationID, version.ID)
	require.Equal(t, -1, template.ActiveUserCount)

	coderdtest.AwaitTemplateVersionJob(t, client, version.ID)
	workspace := coderdtest.CreateWorkspace(t, client, user.OrganizationID, template.ID)
	coderdtest.AwaitWorkspaceBuildJob(t, client, workspace.LatestBuild.ID)

	agentClient := codersdk.New(client.URL)
	agentClient.SessionToken = authToken
	agentCloser := agent.New(agent.Options{
		Logger:            slogtest.Make(t, nil),
		StatsReporter:     agentClient.AgentReportStats,
		FetchMetadata:     agentClient.WorkspaceAgentMetadata,
		CoordinatorDialer: agentClient.ListenWorkspaceAgentTailnet,
	})
	defer func() {
		_ = agentCloser.Close()
	}()
	resources := coderdtest.AwaitWorkspaceAgents(t, client, workspace.LatestBuild.ID)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()

	daus, err := client.TemplateDAUs(context.Background(), template.ID)
	require.NoError(t, err)

	require.Equal(t, &codersdk.TemplateDAUsResponse{
		Entries: []codersdk.DAUEntry{},
	}, daus, "no DAUs when stats are empty")

	workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	assert.Zero(t, workspaces[0].LastUsedAt)

	conn, err := client.DialWorkspaceAgentTailnet(ctx, slogtest.Make(t, nil).Named("tailnet"), resources[0].Agents[0].ID)
	require.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()

	sshConn, err := conn.SSHClient()
	require.NoError(t, err)
	_ = sshConn.Close()

	wantDAUs := &codersdk.TemplateDAUsResponse{
		Entries: []codersdk.DAUEntry{
			{

				Date:   time.Now().UTC().Truncate(time.Hour * 24),
				Amount: 1,
			},
		},
	}
	require.Eventuallyf(t, func() bool {
		daus, err = client.TemplateDAUs(ctx, template.ID)
		require.NoError(t, err)
		return len(daus.Entries) > 0
	},
		testutil.WaitShort, testutil.IntervalFast,
		"template daus never loaded",
	)
	gotDAUs, err := client.TemplateDAUs(ctx, template.ID)
	require.NoError(t, err)
	require.Equal(t, gotDAUs, wantDAUs)

	template, err = client.Template(ctx, template.ID)
	require.NoError(t, err)
	require.Equal(t, 1, template.ActiveUserCount)

	workspaces, err = client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	require.NoError(t, err)
	assert.WithinDuration(t,
		database.Now(), workspaces[0].LastUsedAt, time.Minute,
	)
}
