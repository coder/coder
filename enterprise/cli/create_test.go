package cli_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/pty/ptytest"
)

func TestEnterpriseCreate(t *testing.T) {
	t.Parallel()

	type setupData struct {
		firstResponse codersdk.CreateFirstUserResponse
		second        codersdk.Organization
		owner         *codersdk.Client
		member        *codersdk.Client
	}

	type setupArgs struct {
		firstTemplates  []string
		secondTemplates []string
	}

	// setupMultipleOrganizations creates an extra organization, assigns a member
	// both organizations, and optionally creates templates in each organization.
	setupMultipleOrganizations := func(t *testing.T, args setupArgs) setupData {
		ownerClient, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				// This only affects the first org.
				IncludeProvisionerDaemon: true,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
					codersdk.FeatureMultipleOrganizations:      1,
				},
			},
		})

		second := coderdenttest.CreateOrganization(t, ownerClient, coderdenttest.CreateOrganizationOptions{
			IncludeProvisionerDaemon: true,
		})
		member, _ := coderdtest.CreateAnotherUser(t, ownerClient, first.OrganizationID, rbac.ScopedRoleOrgMember(second.ID))

		var wg sync.WaitGroup

		createTemplate := func(tplName string, orgID uuid.UUID) {
			version := coderdtest.CreateTemplateVersion(t, ownerClient, orgID, nil)
			wg.Add(1)
			go func() {
				coderdtest.AwaitTemplateVersionJobCompleted(t, ownerClient, version.ID)
				wg.Done()
			}()

			coderdtest.CreateTemplate(t, ownerClient, orgID, version.ID, func(request *codersdk.CreateTemplateRequest) {
				request.Name = tplName
			})
		}

		for _, tplName := range args.firstTemplates {
			createTemplate(tplName, first.OrganizationID)
		}

		for _, tplName := range args.secondTemplates {
			createTemplate(tplName, second.ID)
		}

		wg.Wait()

		return setupData{
			firstResponse: first,
			owner:         ownerClient,
			second:        second,
			member:        member,
		}
	}

	// Test creating a workspace in the second organization with a template
	// name.
	t.Run("CreateMultipleOrganization", func(t *testing.T) {
		t.Parallel()

		const templateName = "secondtemplate"
		setup := setupMultipleOrganizations(t, setupArgs{
			secondTemplates: []string{templateName},
		})
		member := setup.member

		args := []string{
			"create",
			"my-workspace",
			"-y",
			"--template", templateName,
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)
		_ = ptytest.New(t).Attach(inv)
		err := inv.Run()
		require.NoError(t, err)

		ws, err := member.WorkspaceByOwnerAndName(context.Background(), codersdk.Me, "my-workspace", codersdk.WorkspaceOptions{})
		if assert.NoError(t, err, "expected workspace to be created") {
			assert.Equal(t, ws.TemplateName, templateName)
			assert.Equal(t, ws.OrganizationName, setup.second.Name, "workspace in second organization")
		}
	})

	// If a template name exists in two organizations, the workspace create will
	// fail.
	t.Run("AmbiguousTemplateName", func(t *testing.T) {
		t.Parallel()

		const templateName = "ambiguous"
		setup := setupMultipleOrganizations(t, setupArgs{
			firstTemplates:  []string{templateName},
			secondTemplates: []string{templateName},
		})
		member := setup.member

		args := []string{
			"create",
			"my-workspace",
			"-y",
			"--template", templateName,
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)
		_ = ptytest.New(t).Attach(inv)
		err := inv.Run()
		require.Error(t, err, "expected error due to ambiguous template name")
		require.ErrorContains(t, err, "multiple templates found")
	})

	// Ambiguous template names are allowed if the organization is specified.
	t.Run("WorkingAmbiguousTemplateName", func(t *testing.T) {
		t.Parallel()

		const templateName = "ambiguous"
		setup := setupMultipleOrganizations(t, setupArgs{
			firstTemplates:  []string{templateName},
			secondTemplates: []string{templateName},
		})
		member := setup.member

		args := []string{
			"create",
			"my-workspace",
			"-y",
			"--template", templateName,
			"--org", setup.second.Name,
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)
		_ = ptytest.New(t).Attach(inv)
		err := inv.Run()
		require.NoError(t, err)

		ws, err := member.WorkspaceByOwnerAndName(context.Background(), codersdk.Me, "my-workspace", codersdk.WorkspaceOptions{})
		if assert.NoError(t, err, "expected workspace to be created") {
			assert.Equal(t, ws.TemplateName, templateName)
			assert.Equal(t, ws.OrganizationName, setup.second.Name, "workspace in second organization")
		}
	})

	// If an organization is specified, but the template is not in that
	// organization, an error is thrown.
	t.Run("CreateIncorrectOrg", func(t *testing.T) {
		t.Parallel()

		const templateName = "secondtemplate"
		setup := setupMultipleOrganizations(t, setupArgs{
			firstTemplates: []string{templateName},
		})
		member := setup.member

		args := []string{
			"create",
			"my-workspace",
			"-y",
			"--org", setup.second.Name,
			"--template", templateName,
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)
		_ = ptytest.New(t).Attach(inv)
		err := inv.Run()
		require.Error(t, err)
		// The error message should indicate the flag to fix the issue.
		require.ErrorContains(t, err, fmt.Sprintf("--org=%q", "coder"))
	})
}
