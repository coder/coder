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
		taskID                  = uuid.New()
	)

	templateAndVersionFoundHandler := func(t *testing.T, ctx context.Context, orgID uuid.UUID, templateName, templateVersionName, presetName, prompt, taskName, username string) http.HandlerFunc {
		t.Helper()

		return func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/v2/users/me/organizations":
				httpapi.Write(ctx, w, http.StatusOK, []codersdk.Organization{
					{MinimalOrganization: codersdk.MinimalOrganization{
						ID: orgID,
					}},
				})
			case fmt.Sprintf("/api/v2/organizations/%s/templates/%s/versions/%s", orgID, templateName, templateVersionName):
				httpapi.Write(ctx, w, http.StatusOK, codersdk.TemplateVersion{
					ID: templateVersionID,
				})
			case fmt.Sprintf("/api/v2/organizations/%s/templates/%s", orgID, templateName):
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
			case "/api/v2/templates":
				httpapi.Write(ctx, w, http.StatusOK, []codersdk.Template{
					{
						ID:              templateID,
						Name:            templateName,
						ActiveVersionID: templateVersionID,
					},
				})
			case fmt.Sprintf("/api/v2/tasks/%s", username):
				var req codersdk.CreateTaskRequest
				if !httpapi.Read(ctx, w, r, &req) {
					return
				}

				assert.Equal(t, prompt, req.Input, "prompt mismatch")
				assert.Equal(t, templateVersionID, req.TemplateVersionID, "template version mismatch")

				if presetName == "" {
					assert.Equal(t, uuid.Nil, req.TemplateVersionPresetID, "expected no template preset id")
				} else {
					assert.Equal(t, templateVersionPresetID, req.TemplateVersionPresetID, "template version preset id mismatch")
				}

				created := codersdk.Task{
					ID:        taskID,
					Name:      taskName,
					CreatedAt: taskCreatedAt,
				}
				if req.Name != "" {
					assert.Equal(t, req.Name, taskName, "name mismatch")
					created.Name = req.Name
				}

				httpapi.Write(ctx, w, http.StatusCreated, created)
			default:
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
		}
	}

	tests := []struct {
		args         []string
		env          []string
		stdin        string
		expectError  string
		expectOutput string
		handler      func(t *testing.T, ctx context.Context) http.HandlerFunc
	}{
		{
			args:         []string{"--stdin"},
			stdin:        "reads prompt from stdin",
			expectOutput: fmt.Sprintf("The task %s has been created at %s!", cliui.Keyword("task-wild-goldfish-27"), cliui.Timestamp(taskCreatedAt)),
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return templateAndVersionFoundHandler(t, ctx, organizationID, "my-template", "my-template-version", "", "reads prompt from stdin", "task-wild-goldfish-27", codersdk.Me)
			},
		},
		{
			args:         []string{"my custom prompt"},
			expectOutput: fmt.Sprintf("The task %s has been created at %s!", cliui.Keyword("task-wild-goldfish-27"), cliui.Timestamp(taskCreatedAt)),
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return templateAndVersionFoundHandler(t, ctx, organizationID, "my-template", "my-template-version", "", "my custom prompt", "task-wild-goldfish-27", codersdk.Me)
			},
		},
		{
			args:         []string{"my custom prompt", "--owner", "someone-else"},
			expectOutput: fmt.Sprintf("The task %s has been created at %s!", cliui.Keyword("task-wild-goldfish-27"), cliui.Timestamp(taskCreatedAt)),
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return templateAndVersionFoundHandler(t, ctx, organizationID, "my-template", "my-template-version", "", "my custom prompt", "task-wild-goldfish-27", "someone-else")
			},
		},
		{
			args:         []string{"--name", "abc123", "my custom prompt"},
			expectOutput: fmt.Sprintf("The task %s has been created at %s!", cliui.Keyword("abc123"), cliui.Timestamp(taskCreatedAt)),
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return templateAndVersionFoundHandler(t, ctx, organizationID, "my-template", "my-template-version", "", "my custom prompt", "abc123", codersdk.Me)
			},
		},
		{
			args:         []string{"my custom prompt", "--template", "my-template", "--template-version", "my-template-version", "--org", organizationID.String()},
			expectOutput: fmt.Sprintf("The task %s has been created at %s!", cliui.Keyword("task-wild-goldfish-27"), cliui.Timestamp(taskCreatedAt)),
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return templateAndVersionFoundHandler(t, ctx, organizationID, "my-template", "my-template-version", "", "my custom prompt", "task-wild-goldfish-27", codersdk.Me)
			},
		},
		{
			args:         []string{"my custom prompt", "--template", "my-template", "--org", organizationID.String()},
			env:          []string{"CODER_TASK_TEMPLATE_VERSION=my-template-version"},
			expectOutput: fmt.Sprintf("The task %s has been created at %s!", cliui.Keyword("task-wild-goldfish-27"), cliui.Timestamp(taskCreatedAt)),
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return templateAndVersionFoundHandler(t, ctx, organizationID, "my-template", "my-template-version", "", "my custom prompt", "task-wild-goldfish-27", codersdk.Me)
			},
		},
		{
			args:         []string{"my custom prompt", "--org", organizationID.String()},
			env:          []string{"CODER_TASK_TEMPLATE_NAME=my-template", "CODER_TASK_TEMPLATE_VERSION=my-template-version"},
			expectOutput: fmt.Sprintf("The task %s has been created at %s!", cliui.Keyword("task-wild-goldfish-27"), cliui.Timestamp(taskCreatedAt)),
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return templateAndVersionFoundHandler(t, ctx, organizationID, "my-template", "my-template-version", "", "my custom prompt", "task-wild-goldfish-27", codersdk.Me)
			},
		},
		{
			args:         []string{"my custom prompt", "--template", "my-template", "--org", organizationID.String()},
			expectOutput: fmt.Sprintf("The task %s has been created at %s!", cliui.Keyword("task-wild-goldfish-27"), cliui.Timestamp(taskCreatedAt)),
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return templateAndVersionFoundHandler(t, ctx, organizationID, "my-template", "", "", "my custom prompt", "task-wild-goldfish-27", codersdk.Me)
			},
		},
		{
			args:         []string{"my custom prompt", "--template", "my-template", "--preset", "my-preset", "--org", organizationID.String()},
			expectOutput: fmt.Sprintf("The task %s has been created at %s!", cliui.Keyword("task-wild-goldfish-27"), cliui.Timestamp(taskCreatedAt)),
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return templateAndVersionFoundHandler(t, ctx, organizationID, "my-template", "", "my-preset", "my custom prompt", "task-wild-goldfish-27", codersdk.Me)
			},
		},
		{
			args:         []string{"my custom prompt", "--template", "my-template"},
			env:          []string{"CODER_TASK_PRESET_NAME=my-preset"},
			expectOutput: fmt.Sprintf("The task %s has been created at %s!", cliui.Keyword("task-wild-goldfish-27"), cliui.Timestamp(taskCreatedAt)),
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return templateAndVersionFoundHandler(t, ctx, organizationID, "my-template", "", "my-preset", "my custom prompt", "task-wild-goldfish-27", codersdk.Me)
			},
		},
		{
			args:         []string{"my custom prompt", "-q"},
			expectOutput: taskID.String(),
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return templateAndVersionFoundHandler(t, ctx, organizationID, "my-template", "my-template-version", "", "my custom prompt", "task-wild-goldfish-27", codersdk.Me)
			},
		},
		{
			args:        []string{"my custom prompt", "--template", "my-template", "--preset", "not-real-preset"},
			expectError: `preset "not-real-preset" not found`,
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return templateAndVersionFoundHandler(t, ctx, organizationID, "my-template", "", "my-preset", "my custom prompt", "task-wild-goldfish-27", codersdk.Me)
			},
		},
		{
			args:        []string{"my custom prompt", "--template", "my-template", "--template-version", "not-real-template-version"},
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
					case fmt.Sprintf("/api/v2/organizations/%s/templates/my-template", organizationID):
						httpapi.Write(ctx, w, http.StatusOK, codersdk.Template{
							ID:              templateID,
							ActiveVersionID: templateVersionID,
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
			args:        []string{"my custom prompt", "--template", "not-real-template", "--org", organizationID.String()},
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
			args:        []string{"my-custom-prompt", "--template", "template-in-different-org", "--org", anotherOrganizationID.String()},
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
			args:        []string{"no-org-prompt"},
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
		{
			args:        []string{"no task templates"},
			expectError: "no task templates configured",
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/v2/users/me/organizations":
						httpapi.Write(ctx, w, http.StatusOK, []codersdk.Organization{
							{MinimalOrganization: codersdk.MinimalOrganization{
								ID: organizationID,
							}},
						})
					case "/api/v2/templates":
						httpapi.Write(ctx, w, http.StatusOK, []codersdk.Template{})
					default:
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
				}
			},
		},
		{
			args:        []string{"no template name provided"},
			expectError: "template name not provided, available templates: wibble, wobble",
			handler: func(t *testing.T, ctx context.Context) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/v2/users/me/organizations":
						httpapi.Write(ctx, w, http.StatusOK, []codersdk.Organization{
							{MinimalOrganization: codersdk.MinimalOrganization{
								ID: organizationID,
							}},
						})
					case "/api/v2/templates":
						httpapi.Write(ctx, w, http.StatusOK, []codersdk.Template{
							{Name: "wibble"},
							{Name: "wobble"},
						})
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
				args   = []string{"task", "create"}
				sb     strings.Builder
				err    error
			)

			t.Cleanup(srv.Close)

			inv, root := clitest.New(t, append(args, tt.args...)...)
			inv.Environ = serpent.ParseEnviron(tt.env, "")
			inv.Stdin = strings.NewReader(tt.stdin)
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
