// nolint:gocritic // This is a test package, so database types do not end up in the build
package toolsdk_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
)

func TestChatGPTSearch_TemplateSearch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		query          string
		setupTemplates int
		expectError    bool
		errorContains  string
	}{
		{
			name:           "ValidTemplatesQuery_MultipleTemplates",
			query:          "templates",
			setupTemplates: 3,
			expectError:    false,
		},
		{
			name:           "ValidTemplatesQuery_NoTemplates",
			query:          "templates",
			setupTemplates: 0,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			client, store := coderdtest.NewWithDatabase(t, nil)
			owner := coderdtest.CreateFirstUser(t, client)

			// Create templates as needed
			var expectedTemplates []database.Template
			for i := 0; i < tt.setupTemplates; i++ {
				template := dbfake.TemplateVersion(t, store).
					Seed(database.TemplateVersion{
						OrganizationID: owner.OrganizationID,
						CreatedBy:      owner.UserID,
					}).Do()
				expectedTemplates = append(expectedTemplates, template.Template)
			}

			// Create tool dependencies
			deps, err := toolsdk.NewDeps(client)
			require.NoError(t, err)

			// Execute tool
			args := toolsdk.SearchArgs{Query: tt.query}
			result, err := testTool(t, toolsdk.ChatGPTSearch, deps, args)

			// Verify results
			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					require.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			require.NoError(t, err)
			require.Len(t, result.Results, tt.setupTemplates)

			// Validate result format for each template
			templateIDsFound := make(map[string]bool)
			for _, item := range result.Results {
				require.NotEmpty(t, item.ID)
				require.Contains(t, item.ID, "template:")
				require.NotEmpty(t, item.Title)
				require.Contains(t, item.URL, "/templates/")

				// Track that we found this template ID
				templateIDsFound[item.ID] = true
			}

			// Verify all expected templates are present
			for _, expectedTemplate := range expectedTemplates {
				expectedID := "template:" + expectedTemplate.ID.String()
				require.True(t, templateIDsFound[expectedID], "Expected template %s not found in results", expectedID)
			}
		})
	}
}

func TestChatGPTSearch_TemplateMultipleFilters(t *testing.T) {
	t.Parallel()

	// Setup
	client, store := coderdtest.NewWithDatabase(t, nil)
	owner := coderdtest.CreateFirstUser(t, client)
	org2 := dbgen.Organization(t, store, database.Organization{
		Name: "org2",
	})

	dbgen.Template(t, store, database.Template{
		OrganizationID: owner.OrganizationID,
		CreatedBy:      owner.UserID,
		Name:           "docker-development", // Name contains "docker"
		DisplayName:    "Docker Development",
		Description:    "A Docker-based development template",
	})

	// Create another template that doesn't contain "docker"
	dbgen.Template(t, store, database.Template{
		OrganizationID: org2.ID,
		CreatedBy:      owner.UserID,
		Name:           "python-web", // Name doesn't contain "docker"
		DisplayName:    "Python Web",
		Description:    "A Python web development template",
	})

	// Create third template with "docker" in name
	dockerTemplate2 := dbgen.Template(t, store, database.Template{
		OrganizationID: org2.ID,
		CreatedBy:      owner.UserID,
		Name:           "old-docker-template", // Name contains "docker"
		DisplayName:    "Old Docker Template",
		Description:    "An old Docker template",
	})

	// Create tool dependencies
	deps, err := toolsdk.NewDeps(client)
	require.NoError(t, err)

	args := toolsdk.SearchArgs{Query: "templates/name:docker organization:org2"}
	result, err := testTool(t, toolsdk.ChatGPTSearch, deps, args)

	// Verify results
	require.NoError(t, err)
	require.Len(t, result.Results, 1, "Should match only the docker template in org2")

	expectedID := "template:" + dockerTemplate2.ID.String()
	require.Equal(t, expectedID, result.Results[0].ID, "Should match the docker template in org2")
}

func TestChatGPTSearch_WorkspaceSearch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		query          string
		setupOwner     string // "self" or "other"
		setupWorkspace bool
		expectError    bool
		errorContains  string
	}{
		{
			name:           "ValidWorkspacesQuery_CurrentUser",
			query:          "workspaces",
			setupOwner:     "self",
			setupWorkspace: true,
			expectError:    false,
		},
		{
			name:           "ValidWorkspacesQuery_CurrentUserMe",
			query:          "workspaces/owner:me",
			setupOwner:     "self",
			setupWorkspace: true,
			expectError:    false,
		},
		{
			name:           "ValidWorkspacesQuery_NoWorkspaces",
			query:          "workspaces",
			setupOwner:     "self",
			setupWorkspace: false,
			expectError:    false,
		},
		{
			name:           "ValidWorkspacesQuery_SpecificUser",
			query:          "workspaces/owner:otheruser",
			setupOwner:     "other",
			setupWorkspace: true,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			client, store := coderdtest.NewWithDatabase(t, nil)
			owner := coderdtest.CreateFirstUser(t, client)

			var workspaceOwnerID uuid.UUID
			var workspaceClient *codersdk.Client
			if tt.setupOwner == "self" {
				workspaceOwnerID = owner.UserID
				workspaceClient = client
			} else {
				var workspaceOwner codersdk.User
				workspaceClient, workspaceOwner = coderdtest.CreateAnotherUserMutators(t, client, owner.OrganizationID, nil, func(r *codersdk.CreateUserRequestWithOrgs) {
					r.Username = "otheruser"
				})
				workspaceOwnerID = workspaceOwner.ID
			}

			// Create workspace if needed
			var expectedWorkspace database.WorkspaceTable
			if tt.setupWorkspace {
				workspace := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
					Name:           "test-workspace",
					OrganizationID: owner.OrganizationID,
					OwnerID:        workspaceOwnerID,
				}).Do()
				expectedWorkspace = workspace.Workspace
			}

			// Create tool dependencies
			deps, err := toolsdk.NewDeps(workspaceClient)
			require.NoError(t, err)

			// Execute tool
			args := toolsdk.SearchArgs{Query: tt.query}
			result, err := testTool(t, toolsdk.ChatGPTSearch, deps, args)

			// Verify results
			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					require.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			require.NoError(t, err)

			if tt.setupWorkspace {
				require.Len(t, result.Results, 1)
				item := result.Results[0]
				require.NotEmpty(t, item.ID)
				require.Contains(t, item.ID, "workspace:")
				require.Equal(t, expectedWorkspace.Name, item.Title)
				require.Contains(t, item.Text, "Owner:")
				require.Contains(t, item.Text, "Template:")
				require.Contains(t, item.Text, "Latest transition:")
				require.Contains(t, item.URL, expectedWorkspace.Name)
			} else {
				require.Len(t, result.Results, 0)
			}
		})
	}
}

func TestChatGPTSearch_QueryParsing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		query       string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "ValidTemplatesQuery",
			query:       "templates",
			expectError: false,
		},
		{
			name:        "ValidWorkspacesQuery",
			query:       "workspaces",
			expectError: false,
		},
		{
			name:        "ValidWorkspacesMeQuery",
			query:       "workspaces/owner:me",
			expectError: false,
		},
		{
			name:        "ValidWorkspacesUserQuery",
			query:       "workspaces/owner:testuser",
			expectError: false,
		},
		{
			name:        "InvalidQueryType",
			query:       "users",
			expectError: true,
			errorMsg:    "invalid query",
		},
		{
			name:        "EmptyQuery",
			query:       "",
			expectError: true,
			errorMsg:    "invalid query",
		},
		{
			name:        "MalformedQuery",
			query:       "invalidtype/somequery",
			expectError: true,
			errorMsg:    "invalid query",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Setup minimal environment
			client, _ := coderdtest.NewWithDatabase(t, nil)
			coderdtest.CreateFirstUser(t, client)

			deps, err := toolsdk.NewDeps(client)
			require.NoError(t, err)

			// Execute tool
			args := toolsdk.SearchArgs{Query: tt.query}
			_, err = testTool(t, toolsdk.ChatGPTSearch, deps, args)

			// Verify results
			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestChatGPTFetch_TemplateFetch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		setupTemplate bool
		objectID      string // if empty, will use real template ID
		expectError   bool
		errorContains string
	}{
		{
			name:          "ValidTemplateFetch",
			setupTemplate: true,
			expectError:   false,
		},
		{
			name:          "NonExistentTemplateID",
			setupTemplate: false,
			objectID:      "template:" + uuid.NewString(),
			expectError:   true,
			errorContains: "Resource not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			client, store := coderdtest.NewWithDatabase(t, nil)
			owner := coderdtest.CreateFirstUser(t, client)

			var templateID string
			var expectedTemplate database.Template
			if tt.setupTemplate {
				template := dbfake.TemplateVersion(t, store).
					Seed(database.TemplateVersion{
						OrganizationID: owner.OrganizationID,
						CreatedBy:      owner.UserID,
					}).Do()
				expectedTemplate = template.Template
				templateID = "template:" + template.Template.ID.String()
			} else if tt.objectID != "" {
				templateID = tt.objectID
			}

			// Create tool dependencies
			deps, err := toolsdk.NewDeps(client)
			require.NoError(t, err)

			// Execute tool
			args := toolsdk.FetchArgs{ID: templateID}
			result, err := testTool(t, toolsdk.ChatGPTFetch, deps, args)

			// Verify results
			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					require.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			require.NoError(t, err)
			require.Equal(t, expectedTemplate.ID.String(), result.ID)
			require.Equal(t, expectedTemplate.DisplayName, result.Title)
			require.NotEmpty(t, result.Text)
			require.Contains(t, result.URL, "/templates/")
			require.Contains(t, result.URL, expectedTemplate.Name)

			// Validate JSON marshaling
			var templateData codersdk.Template
			err = json.Unmarshal([]byte(result.Text), &templateData)
			require.NoError(t, err)
			require.Equal(t, expectedTemplate.ID, templateData.ID)
		})
	}
}

func TestChatGPTFetch_WorkspaceFetch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setupWorkspace bool
		objectID       string // if empty, will use real workspace ID
		expectError    bool
		errorContains  string
	}{
		{
			name:           "ValidWorkspaceFetch",
			setupWorkspace: true,
			expectError:    false,
		},
		{
			name:           "NonExistentWorkspaceID",
			setupWorkspace: false,
			objectID:       "workspace:" + uuid.NewString(),
			expectError:    true,
			errorContains:  "Resource not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			client, store := coderdtest.NewWithDatabase(t, nil)
			owner := coderdtest.CreateFirstUser(t, client)

			var workspaceID string
			var expectedWorkspace database.WorkspaceTable
			if tt.setupWorkspace {
				workspace := dbfake.WorkspaceBuild(t, store, database.WorkspaceTable{
					OrganizationID: owner.OrganizationID,
					OwnerID:        owner.UserID,
				}).Do()
				expectedWorkspace = workspace.Workspace
				workspaceID = "workspace:" + workspace.Workspace.ID.String()
			} else if tt.objectID != "" {
				workspaceID = tt.objectID
			}

			// Create tool dependencies
			deps, err := toolsdk.NewDeps(client)
			require.NoError(t, err)

			// Execute tool
			args := toolsdk.FetchArgs{ID: workspaceID}
			result, err := testTool(t, toolsdk.ChatGPTFetch, deps, args)

			// Verify results
			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					require.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			require.NoError(t, err)
			require.Equal(t, expectedWorkspace.ID.String(), result.ID)
			require.Equal(t, expectedWorkspace.Name, result.Title)
			require.NotEmpty(t, result.Text)
			require.Contains(t, result.URL, expectedWorkspace.Name)

			// Validate JSON marshaling
			var workspaceData codersdk.Workspace
			err = json.Unmarshal([]byte(result.Text), &workspaceData)
			require.NoError(t, err)
			require.Equal(t, expectedWorkspace.ID, workspaceData.ID)
		})
	}
}

func TestChatGPTFetch_ObjectIDParsing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		objectID    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "ValidTemplateID",
			objectID:    "template:" + uuid.NewString(),
			expectError: false,
		},
		{
			name:        "ValidWorkspaceID",
			objectID:    "workspace:" + uuid.NewString(),
			expectError: false,
		},
		{
			name:        "MissingColon",
			objectID:    "template" + uuid.NewString(),
			expectError: true,
			errorMsg:    "invalid ID",
		},
		{
			name:        "InvalidUUID",
			objectID:    "template:invalid-uuid",
			expectError: true,
			errorMsg:    "invalid template ID, must be a valid UUID",
		},
		{
			name:        "UnsupportedType",
			objectID:    "user:" + uuid.NewString(),
			expectError: true,
			errorMsg:    "invalid ID",
		},
		{
			name:        "EmptyID",
			objectID:    "",
			expectError: true,
			errorMsg:    "invalid ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Setup minimal environment
			client, _ := coderdtest.NewWithDatabase(t, nil)
			coderdtest.CreateFirstUser(t, client)

			deps, err := toolsdk.NewDeps(client)
			require.NoError(t, err)

			// Execute tool
			args := toolsdk.FetchArgs{ID: tt.objectID}
			_, err = testTool(t, toolsdk.ChatGPTFetch, deps, args)

			// Verify results
			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorMsg)
			} else {
				// For valid formats, we expect it to fail on API call since IDs don't exist
				// but parsing should succeed
				require.Error(t, err)
				require.Contains(t, err.Error(), "Resource not found")
			}
		})
	}
}
