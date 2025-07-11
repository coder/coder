package coderd_test

import (
	"context"
	_ "embed"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

func TestDynamicParameterBuild(t *testing.T) {
	t.Parallel()

	owner, _, _, first := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		},
	})

	orgID := first.OrganizationID

	templateAdmin, templateAdminData := coderdtest.CreateAnotherUser(t, owner, orgID, rbac.ScopedRoleOrgTemplateAdmin(orgID))

	coderdtest.CreateGroup(t, owner, orgID, "developer")
	coderdtest.CreateGroup(t, owner, orgID, "admin", templateAdminData)
	coderdtest.CreateGroup(t, owner, orgID, "auditor")

	// Create a set of templates to test with
	numberValidation, _ := coderdtest.DynamicParameterTemplate(t, templateAdmin, orgID, coderdtest.DynamicParameterTemplateParams{
		MainTF: string(must(os.ReadFile("testdata/parameters/numbers/main.tf"))),
	})

	regexValidation, _ := coderdtest.DynamicParameterTemplate(t, templateAdmin, orgID, coderdtest.DynamicParameterTemplateParams{
		MainTF: string(must(os.ReadFile("testdata/parameters/regex/main.tf"))),
	})

	ephemeralValidation, _ := coderdtest.DynamicParameterTemplate(t, templateAdmin, orgID, coderdtest.DynamicParameterTemplateParams{
		MainTF: string(must(os.ReadFile("testdata/parameters/ephemeral/main.tf"))),
	})

	// complexValidation does conditional parameters, conditional options, and more.
	complexValidation, _ := coderdtest.DynamicParameterTemplate(t, templateAdmin, orgID, coderdtest.DynamicParameterTemplateParams{
		MainTF: string(must(os.ReadFile("testdata/parameters/dynamic/main.tf"))),
	})

	t.Run("NumberValidation", func(t *testing.T) {
		t.Parallel()

		t.Run("OK", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			wrk, err := templateAdmin.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateID: numberValidation.ID,
				Name:       coderdtest.RandomUsername(t),
				RichParameterValues: []codersdk.WorkspaceBuildParameter{
					{Name: "number", Value: `7`},
				},
			})
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, templateAdmin, wrk.LatestBuild.ID)
		})

		t.Run("TooLow", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			_, err := templateAdmin.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateID: numberValidation.ID,
				Name:       coderdtest.RandomUsername(t),
				RichParameterValues: []codersdk.WorkspaceBuildParameter{
					{Name: "number", Value: `-10`},
				},
			})
			require.ErrorContains(t, err, "Number must be between 0 and 10")
		})

		t.Run("TooHigh", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			_, err := templateAdmin.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateID: numberValidation.ID,
				Name:       coderdtest.RandomUsername(t),
				RichParameterValues: []codersdk.WorkspaceBuildParameter{
					{Name: "number", Value: `15`},
				},
			})
			require.ErrorContains(t, err, "Number must be between 0 and 10")
		})
	})

	t.Run("RegexValidation", func(t *testing.T) {
		t.Parallel()

		t.Run("OK", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			wrk, err := templateAdmin.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateID: regexValidation.ID,
				Name:       coderdtest.RandomUsername(t),
				RichParameterValues: []codersdk.WorkspaceBuildParameter{
					{Name: "string", Value: `Hello World!`},
				},
			})
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, templateAdmin, wrk.LatestBuild.ID)
		})

		t.Run("NoValue", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			_, err := templateAdmin.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateID:          regexValidation.ID,
				Name:                coderdtest.RandomUsername(t),
				RichParameterValues: []codersdk.WorkspaceBuildParameter{},
			})
			require.ErrorContains(t, err, "All messages must start with 'Hello'")
		})

		t.Run("Invalid", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			_, err := templateAdmin.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateID: regexValidation.ID,
				Name:       coderdtest.RandomUsername(t),
				RichParameterValues: []codersdk.WorkspaceBuildParameter{
					{Name: "string", Value: `Goodbye!`},
				},
			})
			require.ErrorContains(t, err, "All messages must start with 'Hello'")
		})
	})

	t.Run("EphemeralValidation", func(t *testing.T) {
		t.Parallel()

		t.Run("OK_EphemeralNoPrevious", func(t *testing.T) {
			t.Parallel()

			// Ephemeral params do not take the previous values into account.
			ctx := testutil.Context(t, testutil.WaitShort)
			wrk, err := templateAdmin.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateID: ephemeralValidation.ID,
				Name:       coderdtest.RandomUsername(t),
				RichParameterValues: []codersdk.WorkspaceBuildParameter{
					{Name: "required", Value: `Hello World!`},
					{Name: "defaulted", Value: `Changed`},
				},
			})
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, templateAdmin, wrk.LatestBuild.ID)
			assertWorkspaceBuildParameters(ctx, t, templateAdmin, wrk.LatestBuild.ID, map[string]string{
				"required":  "Hello World!",
				"defaulted": "Changed",
			})

			bld, err := templateAdmin.CreateWorkspaceBuild(ctx, wrk.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStart,
				RichParameterValues: []codersdk.WorkspaceBuildParameter{
					{Name: "required", Value: `Hello World, Again!`},
				},
			})
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, templateAdmin, bld.ID)
			assertWorkspaceBuildParameters(ctx, t, templateAdmin, bld.ID, map[string]string{
				"required":  "Hello World, Again!",
				"defaulted": "original", // Reverts back to the original default value.
			})
		})

		t.Run("Immutable", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			wrk, err := templateAdmin.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateID: numberValidation.ID,
				Name:       coderdtest.RandomUsername(t),
				RichParameterValues: []codersdk.WorkspaceBuildParameter{
					{Name: "number", Value: `7`},
				},
			})
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, templateAdmin, wrk.LatestBuild.ID)
			assertWorkspaceBuildParameters(ctx, t, templateAdmin, wrk.LatestBuild.ID, map[string]string{
				"number": "7",
			})

			_, err = templateAdmin.CreateWorkspaceBuild(ctx, wrk.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStart,
				RichParameterValues: []codersdk.WorkspaceBuildParameter{
					{Name: "number", Value: `8`},
				},
			})
			require.ErrorContains(t, err, `Parameter "number" is not mutable`)
		})

		t.Run("RequiredMissing", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			_, err := templateAdmin.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateID:          ephemeralValidation.ID,
				Name:                coderdtest.RandomUsername(t),
				RichParameterValues: []codersdk.WorkspaceBuildParameter{},
			})
			require.ErrorContains(t, err, "Required parameter not provided")
		})
	})

	t.Run("ComplexValidation", func(t *testing.T) {
		t.Parallel()

		t.Run("OK", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			wrk, err := templateAdmin.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateID: complexValidation.ID,
				Name:       coderdtest.RandomUsername(t),
				RichParameterValues: []codersdk.WorkspaceBuildParameter{
					{Name: "groups", Value: `["admin"]`},
					{Name: "colors", Value: `["red"]`},
					{Name: "thing", Value: "apple"},
				},
			})
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, templateAdmin, wrk.LatestBuild.ID)
		})

		t.Run("BadGroup", func(t *testing.T) {
			// Template admin is not in the "auditor" group, so this should fail.
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			_, err := templateAdmin.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateID: complexValidation.ID,
				Name:       coderdtest.RandomUsername(t),
				RichParameterValues: []codersdk.WorkspaceBuildParameter{
					{Name: "groups", Value: `["auditor", "admin"]`},
					{Name: "colors", Value: `["red"]`},
					{Name: "thing", Value: "apple"},
				},
			})
			require.ErrorContains(t, err, "is not a valid option")
		})

		t.Run("BadColor", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			_, err := templateAdmin.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateID: complexValidation.ID,
				Name:       coderdtest.RandomUsername(t),
				RichParameterValues: []codersdk.WorkspaceBuildParameter{
					{Name: "groups", Value: `["admin"]`},
					{Name: "colors", Value: `["purple"]`},
				},
			})
			require.ErrorContains(t, err, "is not a valid option")
			require.ErrorContains(t, err, "purple")
		})

		t.Run("BadThing", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			_, err := templateAdmin.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateID: complexValidation.ID,
				Name:       coderdtest.RandomUsername(t),
				RichParameterValues: []codersdk.WorkspaceBuildParameter{
					{Name: "groups", Value: `["admin"]`},
					{Name: "colors", Value: `["red"]`},
					{Name: "thing", Value: "leaf"},
				},
			})
			require.ErrorContains(t, err, "must be defined as one of options")
			require.ErrorContains(t, err, "leaf")
		})

		t.Run("BadNumber", func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			_, err := templateAdmin.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateID: complexValidation.ID,
				Name:       coderdtest.RandomUsername(t),
				RichParameterValues: []codersdk.WorkspaceBuildParameter{
					{Name: "groups", Value: `["admin"]`},
					{Name: "colors", Value: `["green"]`},
					{Name: "thing", Value: "leaf"},
					{Name: "number", Value: "100"},
				},
			})
			require.ErrorContains(t, err, "Number must be between 0 and 10")
		})
	})

	t.Run("ImmutableValidation", func(t *testing.T) {
		t.Parallel()

		// NewImmutable tests the case where a new immutable parameter is added to a template
		// after a workspace has been created with an older version of the template.
		// The test tries to delete the workspace, which should succeed.
		t.Run("NewImmutable", func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			// Start with a new template that has 0 parameters
			empty, _ := coderdtest.DynamicParameterTemplate(t, templateAdmin, orgID, coderdtest.DynamicParameterTemplateParams{
				MainTF: string(must(os.ReadFile("testdata/parameters/none/main.tf"))),
			})

			// Create the workspace with 0 parameters
			wrk, err := templateAdmin.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateID:          empty.ID,
				Name:                coderdtest.RandomUsername(t),
				RichParameterValues: []codersdk.WorkspaceBuildParameter{},
			})
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, templateAdmin, wrk.LatestBuild.ID)

			// Update the template with a new immutable parameter
			_, immutable := coderdtest.DynamicParameterTemplate(t, templateAdmin, orgID, coderdtest.DynamicParameterTemplateParams{
				MainTF:     string(must(os.ReadFile("testdata/parameters/immutable/main.tf"))),
				TemplateID: empty.ID,
			})

			bld, err := templateAdmin.CreateWorkspaceBuild(ctx, wrk.ID, codersdk.CreateWorkspaceBuildRequest{
				TemplateVersionID: immutable.ID, // Use the new template version with the immutable parameter
				Transition:        codersdk.WorkspaceTransitionDelete,
			})
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, templateAdmin, bld.ID)

			// Verify the immutable parameter is set on the workspace build
			params, err := templateAdmin.WorkspaceBuildParameters(ctx, bld.ID)
			require.NoError(t, err)
			require.Len(t, params, 1)
			require.Equal(t, "Hello World", params[0].Value)

			// Verify the workspace is deleted
			deleted, err := templateAdmin.DeletedWorkspace(ctx, wrk.ID)
			require.NoError(t, err)
			require.Equal(t, wrk.ID, deleted.ID, "workspace should be deleted")
		})

		t.Run("PreviouslyImmutable", func(t *testing.T) {
			// Ok this is a weird test to document how things are working.
			// What if a parameter flips it's immutability based on a value?
			// The current behavior is to source immutability from the new state.
			// So the value is allowed to be changed.
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			// Start with a new template that has 1 parameter that is immutable
			immutable, _ := coderdtest.DynamicParameterTemplate(t, templateAdmin, orgID, coderdtest.DynamicParameterTemplateParams{
				MainTF: "# PreviouslyImmutable\n" + string(must(os.ReadFile("testdata/parameters/dynamicimmutable/main.tf"))),
			})

			// Create the workspace with the immutable parameter
			wrk, err := templateAdmin.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateID: immutable.ID,
				Name:       coderdtest.RandomUsername(t),
				RichParameterValues: []codersdk.WorkspaceBuildParameter{
					{Name: "isimmutable", Value: "true"},
					{Name: "immutable", Value: "coder"},
				},
			})
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, templateAdmin, wrk.LatestBuild.ID)

			// Try new values
			_, err = templateAdmin.CreateWorkspaceBuild(ctx, wrk.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStart,
				RichParameterValues: []codersdk.WorkspaceBuildParameter{
					{Name: "isimmutable", Value: "false"},
					{Name: "immutable", Value: "not-coder"},
				},
			})
			require.NoError(t, err)
		})

		t.Run("PreviouslyMutable", func(t *testing.T) {
			// The value cannot be changed because it becomes immutable.
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			immutable, _ := coderdtest.DynamicParameterTemplate(t, templateAdmin, orgID, coderdtest.DynamicParameterTemplateParams{
				MainTF: "# PreviouslyMutable\n" + string(must(os.ReadFile("testdata/parameters/dynamicimmutable/main.tf"))),
			})

			// Create the workspace with the mutable parameter
			wrk, err := templateAdmin.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateID: immutable.ID,
				Name:       coderdtest.RandomUsername(t),
				RichParameterValues: []codersdk.WorkspaceBuildParameter{
					{Name: "isimmutable", Value: "false"},
					{Name: "immutable", Value: "coder"},
				},
			})
			require.NoError(t, err)
			coderdtest.AwaitWorkspaceBuildJobCompleted(t, templateAdmin, wrk.LatestBuild.ID)

			// Switch it to immutable, which breaks the validation
			_, err = templateAdmin.CreateWorkspaceBuild(ctx, wrk.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStart,
				RichParameterValues: []codersdk.WorkspaceBuildParameter{
					{Name: "isimmutable", Value: "true"},
					{Name: "immutable", Value: "not-coder"},
				},
			})
			require.Error(t, err)
			require.ErrorContains(t, err, "is not mutable")
		})
	})
}

func TestDynamicWorkspaceTags(t *testing.T) {
	t.Parallel()

	owner, _, _, first := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			IncludeProvisionerDaemon: true,
		},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC:               1,
				codersdk.FeatureExternalProvisionerDaemons: 1,
			},
		},
	})

	orgID := first.OrganizationID

	templateAdmin, _ := coderdtest.CreateAnotherUser(t, owner, orgID, rbac.ScopedRoleOrgTemplateAdmin(orgID))
	// create the template first, mark it as dynamic, then create the second version with the workspace tags.
	// This ensures the template import uses the dynamic tags flow. The second step will happen in a test below.
	workspaceTags, _ := coderdtest.DynamicParameterTemplate(t, templateAdmin, orgID, coderdtest.DynamicParameterTemplateParams{
		MainTF: ``,
	})

	expectedTags := map[string]string{
		"function":    "param is foo",
		"stringvar":   "bar",
		"numvar":      "42",
		"boolvar":     "true",
		"stringparam": "foo",
		"numparam":    "7",
		"boolparam":   "true",
		"listparam":   `["a","b"]`,
		"static":      "static value",
	}

	// A new provisioner daemon is required to make the template version.
	importProvisioner := coderdenttest.NewExternalProvisionerDaemon(t, owner, first.OrganizationID, expectedTags)
	defer importProvisioner.Close()

	// This tests the template import's workspace tags extraction.
	workspaceTags, workspaceTagsVersion := coderdtest.DynamicParameterTemplate(t, templateAdmin, orgID, coderdtest.DynamicParameterTemplateParams{
		MainTF:     string(must(os.ReadFile("testdata/parameters/workspacetags/main.tf"))),
		TemplateID: workspaceTags.ID,
		Version: func(request *codersdk.CreateTemplateVersionRequest) {
			request.ProvisionerTags = map[string]string{
				"static": "static value",
			}
		},
	})
	importProvisioner.Close() // No longer need this provisioner daemon, as the template import is done.

	// Test the workspace create tag extraction.
	expectedTags["function"] = "param is baz"
	expectedTags["stringparam"] = "baz"
	expectedTags["numparam"] = "8"
	expectedTags["boolparam"] = "false"
	workspaceProvisioner := coderdenttest.NewExternalProvisionerDaemon(t, owner, first.OrganizationID, expectedTags)
	defer workspaceProvisioner.Close()

	ctx := testutil.Context(t, testutil.WaitShort)
	wrk, err := templateAdmin.CreateUserWorkspace(ctx, codersdk.Me, codersdk.CreateWorkspaceRequest{
		TemplateVersionID: workspaceTagsVersion.ID,
		Name:              coderdtest.RandomUsername(t),
		RichParameterValues: []codersdk.WorkspaceBuildParameter{
			{Name: "stringparam", Value: "baz"},
			{Name: "numparam", Value: "8"},
			{Name: "boolparam", Value: "false"},
		},
	})
	require.NoError(t, err)

	build, err := templateAdmin.WorkspaceBuild(ctx, wrk.LatestBuild.ID)
	require.NoError(t, err)

	job, err := templateAdmin.OrganizationProvisionerJob(ctx, first.OrganizationID, build.Job.ID)
	require.NoError(t, err)

	// If the tags do no match, the await will fail.
	// 'scope' and 'owner' tags are always included.
	expectedTags["scope"] = "organization"
	expectedTags["owner"] = ""
	require.Equal(t, expectedTags, job.Tags)
	coderdtest.AwaitWorkspaceBuildJobCompleted(t, templateAdmin, wrk.LatestBuild.ID)
}

// TestDynamicParameterTemplate uses a template with some dynamic elements, and
// tests the parameters, values, etc are all as expected.
func TestDynamicParameterTemplate(t *testing.T) {
	t.Parallel()

	owner, _, api, first := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{IncludeProvisionerDaemon: true},
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureTemplateRBAC: 1,
			},
		},
	})

	orgID := first.OrganizationID

	_, userData := coderdtest.CreateAnotherUser(t, owner, orgID)
	templateAdmin, templateAdminData := coderdtest.CreateAnotherUser(t, owner, orgID, rbac.ScopedRoleOrgTemplateAdmin(orgID))
	userAdmin, userAdminData := coderdtest.CreateAnotherUser(t, owner, orgID, rbac.ScopedRoleOrgUserAdmin(orgID))
	_, auditorData := coderdtest.CreateAnotherUser(t, owner, orgID, rbac.ScopedRoleOrgAuditor(orgID))

	coderdtest.CreateGroup(t, owner, orgID, "developer", auditorData, userData)
	coderdtest.CreateGroup(t, owner, orgID, "admin", templateAdminData, userAdminData)
	coderdtest.CreateGroup(t, owner, orgID, "auditor", auditorData, templateAdminData, userAdminData)

	dynamicParametersTerraformSource, err := os.ReadFile("testdata/parameters/dynamic/main.tf")
	require.NoError(t, err)

	_, version := coderdtest.DynamicParameterTemplate(t, templateAdmin, orgID, coderdtest.DynamicParameterTemplateParams{
		MainTF:         string(dynamicParametersTerraformSource),
		Plan:           nil,
		ModulesArchive: nil,
		StaticParams:   nil,
	})

	_ = userAdmin

	ctx := testutil.Context(t, testutil.WaitLong)

	stream, err := templateAdmin.TemplateVersionDynamicParameters(ctx, userData.ID.String(), version.ID)
	require.NoError(t, err)
	defer func() {
		_ = stream.Close(websocket.StatusNormalClosure)

		// Wait until the cache ends up empty. This verifies the cache does not
		// leak any files.
		require.Eventually(t, func() bool {
			return api.AGPL.FileCache.Count() == 0
		}, testutil.WaitShort, testutil.IntervalFast, "file cache should be empty after the test")
	}()

	// Initial response
	preview, pop := coderdtest.SynchronousStream(stream)
	init := pop()
	require.Len(t, init.Diagnostics, 0, "no top level diags")
	coderdtest.AssertParameter(t, "isAdmin", init.Parameters).
		Exists().Value("false")
	coderdtest.AssertParameter(t, "adminonly", init.Parameters).
		NotExists()
	coderdtest.AssertParameter(t, "groups", init.Parameters).
		Exists().Options(database.EveryoneGroup, "developer")

	// Switch to an admin
	resp, err := preview(codersdk.DynamicParametersRequest{
		ID: 1,
		Inputs: map[string]string{
			"colors": `["red"]`,
			"thing":  "apple",
		},
		OwnerID: userAdminData.ID,
	})
	require.NoError(t, err)
	require.Equal(t, resp.ID, 1)
	require.Len(t, resp.Diagnostics, 0, "no top level diags")

	coderdtest.AssertParameter(t, "isAdmin", resp.Parameters).
		Exists().Value("true")
	coderdtest.AssertParameter(t, "adminonly", resp.Parameters).
		Exists()
	coderdtest.AssertParameter(t, "groups", resp.Parameters).
		Exists().Options(database.EveryoneGroup, "admin", "auditor")
	coderdtest.AssertParameter(t, "colors", resp.Parameters).
		Exists().Value(`["red"]`)
	coderdtest.AssertParameter(t, "thing", resp.Parameters).
		Exists().Value("apple").Options("apple", "ruby")
	coderdtest.AssertParameter(t, "cool", resp.Parameters).
		NotExists()

	// Try some other colors
	resp, err = preview(codersdk.DynamicParametersRequest{
		ID: 2,
		Inputs: map[string]string{
			"colors": `["yellow", "blue"]`,
			"thing":  "banana",
		},
		OwnerID: userAdminData.ID,
	})
	require.NoError(t, err)
	require.Equal(t, resp.ID, 2)
	require.Len(t, resp.Diagnostics, 0, "no top level diags")

	coderdtest.AssertParameter(t, "cool", resp.Parameters).
		Exists()
	coderdtest.AssertParameter(t, "isAdmin", resp.Parameters).
		Exists().Value("true")
	coderdtest.AssertParameter(t, "colors", resp.Parameters).
		Exists().Value(`["yellow", "blue"]`)
	coderdtest.AssertParameter(t, "thing", resp.Parameters).
		Exists().Value("banana").Options("banana", "ocean", "sky")
}

func assertWorkspaceBuildParameters(ctx context.Context, t *testing.T, client *codersdk.Client, buildID uuid.UUID, values map[string]string) {
	t.Helper()

	params, err := client.WorkspaceBuildParameters(ctx, buildID)
	require.NoError(t, err)

	for name, value := range values {
		param, ok := slice.Find(params, func(parameter codersdk.WorkspaceBuildParameter) bool {
			return parameter.Name == name
		})
		if !ok {
			assert.Failf(t, "parameter not found", "expected parameter %q to exist with value %q", name, value)
			continue
		}
		assert.Equalf(t, value, param.Value, "parameter %q should have value %q", name, value)
	}

	for _, param := range params {
		if _, ok := values[param.Name]; !ok {
			assert.Failf(t, "unexpected parameter", "parameter %q should not exist", param.Name)
		}
	}
}
