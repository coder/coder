package cli_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

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

	setupMultipleOrganizations := func(t *testing.T, args setupArgs) setupData {
		ownerClient, first := coderdenttest.New(t, &coderdenttest.Options{
			Options: &coderdtest.Options{
				// This only affects the first org.
				IncludeProvisionerDaemon: false,
			},
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{
					codersdk.FeatureExternalProvisionerDaemons: 1,
				},
			},
		})

		second := coderdtest.CreateOrganization(t, ownerClient, coderdtest.CreateOrganizationOptions{
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

	t.Run("CreateMultipleOrganization", func(t *testing.T) {
		// Creates a workspace in another organization
		t.Parallel()

		const templateName = "secondtemplate"
		setup := setupMultipleOrganizations(t, setupArgs{
			secondTemplates: []string{templateName},
		})
		member := setup.member

		args := []string{
			"create",
			"my-workspace",
			"--template", templateName,
		}
		inv, root := clitest.New(t, args...)
		clitest.SetupConfig(t, member, root)
		doneChan := make(chan struct{})
		pty := ptytest.New(t).Attach(inv)
		go func() {
			defer close(doneChan)
			err := inv.Run()
			assert.NoError(t, err)
		}()
		matches := []struct {
			match string
			write string
		}{
			{match: "compute.main"},
			{match: "smith (linux, i386)"},
			{match: "Confirm create", write: "yes"},
		}
		for _, m := range matches {
			pty.ExpectMatch(m.match)
			if len(m.write) > 0 {
				pty.WriteLine(m.write)
			}
		}
		<-doneChan

		ws, err := member.WorkspaceByOwnerAndName(context.Background(), codersdk.Me, "my-workspace", codersdk.WorkspaceOptions{})
		if assert.NoError(t, err, "expected workspace to be created") {
			assert.Equal(t, ws.TemplateName, templateName)
			assert.Equal(t, ws.OrganizationName, setup.second.ID, "workspace in second organization")
		}
	})
}
