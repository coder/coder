package cli_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestTaskCreate(t *testing.T) {
	t.Parallel()

	var (
		taskCreatedAt = time.Now()

		organizationID          = uuid.New()
		anotherOrganizationID   = uuid.New()
		templateID              = uuid.New()
		templateVersionID       = uuid.New()
		templateVersionPresetID = uuid.New()
	)

	templateAndVersionFoundHandler := func(t *testing.T, ctx context.Context, orgID uuid.UUID, templateName, templateVersionName, presetName, prompt string) http.HandlerFunc {
		t.Helper()

		return func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/v2/users/me/organizations":
				httpapi.Write(ctx, w, http.StatusOK, []codersdk.Organization{
					{MinimalOrganization: codersdk.MinimalOrganization{
						ID: orgID,
					}},
				})
			case fmt.Sprintf("/api/v2/organizations/%s/templates/my-template/versions/my-template-version", orgID):
				httpapi.Write(ctx, w, http.StatusOK, codersdk.TemplateVersion{
					ID: templateVersionID,
				})
			case fmt.Sprintf("/api/v2/organizations/%s/templates/my-template", orgID):
				httpapi.Write(ctx, w, http.StatusOK, codersdk.Template{
					ID:              templateID,
					ActiveVersionID: templateVersionID,
				})
			case fmt.Sprintf("/api/v2/templateversions/%s/presets", templateVersionID):
				httpapi.Write(ctx, w, http.StatusOK, []codersdk.Preset{
					{
						ID:   templateVersionPresetID,
						Name: presetName,
					},
				})
			case "/api/experimental/tasks/me":
				var req codersdk.CreateTaskRequest
				if !httpapi.Read(ctx, w, r, &req) {
					return
				}

				assert.Equal(t, prompt, req.Prompt, "prompt mismatch")
				assert.Equal(t, templateVersionID, req.TemplateVersionID, "template version mismatch")

				if presetName == "" {
					assert.Equal(t, uuid.Nil, req.TemplateVersionPresetID, "expected no template preset id")
				} else {
					assert.Equal(t, templateVersionPresetID, req.TemplateVersionPresetID, "template version preset id mismatch")
				}

				httpapi.Write(ctx, w, http.StatusCreated, codersdk.Workspace{
					Name:      "task-wild-goldfish-27",
					CreatedAt: taskCreatedAt,
				})
			default:
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
		}
	}

	tests := []struct {
		args         []string
		env          []string
		expectError  string
		expectOutput string
		handler      func(t *testing.T, ctx context.Context) http.HandlerFunc
	}{
		{
			args:         []string{"my-template@my-template-version", "--input", "my custom prompt", "--org", organizationID.String()},
			expectOutput: fmt.Sprintf("The task %s has been created at %s!", cliui.Keyword("task-wild-goldfish-27"), cliui.Timestamp(taskCreatedAt)),
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return templateAndVersionFoundHandler(t, ctx, organizationID, "my-template", "my-template-version", "", "my custom prompt")
			},
		},
		{
			args:         []string{"my-template", "--input", "my custom prompt", "--org", organizationID.String()},
			env:          []string{"CODER_TASK_TEMPLATE_VERSION=my-template-version"},
			expectOutput: fmt.Sprintf("The task %s has been created at %s!", cliui.Keyword("task-wild-goldfish-27"), cliui.Timestamp(taskCreatedAt)),
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return templateAndVersionFoundHandler(t, ctx, organizationID, "my-template", "my-template-version", "", "my custom prompt")
			},
		},
		{
			args:         []string{"--input", "my custom prompt", "--org", organizationID.String()},
			env:          []string{"CODER_TASK_TEMPLATE_NAME=my-template", "CODER_TASK_TEMPLATE_VERSION=my-template-version"},
			expectOutput: fmt.Sprintf("The task %s has been created at %s!", cliui.Keyword("task-wild-goldfish-27"), cliui.Timestamp(taskCreatedAt)),
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return templateAndVersionFoundHandler(t, ctx, organizationID, "my-template", "my-template-version", "", "my custom prompt")
			},
		},
		{
			env:          []string{"CODER_TASK_TEMPLATE_NAME=my-template", "CODER_TASK_TEMPLATE_VERSION=my-template-version", "CODER_TASK_INPUT=my custom prompt", "CODER_ORGANIZATION=" + organizationID.String()},
			expectOutput: fmt.Sprintf("The task %s has been created at %s!", cliui.Keyword("task-wild-goldfish-27"), cliui.Timestamp(taskCreatedAt)),
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return templateAndVersionFoundHandler(t, ctx, organizationID, "my-template", "my-template-version", "", "my custom prompt")
			},
		},
		{
			args:         []string{"my-template", "--input", "my custom prompt", "--org", organizationID.String()},
			expectOutput: fmt.Sprintf("The task %s has been created at %s!", cliui.Keyword("task-wild-goldfish-27"), cliui.Timestamp(taskCreatedAt)),
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return templateAndVersionFoundHandler(t, ctx, organizationID, "my-template", "", "", "my custom prompt")
			},
		},
		{
			args:         []string{"my-template", "--input", "my custom prompt", "--preset", "my-preset", "--org", organizationID.String()},
			expectOutput: fmt.Sprintf("The task %s has been created at %s!", cliui.Keyword("task-wild-goldfish-27"), cliui.Timestamp(taskCreatedAt)),
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return templateAndVersionFoundHandler(t, ctx, organizationID, "my-template", "", "my-preset", "my custom prompt")
			},
		},
		{
			args:         []string{"my-template", "--input", "my custom prompt"},
			env:          []string{"CODER_TASK_PRESET_NAME=my-preset"},
			expectOutput: fmt.Sprintf("The task %s has been created at %s!", cliui.Keyword("task-wild-goldfish-27"), cliui.Timestamp(taskCreatedAt)),
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return templateAndVersionFoundHandler(t, ctx, organizationID, "my-template", "", "my-preset", "my custom prompt")
			},
		},
		{
			args:        []string{"my-template", "--input", "my custom prompt", "--preset", "not-real-preset"},
			expectError: `preset "not-real-preset" not found`,
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return templateAndVersionFoundHandler(t, ctx, organizationID, "my-template", "", "my-preset", "my custom prompt")
			},
		},
		{
			args:        []string{"my-template@not-real-template-version", "--input", "my custom prompt"},
			expectError: httpapi.ResourceNotFoundResponse.Message,
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/v2/users/me/organizations":
						httpapi.Write(ctx, w, http.StatusOK, []codersdk.Organization{
							{MinimalOrganization: codersdk.MinimalOrganization{
								ID: organizationID,
							}},
						})
					case fmt.Sprintf("/api/v2/organizations/%s/templates/my-template/versions/not-real-template-version", organizationID):
						httpapi.ResourceNotFound(w)
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			args:        []string{"not-real-template", "--input", "my custom prompt", "--org", organizationID.String()},
			expectError: httpapi.ResourceNotFoundResponse.Message,
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/v2/users/me/organizations":
						httpapi.Write(ctx, w, http.StatusOK, []codersdk.Organization{
							{MinimalOrganization: codersdk.MinimalOrganization{
								ID: organizationID,
							}},
						})
					case fmt.Sprintf("/api/v2/organizations/%s/templates/not-real-template", organizationID):
						httpapi.ResourceNotFound(w)
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			args:        []string{"template-in-different-org", "--input", "my-custom-prompt", "--org", anotherOrganizationID.String()},
			expectError: httpapi.ResourceNotFoundResponse.Message,
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/v2/users/me/organizations":
						httpapi.Write(ctx, w, http.StatusOK, []codersdk.Organization{
							{MinimalOrganization: codersdk.MinimalOrganization{
								ID: anotherOrganizationID,
							}},
						})
					case fmt.Sprintf("/api/v2/organizations/%s/templates/template-in-different-org", anotherOrganizationID):
						httpapi.ResourceNotFound(w)
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			args:        []string{"no-org", "--input", "my-custom-prompt"},
			expectError: "Must select an organization with --org=<org_name>",
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/v2/users/me/organizations":
						httpapi.Write(ctx, w, http.StatusOK, []codersdk.Organization{})
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.args, ","), func(t *testing.T) {
			t.Parallel()

			var (
				ctx    = testutil.Context(t, testutil.WaitShort)
				srv    = httptest.NewServer(tt.handler(t, ctx))
				client = codersdk.New(testutil.MustURL(t, srv.URL))
				args   = []string{"exp", "task", "create"}
				sb     strings.Builder
				err    error
			)

			t.Cleanup(srv.Close)

			inv, root := clitest.New(t, append(args, tt.args...)...)
			inv.Environ = serpent.ParseEnviron(tt.env, "")
			inv.Stdout = &sb
			inv.Stderr = &sb
			clitest.SetupConfig(t, client, root)

			err = inv.WithContext(ctx).Run()
			if tt.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.expectError)
			}

			assert.Contains(t, sb.String(), tt.expectOutput)
		})
	}
}
