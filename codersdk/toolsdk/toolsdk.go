package toolsdk

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// Tool name constants to avoid hardcoded strings
const (
	ToolNameReportTask                  = "coder_report_task"
	ToolNameGetWorkspace                = "coder_get_workspace"
	ToolNameCreateWorkspace             = "coder_create_workspace"
	ToolNameListWorkspaces              = "coder_list_workspaces"
	ToolNameListTemplates               = "coder_list_templates"
	ToolNameListTemplateVersionParams   = "coder_template_version_parameters"
	ToolNameGetAuthenticatedUser        = "coder_get_authenticated_user"
	ToolNameCreateWorkspaceBuild        = "coder_create_workspace_build"
	ToolNameCreateTemplateVersion       = "coder_create_template_version"
	ToolNameGetWorkspaceAgentLogs       = "coder_get_workspace_agent_logs"
	ToolNameGetWorkspaceBuildLogs       = "coder_get_workspace_build_logs"
	ToolNameGetTemplateVersionLogs      = "coder_get_template_version_logs"
	ToolNameUpdateTemplateActiveVersion = "coder_update_template_active_version"
	ToolNameUploadTarFile               = "coder_upload_tar_file"
	ToolNameCreateTemplate              = "coder_create_template"
	ToolNameDeleteTemplate              = "coder_delete_template"
	ToolNameWorkspaceBash               = "coder_workspace_bash"
	ToolNameChatGPTSearch               = "search"
	ToolNameChatGPTFetch                = "fetch"
	ToolNameWorkspaceLS                 = "coder_workspace_ls"
	ToolNameWorkspaceReadFile           = "coder_workspace_read_file"
	ToolNameWorkspaceWriteFile          = "coder_workspace_write_file"
	ToolNameWorkspaceEditFile           = "coder_workspace_edit_file"
	ToolNameWorkspaceEditFiles          = "coder_workspace_edit_files"
	ToolNameWorkspacePortForward        = "coder_workspace_port_forward"
	ToolNameWorkspaceListApps           = "coder_workspace_list_apps"
	ToolNameCreateTask                  = "coder_create_task"
	ToolNameDeleteTask                  = "coder_delete_task"
	ToolNameListTasks                   = "coder_list_tasks"
	ToolNameGetTaskStatus               = "coder_get_task_status"
	ToolNameSendTaskInput               = "coder_send_task_input"
	ToolNameGetTaskLogs                 = "coder_get_task_logs"
)

func NewDeps(client *codersdk.Client, opts ...func(*Deps)) (Deps, error) {
	d := Deps{
		coderClient: client,
	}
	for _, opt := range opts {
		opt(&d)
	}
	// Allow nil client for unauthenticated operation
	// This enables tools that don't require user authentication to function
	return d, nil
}

// Deps provides access to tool dependencies.
type Deps struct {
	coderClient *codersdk.Client
	report      func(ReportTaskArgs) error
}

func (d Deps) ServerURL() string {
	serverURLCopy := *d.coderClient.URL
	serverURLCopy.Path = ""
	serverURLCopy.RawQuery = ""
	return serverURLCopy.String()
}

func WithTaskReporter(fn func(ReportTaskArgs) error) func(*Deps) {
	return func(d *Deps) {
		d.report = fn
	}
}

// HandlerFunc is a typed function that handles a tool call.
type HandlerFunc[Arg, Ret any] func(context.Context, Deps, Arg) (Ret, error)

// Tool consists of an aisdk.Tool and a corresponding typed handler function.
type Tool[Arg, Ret any] struct {
	aisdk.Tool
	Handler HandlerFunc[Arg, Ret]

	// UserClientOptional indicates whether this tool can function without a valid
	// user authentication token. If true, the tool will be available even when
	// running in an unauthenticated mode with just an agent token.
	UserClientOptional bool
}

// Generic returns a type-erased version of a TypedTool where the arguments and
// return values are converted to/from json.RawMessage.
// This allows the tool to be referenced without knowing the concrete arguments
// or return values. The original TypedHandlerFunc is wrapped to handle type
// conversion.
func (t Tool[Arg, Ret]) Generic() GenericTool {
	return GenericTool{
		Tool:               t.Tool,
		UserClientOptional: t.UserClientOptional,
		Handler: wrap(func(ctx context.Context, deps Deps, args json.RawMessage) (json.RawMessage, error) {
			var typedArgs Arg
			if err := json.Unmarshal(args, &typedArgs); err != nil {
				return nil, xerrors.Errorf("failed to unmarshal args: %w", err)
			}
			ret, err := t.Handler(ctx, deps, typedArgs)
			var buf bytes.Buffer
			if err := json.NewEncoder(&buf).Encode(ret); err != nil {
				return json.RawMessage{}, err
			}
			return buf.Bytes(), err
		}, WithCleanContext, WithRecover),
	}
}

// GenericTool is a type-erased wrapper for GenericTool.
// This allows referencing the tool without knowing the concrete argument or
// return type. The Handler function allows calling the tool with known types.
type GenericTool struct {
	aisdk.Tool
	Handler GenericHandlerFunc

	// UserClientOptional indicates whether this tool can function without a valid
	// user authentication token. If true, the tool will be available even when
	// running in an unauthenticated mode with just an agent token.
	UserClientOptional bool
}

// GenericHandlerFunc is a function that handles a tool call.
type GenericHandlerFunc func(context.Context, Deps, json.RawMessage) (json.RawMessage, error)

// NoArgs just represents an empty argument struct.
type NoArgs struct{}

// WithRecover wraps a HandlerFunc to recover from panics and return an error.
func WithRecover(h GenericHandlerFunc) GenericHandlerFunc {
	return func(ctx context.Context, deps Deps, args json.RawMessage) (ret json.RawMessage, err error) {
		defer func() {
			if r := recover(); r != nil {
				if buildinfo.IsDev() {
					// Capture stack trace in dev builds
					stack := debug.Stack()
					err = xerrors.Errorf("tool handler panic: %v\nstack trace:\n%s", r, stack)
				} else {
					// Simple error message in production builds
					err = xerrors.Errorf("tool handler panic: %v", r)
				}
			}
		}()
		return h(ctx, deps, args)
	}
}

// WithCleanContext wraps a HandlerFunc to provide it with a new context.
// This ensures that no data is passed using context.Value.
// If a deadline is set on the parent context, it will be passed to the child
// context.
func WithCleanContext(h GenericHandlerFunc) GenericHandlerFunc {
	return func(parent context.Context, deps Deps, args json.RawMessage) (ret json.RawMessage, err error) {
		child, childCancel := context.WithCancel(context.Background())
		defer childCancel()
		// Ensure that the child context has the same deadline as the parent
		// context.
		if deadline, ok := parent.Deadline(); ok {
			deadlineCtx, deadlineCancel := context.WithDeadline(child, deadline)
			defer deadlineCancel()
			child = deadlineCtx
		}
		// Ensure that cancellation propagates from the parent context to the child context.
		go func() {
			select {
			case <-child.Done():
				return
			case <-parent.Done():
				childCancel()
			}
		}()
		return h(child, deps, args)
	}
}

// wrap wraps the provided GenericHandlerFunc with the provided middleware functions.
func wrap(hf GenericHandlerFunc, mw ...func(GenericHandlerFunc) GenericHandlerFunc) GenericHandlerFunc {
	for _, m := range mw {
		hf = m(hf)
	}
	return hf
}

// All is a list of all tools that can be used in the Coder CLI.
// When you add a new tool, be sure to include it here!
var All = []GenericTool{
	CreateTemplate.Generic(),
	CreateTemplateVersion.Generic(),
	CreateWorkspace.Generic(),
	CreateWorkspaceBuild.Generic(),
	DeleteTemplate.Generic(),
	ListTemplates.Generic(),
	ListTemplateVersionParameters.Generic(),
	ListWorkspaces.Generic(),
	GetAuthenticatedUser.Generic(),
	GetTemplateVersionLogs.Generic(),
	GetWorkspace.Generic(),
	GetWorkspaceAgentLogs.Generic(),
	GetWorkspaceBuildLogs.Generic(),
	ReportTask.Generic(),
	UploadTarFile.Generic(),
	UpdateTemplateActiveVersion.Generic(),
	WorkspaceBash.Generic(),
	ChatGPTSearch.Generic(),
	ChatGPTFetch.Generic(),
	WorkspaceLS.Generic(),
	WorkspaceReadFile.Generic(),
	WorkspaceWriteFile.Generic(),
	WorkspaceEditFile.Generic(),
	WorkspaceEditFiles.Generic(),
	WorkspacePortForward.Generic(),
	WorkspaceListApps.Generic(),
	CreateTask.Generic(),
	DeleteTask.Generic(),
	ListTasks.Generic(),
	GetTaskStatus.Generic(),
	SendTaskInput.Generic(),
	GetTaskLogs.Generic(),
}

type ReportTaskArgs struct {
	Link    string `json:"link"`
	State   string `json:"state"`
	Summary string `json:"summary"`
}

var ReportTask = Tool[ReportTaskArgs, codersdk.Response]{
	Tool: aisdk.Tool{
		Name: ToolNameReportTask,
		Description: `Report progress on your work.

The user observes your work through a Task UI. To keep them updated
on your progress, or if you need help - use this tool.

Good Tasks
- "Cloning the repository <repository-url>"
- "Working on <feature-name>"
- "Figuring our why <issue> is happening"

Bad Tasks
- "I'm working on it"
- "I'm trying to fix it"
- "I'm trying to implement <feature-name>"

Use the "state" field to indicate your progress. Periodically report
progress with state "working" to keep the user updated. It is not possible to send too many updates!

ONLY report an "idle" or "failure" state if you have FULLY completed the task.
`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"summary": map[string]any{
					"type":        "string",
					"description": "A concise summary of your current progress on the task. This must be less than 160 characters in length and must not include newlines or other control characters.",
				},
				"link": map[string]any{
					"type":        "string",
					"description": "A link to a relevant resource, such as a PR or issue.",
				},
				"state": map[string]any{
					"type":        "string",
					"description": "The state of your task. This can be one of the following: working, idle, or failure. Select the state that best represents your current progress.",
					"enum": []string{
						string(codersdk.WorkspaceAppStatusStateWorking),
						string(codersdk.WorkspaceAppStatusStateIdle),
						string(codersdk.WorkspaceAppStatusStateFailure),
					},
				},
			},
			Required: []string{"summary", "link", "state"},
		},
	},
	UserClientOptional: true,
	Handler: func(_ context.Context, deps Deps, args ReportTaskArgs) (codersdk.Response, error) {
		if len(args.Summary) > 160 {
			return codersdk.Response{}, xerrors.New("summary must be less than 160 characters")
		}
		// Check if task reporting is available to prevent nil pointer dereference
		if deps.report == nil {
			return codersdk.Response{}, xerrors.New("task reporting not available. Please ensure a task reporter is configured.")
		}
		err := deps.report(args)
		if err != nil {
			return codersdk.Response{}, err
		}
		return codersdk.Response{
			Message: "Thanks for reporting!",
		}, nil
	},
}

type GetWorkspaceArgs struct {
	WorkspaceID string `json:"workspace_id"`
}

var GetWorkspace = Tool[GetWorkspaceArgs, codersdk.Workspace]{
	Tool: aisdk.Tool{
		Name: ToolNameGetWorkspace,
		Description: `Get a workspace by name or ID.

This returns more data than list_workspaces to reduce token usage.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"workspace_id": map[string]any{
					"type":        "string",
					"description": workspaceDescription,
				},
			},
			Required: []string{"workspace_id"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args GetWorkspaceArgs) (codersdk.Workspace, error) {
		wsID, err := uuid.Parse(args.WorkspaceID)
		if err != nil {
			return namedWorkspace(ctx, deps.coderClient, NormalizeWorkspaceInput(args.WorkspaceID))
		}
		return deps.coderClient.Workspace(ctx, wsID)
	},
}

type CreateWorkspaceArgs struct {
	Name              string            `json:"name"`
	RichParameters    map[string]string `json:"rich_parameters"`
	TemplateVersionID string            `json:"template_version_id"`
	User              string            `json:"user"`
}

var CreateWorkspace = Tool[CreateWorkspaceArgs, codersdk.Workspace]{
	Tool: aisdk.Tool{
		Name: ToolNameCreateWorkspace,
		Description: `Create a new workspace in Coder.

If a user is asking to "test a template", they are typically referring
to creating a workspace from a template to ensure the infrastructure
is provisioned correctly and the agent can connect to the control plane.

Before creating a workspace, always confirm the template choice with the user by:

	1. Listing the available templates that match their request.
	2. Recommending the most relevant option.
	2. Asking the user to confirm which template to use.

It is important to not create a workspace without confirming the template
choice with the user.

After creating a workspace, watch the build logs and wait for the workspace to
be ready before trying to use or connect to the workspace.
`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"user": map[string]any{
					"type":        "string",
					"description": userDescription("create a workspace"),
				},
				"template_version_id": map[string]any{
					"type":        "string",
					"description": "ID of the template version to create the workspace from.",
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Name of the workspace to create.",
				},
				"rich_parameters": map[string]any{
					"type":        "object",
					"description": "Key/value pairs of rich parameters to pass to the template version to create the workspace.",
				},
			},
			Required: []string{"user", "template_version_id", "name", "rich_parameters"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args CreateWorkspaceArgs) (codersdk.Workspace, error) {
		tvID, err := uuid.Parse(args.TemplateVersionID)
		if err != nil {
			return codersdk.Workspace{}, xerrors.New("template_version_id must be a valid UUID")
		}
		if args.User == "" {
			args.User = codersdk.Me
		}
		var buildParams []codersdk.WorkspaceBuildParameter
		for k, v := range args.RichParameters {
			buildParams = append(buildParams, codersdk.WorkspaceBuildParameter{
				Name:  k,
				Value: v,
			})
		}
		workspace, err := deps.coderClient.CreateUserWorkspace(ctx, args.User, codersdk.CreateWorkspaceRequest{
			TemplateVersionID:   tvID,
			Name:                args.Name,
			RichParameterValues: buildParams,
		})
		if err != nil {
			return codersdk.Workspace{}, err
		}
		return workspace, nil
	},
}

type ListWorkspacesArgs struct {
	Owner string `json:"owner"`
}

var ListWorkspaces = Tool[ListWorkspacesArgs, []MinimalWorkspace]{
	Tool: aisdk.Tool{
		Name:        ToolNameListWorkspaces,
		Description: "Lists workspaces for the authenticated user.",
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"owner": map[string]any{
					"type":        "string",
					"description": "The owner of the workspaces to list. Use \"me\" to list workspaces for the authenticated user. If you do not specify an owner, \"me\" will be assumed by default.",
				},
			},
			Required: []string{},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args ListWorkspacesArgs) ([]MinimalWorkspace, error) {
		owner := args.Owner
		if owner == "" {
			owner = codersdk.Me
		}
		workspaces, err := deps.coderClient.Workspaces(ctx, codersdk.WorkspaceFilter{
			Owner: owner,
		})
		if err != nil {
			return nil, err
		}
		minimalWorkspaces := make([]MinimalWorkspace, len(workspaces.Workspaces))
		for i, workspace := range workspaces.Workspaces {
			minimalWorkspaces[i] = MinimalWorkspace{
				ID:                      workspace.ID.String(),
				Name:                    workspace.Name,
				TemplateID:              workspace.TemplateID.String(),
				TemplateName:            workspace.TemplateName,
				TemplateDisplayName:     workspace.TemplateDisplayName,
				TemplateIcon:            workspace.TemplateIcon,
				TemplateActiveVersionID: workspace.TemplateActiveVersionID,
				Outdated:                workspace.Outdated,
			}
		}
		return minimalWorkspaces, nil
	},
}

var ListTemplates = Tool[NoArgs, []MinimalTemplate]{
	Tool: aisdk.Tool{
		Name:        ToolNameListTemplates,
		Description: "Lists templates for the authenticated user.",
		Schema: aisdk.Schema{
			Properties: map[string]any{},
			Required:   []string{},
		},
	},
	Handler: func(ctx context.Context, deps Deps, _ NoArgs) ([]MinimalTemplate, error) {
		templates, err := deps.coderClient.Templates(ctx, codersdk.TemplateFilter{})
		if err != nil {
			return nil, err
		}
		minimalTemplates := make([]MinimalTemplate, len(templates))
		for i, template := range templates {
			minimalTemplates[i] = MinimalTemplate{
				DisplayName:     template.DisplayName,
				ID:              template.ID.String(),
				Name:            template.Name,
				Description:     template.Description,
				ActiveVersionID: template.ActiveVersionID,
				ActiveUserCount: template.ActiveUserCount,
			}
		}
		return minimalTemplates, nil
	},
}

type ListTemplateVersionParametersArgs struct {
	TemplateVersionID string `json:"template_version_id"`
}

var ListTemplateVersionParameters = Tool[ListTemplateVersionParametersArgs, []codersdk.TemplateVersionParameter]{
	Tool: aisdk.Tool{
		Name:        ToolNameListTemplateVersionParams,
		Description: "Get the parameters for a template version. You can refer to these as workspace parameters to the user, as they are typically important for creating a workspace.",
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"template_version_id": map[string]any{
					"type": "string",
				},
			},
			Required: []string{"template_version_id"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args ListTemplateVersionParametersArgs) ([]codersdk.TemplateVersionParameter, error) {
		templateVersionID, err := uuid.Parse(args.TemplateVersionID)
		if err != nil {
			return nil, xerrors.Errorf("template_version_id must be a valid UUID: %w", err)
		}
		parameters, err := deps.coderClient.TemplateVersionRichParameters(ctx, templateVersionID)
		if err != nil {
			return nil, err
		}
		return parameters, nil
	},
}

var GetAuthenticatedUser = Tool[NoArgs, codersdk.User]{
	Tool: aisdk.Tool{
		Name:        ToolNameGetAuthenticatedUser,
		Description: "Get the currently authenticated user, similar to the `whoami` command.",
		Schema: aisdk.Schema{
			Properties: map[string]any{},
			Required:   []string{},
		},
	},
	Handler: func(ctx context.Context, deps Deps, _ NoArgs) (codersdk.User, error) {
		return deps.coderClient.User(ctx, "me")
	},
}

type CreateWorkspaceBuildArgs struct {
	TemplateVersionID string `json:"template_version_id"`
	Transition        string `json:"transition"`
	WorkspaceID       string `json:"workspace_id"`
}

var CreateWorkspaceBuild = Tool[CreateWorkspaceBuildArgs, codersdk.WorkspaceBuild]{
	Tool: aisdk.Tool{
		Name: ToolNameCreateWorkspaceBuild,
		Description: `Create a new workspace build for an existing workspace. Use this to start, stop, or delete.

After creating a workspace build, watch the build logs and wait for the
workspace build to complete before trying to start another build or use or
connect to the workspace.
`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"workspace_id": map[string]any{
					"type": "string",
				},
				"transition": map[string]any{
					"type":        "string",
					"description": "The transition to perform. Must be one of: start, stop, delete",
					"enum":        []string{"start", "stop", "delete"},
				},
				"template_version_id": map[string]any{
					"type":        "string",
					"description": "(Optional) The template version ID to use for the workspace build. If not provided, the previously built version will be used.",
				},
			},
			Required: []string{"workspace_id", "transition"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args CreateWorkspaceBuildArgs) (codersdk.WorkspaceBuild, error) {
		workspaceID, err := uuid.Parse(args.WorkspaceID)
		if err != nil {
			return codersdk.WorkspaceBuild{}, xerrors.Errorf("workspace_id must be a valid UUID: %w", err)
		}
		var templateVersionID uuid.UUID
		if args.TemplateVersionID != "" {
			tvID, err := uuid.Parse(args.TemplateVersionID)
			if err != nil {
				return codersdk.WorkspaceBuild{}, xerrors.Errorf("template_version_id must be a valid UUID: %w", err)
			}
			templateVersionID = tvID
		}
		cbr := codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransition(args.Transition),
		}
		if templateVersionID != uuid.Nil {
			cbr.TemplateVersionID = templateVersionID
		}
		return deps.coderClient.CreateWorkspaceBuild(ctx, workspaceID, cbr)
	},
}

type CreateTemplateVersionArgs struct {
	FileID     string `json:"file_id"`
	TemplateID string `json:"template_id"`
}

var CreateTemplateVersion = Tool[CreateTemplateVersionArgs, codersdk.TemplateVersion]{
	Tool: aisdk.Tool{
		Name: ToolNameCreateTemplateVersion,
		Description: `Create a new template version. This is a precursor to creating a template, or you can update an existing template.

Templates are Terraform defining a development environment. The provisioned infrastructure must run
an Agent that connects to the Coder Control Plane to provide a rich experience.

Here are some strict rules for creating a template version:
- YOU MUST NOT use "variable" or "output" blocks in the Terraform code.
- YOU MUST ALWAYS check template version logs after creation to ensure the template was imported successfully.

When a template version is created, a Terraform Plan occurs that ensures the infrastructure
_could_ be provisioned, but actual provisioning occurs when a workspace is created.

<terraform-spec>
The Coder Terraform Provider can be imported like:

` + "```" + `hcl
terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}
` + "```" + `

A destroy does not occur when a user stops a workspace, but rather the transition changes:

` + "```" + `hcl
data "coder_workspace" "me" {}
` + "```" + `

This data source provides the following fields:
- id: The UUID of the workspace.
- name: The name of the workspace.
- transition: Either "start" or "stop".
- start_count: A computed count based on the transition field. If "start", this will be 1.

Access workspace owner information with:

` + "```" + `hcl
data "coder_workspace_owner" "me" {}
` + "```" + `

This data source provides the following fields:
- id: The UUID of the workspace owner.
- name: The name of the workspace owner.
- full_name: The full name of the workspace owner.
- email: The email of the workspace owner.
- session_token: A token that can be used to authenticate the workspace owner. It is regenerated every time the workspace is started.
- oidc_access_token: A valid OpenID Connect access token of the workspace owner. This is only available if the workspace owner authenticated with OpenID Connect. If a valid token cannot be obtained, this value will be an empty string.

Parameters are defined in the template version. They are rendered in the UI on the workspace creation page:

` + "```" + `hcl
resource "coder_parameter" "region" {
  name = "region"
  type = "string"
  default = "us-east-1"
}
` + "```" + `

This resource accepts the following properties:
- name: The name of the parameter.
- default: The default value of the parameter.
- type: The type of the parameter. Must be one of: "string", "number", "bool", or "list(string)".
- display_name: The displayed name of the parameter as it will appear in the UI.
- description: The description of the parameter as it will appear in the UI.
- ephemeral: The value of an ephemeral parameter will not be preserved between consecutive workspace builds.
- form_type: The type of this parameter. Must be one of: [radio, slider, input, dropdown, checkbox, switch, multi-select, tag-select, textarea, error].
- icon: A URL to an icon to display in the UI.
- mutable: Whether this value can be changed after workspace creation. This can be destructive for values like region, so use with caution!
- option: Each option block defines a value for a user to select from. (see below for nested schema)
  Required:
  - name: The name of the option.
  - value: The value of the option.
  Optional:
  - description: The description of the option as it will appear in the UI.
  - icon: A URL to an icon to display in the UI.

A Workspace Agent runs on provisioned infrastructure to provide access to the workspace:

` + "```" + `hcl
resource "coder_agent" "dev" {
  arch = "amd64"
  os = "linux"
}
` + "```" + `

This resource accepts the following properties:
- arch: The architecture of the agent. Must be one of: "amd64", "arm64", or "armv7".
- os: The operating system of the agent. Must be one of: "linux", "windows", or "darwin".
- auth: The authentication method for the agent. Must be one of: "token", "google-instance-identity", "aws-instance-identity", or "azure-instance-identity". It is insecure to pass the agent token via exposed variables to Virtual Machines. Instance Identity enables provisioned VMs to authenticate by instance ID on start.
- dir: The starting directory when a user creates a shell session. Defaults to "$HOME".
- env: A map of environment variables to set for the agent.
- startup_script: A script to run after the agent starts. This script MUST exit eventually to signal that startup has completed. Use "&" or "screen" to run processes in the background.

This resource provides the following fields:
- id: The UUID of the agent.
- init_script: The script to run on provisioned infrastructure to fetch and start the agent.
- token: Set the environment variable CODER_AGENT_TOKEN to this value to authenticate the agent.

The agent MUST be installed and started using the init_script. A utility like curl or wget to fetch the agent binary must exist in the provisioned infrastructure.

Expose terminal or HTTP applications running in a workspace with:

` + "```" + `hcl
resource "coder_app" "dev" {
  agent_id = coder_agent.dev.id
  slug = "my-app-name"
  display_name = "My App"
  icon = "https://my-app.com/icon.svg"
  url = "http://127.0.0.1:3000"
}
` + "```" + `

This resource accepts the following properties:
- agent_id: The ID of the agent to attach the app to.
- slug: The slug of the app.
- display_name: The displayed name of the app as it will appear in the UI.
- icon: A URL to an icon to display in the UI.
- url: An external url if external=true or a URL to be proxied to from inside the workspace. This should be of the form http://localhost:PORT[/SUBPATH]. Either command or url may be specified, but not both.
- command: A command to run in a terminal opening this app. In the web, this will open in a new tab. In the CLI, this will SSH and execute the command. Either command or url may be specified, but not both.
- external: Whether this app is an external app. If true, the url will be opened in a new tab.
</terraform-spec>

The Coder Server may not be authenticated with the infrastructure provider a user requests. In this scenario,
the user will need to provide credentials to the Coder Server before the workspace can be provisioned.

Here are examples of provisioning the Coder Agent on specific infrastructure providers:

<aws-ec2-instance>
// The agent is configured with "aws-instance-identity" auth.
terraform {
  required_providers {
    cloudinit = {
      source = "hashicorp/cloudinit"
    }
    aws = {
      source = "hashicorp/aws"
    }
  }
}

data "cloudinit_config" "user_data" {
  gzip          = false
  base64_encode = false
  boundary = "//"
  part {
    filename     = "cloud-config.yaml"
    content_type = "text/cloud-config"

	// Here is the content of the cloud-config.yaml.tftpl file:
	// #cloud-config
	// cloud_final_modules:
	//   - [scripts-user, always]
	// hostname: ${hostname}
	// users:
	//   - name: ${linux_user}
	//     sudo: ALL=(ALL) NOPASSWD:ALL
	//     shell: /bin/bash
    content = templatefile("${path.module}/cloud-init/cloud-config.yaml.tftpl", {
      hostname   = local.hostname
      linux_user = local.linux_user
    })
  }

  part {
    filename     = "userdata.sh"
    content_type = "text/x-shellscript"

	// Here is the content of the userdata.sh.tftpl file:
	// #!/bin/bash
	// sudo -u '${linux_user}' sh -c '${init_script}'
    content = templatefile("${path.module}/cloud-init/userdata.sh.tftpl", {
      linux_user = local.linux_user

      init_script = try(coder_agent.dev[0].init_script, "")
    })
  }
}

resource "aws_instance" "dev" {
  ami               = data.aws_ami.ubuntu.id
  availability_zone = "${data.coder_parameter.region.value}a"
  instance_type     = data.coder_parameter.instance_type.value

  user_data = data.cloudinit_config.user_data.rendered
  tags = {
    Name = "coder-${data.coder_workspace_owner.me.name}-${data.coder_workspace.me.name}"
  }
  lifecycle {
    ignore_changes = [ami]
  }
}
</aws-ec2-instance>

<gcp-vm-instance>
// The agent is configured with "google-instance-identity" auth.
terraform {
  required_providers {
    google = {
      source = "hashicorp/google"
    }
  }
}

resource "google_compute_instance" "dev" {
  zone         = module.gcp_region.value
  count        = data.coder_workspace.me.start_count
  name         = "coder-${lower(data.coder_workspace_owner.me.name)}-${lower(data.coder_workspace.me.name)}-root"
  machine_type = "e2-medium"
  network_interface {
    network = "default"
    access_config {
      // Ephemeral public IP
    }
  }
  boot_disk {
    auto_delete = false
    source      = google_compute_disk.root.name
  }
  // In order to use google-instance-identity, a service account *must* be provided.
  service_account {
    email  = data.google_compute_default_service_account.default.email
    scopes = ["cloud-platform"]
  }
  # ONLY FOR WINDOWS:
  # metadata = {
  #   windows-startup-script-ps1 = coder_agent.main.init_script
  # }
  # The startup script runs as root with no $HOME environment set up, so instead of directly
  # running the agent init script, create a user (with a homedir, default shell and sudo
  # permissions) and execute the init script as that user.
  #
  # The agent MUST be started in here.
  metadata_startup_script = <<EOMETA
#!/usr/bin/env sh
set -eux

# If user does not exist, create it and set up passwordless sudo
if ! id -u "${local.linux_user}" >/dev/null 2>&1; then
  useradd -m -s /bin/bash "${local.linux_user}"
  echo "${local.linux_user} ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/coder-user
fi

exec sudo -u "${local.linux_user}" sh -c '${coder_agent.main.init_script}'
EOMETA
}
</gcp-vm-instance>

<azure-vm-instance>
// The agent is configured with "azure-instance-identity" auth.
terraform {
  required_providers {
    azurerm = {
      source = "hashicorp/azurerm"
    }
    cloudinit = {
      source = "hashicorp/cloudinit"
    }
  }
}

data "cloudinit_config" "user_data" {
  gzip          = false
  base64_encode = true

  boundary = "//"

  part {
    filename     = "cloud-config.yaml"
    content_type = "text/cloud-config"

	// Here is the content of the cloud-config.yaml.tftpl file:
	// #cloud-config
	// cloud_final_modules:
	// - [scripts-user, always]
	// bootcmd:
	//   # work around https://github.com/hashicorp/terraform-provider-azurerm/issues/6117
	//   - until [ -e /dev/disk/azure/scsi1/lun10 ]; do sleep 1; done
	// device_aliases:
	//   homedir: /dev/disk/azure/scsi1/lun10
	// disk_setup:
	//   homedir:
	//     table_type: gpt
	//     layout: true
	// fs_setup:
	//   - label: coder_home
	//     filesystem: ext4
	//     device: homedir.1
	// mounts:
	//   - ["LABEL=coder_home", "/home/${username}"]
	// hostname: ${hostname}
	// users:
	//   - name: ${username}
	//     sudo: ["ALL=(ALL) NOPASSWD:ALL"]
	//     groups: sudo
	//     shell: /bin/bash
	// packages:
	//   - git
	// write_files:
	//   - path: /opt/coder/init
	//     permissions: "0755"
	//     encoding: b64
	//     content: ${init_script}
	//   - path: /etc/systemd/system/coder-agent.service
	//     permissions: "0644"
	//     content: |
	//       [Unit]
	//       Description=Coder Agent
	//       After=network-online.target
	//       Wants=network-online.target

	//       [Service]
	//       User=${username}
	//       ExecStart=/opt/coder/init
	//       Restart=always
	//       RestartSec=10
	//       TimeoutStopSec=90
	//       KillMode=process

	//       OOMScoreAdjust=-900
	//       SyslogIdentifier=coder-agent

	//       [Install]
	//       WantedBy=multi-user.target
	// runcmd:
	//   - chown ${username}:${username} /home/${username}
	//   - systemctl enable coder-agent
	//   - systemctl start coder-agent
    content = templatefile("${path.module}/cloud-init/cloud-config.yaml.tftpl", {
      username    = "coder" # Ensure this user/group does not exist in your VM image
      init_script = base64encode(coder_agent.main.init_script)
      hostname    = lower(data.coder_workspace.me.name)
    })
  }
}

resource "azurerm_linux_virtual_machine" "main" {
  count               = data.coder_workspace.me.start_count
  name                = "vm"
  resource_group_name = azurerm_resource_group.main.name
  location            = azurerm_resource_group.main.location
  size                = data.coder_parameter.instance_type.value
  // cloud-init overwrites this, so the value here doesn't matter
  admin_username = "adminuser"
  admin_ssh_key {
    public_key = tls_private_key.dummy.public_key_openssh
    username   = "adminuser"
  }

  network_interface_ids = [
    azurerm_network_interface.main.id,
  ]
  computer_name = lower(data.coder_workspace.me.name)
  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "Standard_LRS"
  }
  source_image_reference {
    publisher = "Canonical"
    offer     = "0001-com-ubuntu-server-focal"
    sku       = "20_04-lts-gen2"
    version   = "latest"
  }
  user_data = data.cloudinit_config.user_data.rendered
}
</azure-vm-instance>

<docker-container>
terraform {
  required_providers {
    coder = {
      source = "kreuzwerker/docker"
    }
  }
}

// The agent is configured with "token" auth.

resource "docker_container" "workspace" {
  count = data.coder_workspace.me.start_count
  image = "codercom/enterprise-base:ubuntu"
  # Uses lower() to avoid Docker restriction on container names.
  name = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  # Hostname makes the shell more user friendly: coder@my-workspace:~$
  hostname = data.coder_workspace.me.name
  # Use the docker gateway if the access URL is 127.0.0.1.
  entrypoint = ["sh", "-c", replace(coder_agent.main.init_script, "/localhost|127\\.0\\.0\\.1/", "host.docker.internal")]
  env        = ["CODER_AGENT_TOKEN=${coder_agent.main.token}"]
  host {
    host = "host.docker.internal"
    ip   = "host-gateway"
  }
  volumes {
    container_path = "/home/coder"
    volume_name    = docker_volume.home_volume.name
    read_only      = false
  }
}
</docker-container>

<kubernetes-pod>
// The agent is configured with "token" auth.

resource "kubernetes_deployment" "main" {
  count = data.coder_workspace.me.start_count
  depends_on = [
    kubernetes_persistent_volume_claim.home
  ]
  wait_for_rollout = false
  metadata {
    name      = "coder-${data.coder_workspace.me.id}"
  }

  spec {
    replicas = 1
    strategy {
      type = "Recreate"
    }

    template {
      spec {
        security_context {
          run_as_user     = 1000
          fs_group        = 1000
          run_as_non_root = true
        }

        container {
          name              = "dev"
          image             = "codercom/enterprise-base:ubuntu"
          image_pull_policy = "Always"
          command           = ["sh", "-c", coder_agent.main.init_script]
          security_context {
            run_as_user = "1000"
          }
          env {
            name  = "CODER_AGENT_TOKEN"
            value = coder_agent.main.token
          }
        }
      }
    }
  }
}
</kubernetes-pod>

The file_id provided is a reference to a tar file you have uploaded containing the Terraform.
`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"template_id": map[string]any{
					"type": "string",
				},
				"file_id": map[string]any{
					"type": "string",
				},
			},
			Required: []string{"file_id"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args CreateTemplateVersionArgs) (codersdk.TemplateVersion, error) {
		me, err := deps.coderClient.User(ctx, "me")
		if err != nil {
			return codersdk.TemplateVersion{}, err
		}
		fileID, err := uuid.Parse(args.FileID)
		if err != nil {
			return codersdk.TemplateVersion{}, xerrors.Errorf("file_id must be a valid UUID: %w", err)
		}
		var templateID uuid.UUID
		if args.TemplateID != "" {
			tid, err := uuid.Parse(args.TemplateID)
			if err != nil {
				return codersdk.TemplateVersion{}, xerrors.Errorf("template_id must be a valid UUID: %w", err)
			}
			templateID = tid
		}
		templateVersion, err := deps.coderClient.CreateTemplateVersion(ctx, me.OrganizationIDs[0], codersdk.CreateTemplateVersionRequest{
			Message:       "Created by AI",
			StorageMethod: codersdk.ProvisionerStorageMethodFile,
			FileID:        fileID,
			Provisioner:   codersdk.ProvisionerTypeTerraform,
			TemplateID:    templateID,
		})
		if err != nil {
			return codersdk.TemplateVersion{}, err
		}
		return templateVersion, nil
	},
}

type GetWorkspaceAgentLogsArgs struct {
	WorkspaceAgentID string `json:"workspace_agent_id"`
}

var GetWorkspaceAgentLogs = Tool[GetWorkspaceAgentLogsArgs, []string]{
	Tool: aisdk.Tool{
		Name: ToolNameGetWorkspaceAgentLogs,
		Description: `Get the logs of a workspace agent.

		More logs may appear after this call. It does not wait for the agent to finish.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"workspace_agent_id": map[string]any{
					"type": "string",
				},
			},
			Required: []string{"workspace_agent_id"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args GetWorkspaceAgentLogsArgs) ([]string, error) {
		workspaceAgentID, err := uuid.Parse(args.WorkspaceAgentID)
		if err != nil {
			return nil, xerrors.Errorf("workspace_agent_id must be a valid UUID: %w", err)
		}
		logs, closer, err := deps.coderClient.WorkspaceAgentLogsAfter(ctx, workspaceAgentID, 0, false)
		if err != nil {
			return nil, err
		}
		defer closer.Close()
		var acc []string
		for logChunk := range logs {
			for _, log := range logChunk {
				acc = append(acc, log.Output)
			}
		}
		return acc, nil
	},
}

type GetWorkspaceBuildLogsArgs struct {
	WorkspaceBuildID string `json:"workspace_build_id"`
}

var GetWorkspaceBuildLogs = Tool[GetWorkspaceBuildLogsArgs, []string]{
	Tool: aisdk.Tool{
		Name: ToolNameGetWorkspaceBuildLogs,
		Description: `Get the logs of a workspace build.

		Useful for checking whether a workspace builds successfully or not.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"workspace_build_id": map[string]any{
					"type": "string",
				},
			},
			Required: []string{"workspace_build_id"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args GetWorkspaceBuildLogsArgs) ([]string, error) {
		workspaceBuildID, err := uuid.Parse(args.WorkspaceBuildID)
		if err != nil {
			return nil, xerrors.Errorf("workspace_build_id must be a valid UUID: %w", err)
		}
		logs, closer, err := deps.coderClient.WorkspaceBuildLogsAfter(ctx, workspaceBuildID, 0)
		if err != nil {
			return nil, err
		}
		defer closer.Close()
		var acc []string
		for log := range logs {
			acc = append(acc, log.Output)
		}
		return acc, nil
	},
}

type GetTemplateVersionLogsArgs struct {
	TemplateVersionID string `json:"template_version_id"`
}

var GetTemplateVersionLogs = Tool[GetTemplateVersionLogsArgs, []string]{
	Tool: aisdk.Tool{
		Name:        ToolNameGetTemplateVersionLogs,
		Description: "Get the logs of a template version. This is useful to check whether a template version successfully imports or not.",
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"template_version_id": map[string]any{
					"type": "string",
				},
			},
			Required: []string{"template_version_id"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args GetTemplateVersionLogsArgs) ([]string, error) {
		templateVersionID, err := uuid.Parse(args.TemplateVersionID)
		if err != nil {
			return nil, xerrors.Errorf("template_version_id must be a valid UUID: %w", err)
		}

		logs, closer, err := deps.coderClient.TemplateVersionLogsAfter(ctx, templateVersionID, 0)
		if err != nil {
			return nil, err
		}
		defer closer.Close()
		var acc []string
		for log := range logs {
			acc = append(acc, log.Output)
		}
		return acc, nil
	},
}

type UpdateTemplateActiveVersionArgs struct {
	TemplateID        string `json:"template_id"`
	TemplateVersionID string `json:"template_version_id"`
}

var UpdateTemplateActiveVersion = Tool[UpdateTemplateActiveVersionArgs, string]{
	Tool: aisdk.Tool{
		Name:        ToolNameUpdateTemplateActiveVersion,
		Description: "Update the active version of a template. This is helpful when iterating on templates.",
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"template_id": map[string]any{
					"type": "string",
				},
				"template_version_id": map[string]any{
					"type": "string",
				},
			},
			Required: []string{"template_id", "template_version_id"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args UpdateTemplateActiveVersionArgs) (string, error) {
		templateID, err := uuid.Parse(args.TemplateID)
		if err != nil {
			return "", xerrors.Errorf("template_id must be a valid UUID: %w", err)
		}
		templateVersionID, err := uuid.Parse(args.TemplateVersionID)
		if err != nil {
			return "", xerrors.Errorf("template_version_id must be a valid UUID: %w", err)
		}
		err = deps.coderClient.UpdateActiveTemplateVersion(ctx, templateID, codersdk.UpdateActiveTemplateVersion{
			ID: templateVersionID,
		})
		if err != nil {
			return "", err
		}
		return "Successfully updated active version!", nil
	},
}

type UploadTarFileArgs struct {
	Files map[string]string `json:"files"`
}

var UploadTarFile = Tool[UploadTarFileArgs, codersdk.UploadResponse]{
	Tool: aisdk.Tool{
		Name:        ToolNameUploadTarFile,
		Description: `Create and upload a tar file by key/value mapping of file names to file contents. Use this to create template versions. Reference the tool description of "create_template_version" to understand template requirements.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"files": map[string]any{
					"type":        "object",
					"description": "A map of file names to file contents.",
				},
			},
			Required: []string{"files"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args UploadTarFileArgs) (codersdk.UploadResponse, error) {
		pipeReader, pipeWriter := io.Pipe()
		done := make(chan struct{})
		go func() {
			defer func() {
				_ = pipeWriter.Close()
				close(done)
			}()
			tarWriter := tar.NewWriter(pipeWriter)
			for name, content := range args.Files {
				header := &tar.Header{
					Name: name,
					Size: int64(len(content)),
					Mode: 0o644,
				}
				if err := tarWriter.WriteHeader(header); err != nil {
					_ = pipeWriter.CloseWithError(err)
					return
				}
				if _, err := tarWriter.Write([]byte(content)); err != nil {
					_ = pipeWriter.CloseWithError(err)
					return
				}
			}
			if err := tarWriter.Close(); err != nil {
				_ = pipeWriter.CloseWithError(err)
			}
		}()

		resp, err := deps.coderClient.Upload(ctx, codersdk.ContentTypeTar, pipeReader)
		if err != nil {
			_ = pipeReader.CloseWithError(err)
			<-done
			return codersdk.UploadResponse{}, err
		}
		<-done
		return resp, nil
	},
}

type CreateTemplateArgs struct {
	Description string `json:"description"`
	DisplayName string `json:"display_name"`
	Icon        string `json:"icon"`
	Name        string `json:"name"`
	VersionID   string `json:"version_id"`
}

var CreateTemplate = Tool[CreateTemplateArgs, codersdk.Template]{
	Tool: aisdk.Tool{
		Name:        ToolNameCreateTemplate,
		Description: "Create a new template in Coder. First, you must create a template version.",
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"name": map[string]any{
					"type": "string",
				},
				"display_name": map[string]any{
					"type": "string",
				},
				"description": map[string]any{
					"type": "string",
				},
				"icon": map[string]any{
					"type":        "string",
					"description": "A URL to an icon to use.",
				},
				"version_id": map[string]any{
					"type":        "string",
					"description": "The ID of the version to use.",
				},
			},
			Required: []string{"name", "display_name", "description", "version_id"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args CreateTemplateArgs) (codersdk.Template, error) {
		me, err := deps.coderClient.User(ctx, "me")
		if err != nil {
			return codersdk.Template{}, err
		}
		versionID, err := uuid.Parse(args.VersionID)
		if err != nil {
			return codersdk.Template{}, xerrors.Errorf("version_id must be a valid UUID: %w", err)
		}
		template, err := deps.coderClient.CreateTemplate(ctx, me.OrganizationIDs[0], codersdk.CreateTemplateRequest{
			Name:        args.Name,
			DisplayName: args.DisplayName,
			Description: args.Description,
			VersionID:   versionID,
		})
		if err != nil {
			return codersdk.Template{}, err
		}
		return template, nil
	},
}

type DeleteTemplateArgs struct {
	TemplateID string `json:"template_id"`
}

var DeleteTemplate = Tool[DeleteTemplateArgs, codersdk.Response]{
	Tool: aisdk.Tool{
		Name:        ToolNameDeleteTemplate,
		Description: "Delete a template. This is irreversible.",
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"template_id": map[string]any{
					"type": "string",
				},
			},
			Required: []string{"template_id"},
		},
	},
	Handler: func(ctx context.Context, deps Deps, args DeleteTemplateArgs) (codersdk.Response, error) {
		templateID, err := uuid.Parse(args.TemplateID)
		if err != nil {
			return codersdk.Response{}, xerrors.Errorf("template_id must be a valid UUID: %w", err)
		}
		err = deps.coderClient.DeleteTemplate(ctx, templateID)
		if err != nil {
			return codersdk.Response{}, err
		}
		return codersdk.Response{
			Message: "Template deleted successfully.",
		}, nil
	},
}

type MinimalWorkspace struct {
	ID                      string    `json:"id"`
	Name                    string    `json:"name"`
	TemplateID              string    `json:"template_id"`
	TemplateName            string    `json:"template_name"`
	TemplateDisplayName     string    `json:"template_display_name"`
	TemplateIcon            string    `json:"template_icon"`
	TemplateActiveVersionID uuid.UUID `json:"template_active_version_id"`
	Outdated                bool      `json:"outdated"`
}

type MinimalTemplate struct {
	DisplayName     string    `json:"display_name"`
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	ActiveVersionID uuid.UUID `json:"active_version_id"`
	ActiveUserCount int       `json:"active_user_count"`
}

type WorkspaceLSArgs struct {
	Workspace string `json:"workspace"`
	Path      string `json:"path"`
}

type WorkspaceLSFile struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

type WorkspaceLSResponse struct {
	Contents []WorkspaceLSFile `json:"contents"`
}

var WorkspaceLS = Tool[WorkspaceLSArgs, WorkspaceLSResponse]{
	Tool: aisdk.Tool{
		Name:        ToolNameWorkspaceLS,
		Description: `List directories in a workspace.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"workspace": map[string]any{
					"type":        "string",
					"description": workspaceAgentDescription,
				},
				"path": map[string]any{
					"type":        "string",
					"description": "The absolute path of the directory in the workspace to list.",
				},
			},
			Required: []string{"path", "workspace"},
		},
	},
	UserClientOptional: true,
	Handler: func(ctx context.Context, deps Deps, args WorkspaceLSArgs) (WorkspaceLSResponse, error) {
		conn, err := newAgentConn(ctx, deps.coderClient, args.Workspace)
		if err != nil {
			return WorkspaceLSResponse{}, err
		}
		defer conn.Close()

		res, err := conn.LS(ctx, args.Path, workspacesdk.LSRequest{})
		if err != nil {
			return WorkspaceLSResponse{}, err
		}

		contents := make([]WorkspaceLSFile, len(res.Contents))
		for i, f := range res.Contents {
			contents[i] = WorkspaceLSFile{
				Path:  f.AbsolutePathString,
				IsDir: f.IsDir,
			}
		}
		return WorkspaceLSResponse{Contents: contents}, nil
	},
}

type WorkspaceReadFileArgs struct {
	Workspace string `json:"workspace"`
	Path      string `json:"path"`
	Offset    int64  `json:"offset"`
	Limit     int64  `json:"limit"`
}

type WorkspaceReadFileResponse struct {
	// Content is the base64-encoded bytes from the file.
	Content  []byte `json:"content"`
	MimeType string `json:"mimeType"`
}

const maxFileLimit = 1 << 20 // 1MiB

var WorkspaceReadFile = Tool[WorkspaceReadFileArgs, WorkspaceReadFileResponse]{
	Tool: aisdk.Tool{
		Name:        ToolNameWorkspaceReadFile,
		Description: `Read from a file in a workspace.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"workspace": map[string]any{
					"type":        "string",
					"description": workspaceAgentDescription,
				},
				"path": map[string]any{
					"type":        "string",
					"description": "The absolute path of the file to read in the workspace.",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "A byte offset indicating where in the file to start reading. Defaults to zero. An empty string indicates the end of the file has been reached.",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "The number of bytes to read. Cannot exceed 1 MiB. Defaults to the full size of the file or 1 MiB, whichever is lower.",
				},
			},
			Required: []string{"path", "workspace"},
		},
	},
	UserClientOptional: true,
	Handler: func(ctx context.Context, deps Deps, args WorkspaceReadFileArgs) (WorkspaceReadFileResponse, error) {
		conn, err := newAgentConn(ctx, deps.coderClient, args.Workspace)
		if err != nil {
			return WorkspaceReadFileResponse{}, err
		}
		defer conn.Close()

		// Ideally we could stream this all the way back, but it looks like the MCP
		// interfaces only allow returning full responses which means the whole
		// thing has to be read into memory.  So, add a maximum limit to compensate.
		limit := args.Limit
		if limit == 0 {
			limit = maxFileLimit
		} else if limit > maxFileLimit {
			return WorkspaceReadFileResponse{}, xerrors.Errorf("limit must be %d or less, got %d", maxFileLimit, limit)
		}

		reader, mimeType, err := conn.ReadFile(ctx, args.Path, args.Offset, limit)
		if err != nil {
			return WorkspaceReadFileResponse{}, err
		}
		defer reader.Close()

		bs, err := io.ReadAll(reader)
		if err != nil {
			return WorkspaceReadFileResponse{}, xerrors.Errorf("read response body: %w", err)
		}

		return WorkspaceReadFileResponse{Content: bs, MimeType: mimeType}, nil
	},
}

type WorkspaceWriteFileArgs struct {
	Workspace string `json:"workspace"`
	Path      string `json:"path"`
	Content   []byte `json:"content"`
}

var WorkspaceWriteFile = Tool[WorkspaceWriteFileArgs, codersdk.Response]{
	Tool: aisdk.Tool{
		Name: ToolNameWorkspaceWriteFile,
		Description: `Write a file in a workspace.

If a file write fails due to syntax errors or encoding issues, do NOT switch
to using bash commands as a workaround. Instead:

	1. Read the error message carefully to identify the issue
	2. Fix the content encoding/syntax
	3. Retry with this tool

The content parameter expects base64-encoded bytes. Ensure your source content
is correct before encoding it. If you encounter errors, decode and verify the
content you are trying to write, then re-encode it properly.
`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"workspace": map[string]any{
					"type":        "string",
					"description": workspaceAgentDescription,
				},
				"path": map[string]any{
					"type":        "string",
					"description": "The absolute path of the file to write in the workspace.",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "The base64-encoded bytes to write to the file.",
				},
			},
			Required: []string{"path", "workspace", "content"},
		},
	},
	UserClientOptional: true,
	Handler: func(ctx context.Context, deps Deps, args WorkspaceWriteFileArgs) (codersdk.Response, error) {
		conn, err := newAgentConn(ctx, deps.coderClient, args.Workspace)
		if err != nil {
			return codersdk.Response{}, err
		}
		defer conn.Close()

		reader := bytes.NewReader(args.Content)
		err = conn.WriteFile(ctx, args.Path, reader)
		if err != nil {
			return codersdk.Response{}, err
		}

		return codersdk.Response{
			Message: "File written successfully.",
		}, nil
	},
}

type WorkspaceEditFileArgs struct {
	Workspace string                  `json:"workspace"`
	Path      string                  `json:"path"`
	Edits     []workspacesdk.FileEdit `json:"edits"`
}

var WorkspaceEditFile = Tool[WorkspaceEditFileArgs, codersdk.Response]{
	Tool: aisdk.Tool{
		Name:        ToolNameWorkspaceEditFile,
		Description: `Edit a file in a workspace.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"workspace": map[string]any{
					"type":        "string",
					"description": workspaceAgentDescription,
				},
				"path": map[string]any{
					"type":        "string",
					"description": "The absolute path of the file to write in the workspace.",
				},
				"edits": map[string]any{
					"type":        "array",
					"description": "An array of edit operations.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"search": map[string]any{
								"type":        "string",
								"description": "The old string to replace.",
							},
							"replace": map[string]any{
								"type":        "string",
								"description": "The new string that replaces the old string.",
							},
						},
						"required": []string{"search", "replace"},
					},
				},
			},
			Required: []string{"path", "workspace", "edits"},
		},
	},
	UserClientOptional: true,
	Handler: func(ctx context.Context, deps Deps, args WorkspaceEditFileArgs) (codersdk.Response, error) {
		conn, err := newAgentConn(ctx, deps.coderClient, args.Workspace)
		if err != nil {
			return codersdk.Response{}, err
		}
		defer conn.Close()

		err = conn.EditFiles(ctx, workspacesdk.FileEditRequest{
			Files: []workspacesdk.FileEdits{
				{
					Path:  args.Path,
					Edits: args.Edits,
				},
			},
		})
		if err != nil {
			return codersdk.Response{}, err
		}

		return codersdk.Response{
			Message: "File edited successfully.",
		}, nil
	},
}

type WorkspaceEditFilesArgs struct {
	Workspace string                   `json:"workspace"`
	Files     []workspacesdk.FileEdits `json:"files"`
}

var WorkspaceEditFiles = Tool[WorkspaceEditFilesArgs, codersdk.Response]{
	Tool: aisdk.Tool{
		Name:        ToolNameWorkspaceEditFiles,
		Description: `Edit one or more files in a workspace.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"workspace": map[string]any{
					"type":        "string",
					"description": workspaceAgentDescription,
				},
				"files": map[string]any{
					"type":        "array",
					"description": "An array of files to edit.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path": map[string]any{
								"type":        "string",
								"description": "The absolute path of the file to write in the workspace.",
							},
							"edits": map[string]any{
								"type":        "array",
								"description": "An array of edit operations.",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"search": map[string]any{
											"type":        "string",
											"description": "The old string to replace.",
										},
										"replace": map[string]any{
											"type":        "string",
											"description": "The new string that replaces the old string.",
										},
									},
									"required": []string{"search", "replace"},
								},
							},
						},
						"required": []string{"path", "edits"},
					},
				},
			},
			Required: []string{"workspace", "files"},
		},
	},
	UserClientOptional: true,
	Handler: func(ctx context.Context, deps Deps, args WorkspaceEditFilesArgs) (codersdk.Response, error) {
		conn, err := newAgentConn(ctx, deps.coderClient, args.Workspace)
		if err != nil {
			return codersdk.Response{}, err
		}
		defer conn.Close()

		err = conn.EditFiles(ctx, workspacesdk.FileEditRequest{Files: args.Files})
		if err != nil {
			return codersdk.Response{}, err
		}

		return codersdk.Response{
			Message: "File(s) edited successfully.",
		}, nil
	},
}

type WorkspacePortForwardArgs struct {
	Workspace string `json:"workspace"`
	Port      int    `json:"port"`
}

type WorkspacePortForwardResponse struct {
	URL string `json:"url"`
}

var WorkspacePortForward = Tool[WorkspacePortForwardArgs, WorkspacePortForwardResponse]{
	Tool: aisdk.Tool{
		Name:        ToolNameWorkspacePortForward,
		Description: `Fetch URLs that forward to the specified port.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"workspace": map[string]any{
					"type":        "string",
					"description": workspaceAgentDescription,
				},
				"port": map[string]any{
					"type":        "number",
					"description": "The port to forward.",
				},
			},
			Required: []string{"workspace", "port"},
		},
	},
	UserClientOptional: true,
	Handler: func(ctx context.Context, deps Deps, args WorkspacePortForwardArgs) (WorkspacePortForwardResponse, error) {
		workspaceName := NormalizeWorkspaceInput(args.Workspace)
		workspace, workspaceAgent, err := findWorkspaceAndAgent(ctx, deps.coderClient, workspaceName)
		if err != nil {
			return WorkspacePortForwardResponse{}, xerrors.Errorf("failed to find workspace: %w", err)
		}
		res, err := deps.coderClient.AppHost(ctx)
		if err != nil {
			return WorkspacePortForwardResponse{}, xerrors.Errorf("failed to get app host: %w", err)
		}
		if res.Host == "" {
			return WorkspacePortForwardResponse{}, xerrors.New("no app host for forwarding has been configured")
		}
		url := appurl.ApplicationURL{
			AppSlugOrPort: strconv.Itoa(args.Port),
			AgentName:     workspaceAgent.Name,
			WorkspaceName: workspace.Name,
			Username:      workspace.OwnerName,
		}
		return WorkspacePortForwardResponse{
			URL: deps.coderClient.URL.Scheme + "://" + strings.Replace(res.Host, "*", url.String(), 1),
		}, nil
	},
}

type WorkspaceListAppsArgs struct {
	Workspace string `json:"workspace"`
}

type WorkspaceListApp struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type WorkspaceListAppsResponse struct {
	Apps []WorkspaceListApp `json:"apps"`
}

var WorkspaceListApps = Tool[WorkspaceListAppsArgs, WorkspaceListAppsResponse]{
	Tool: aisdk.Tool{
		Name:        ToolNameWorkspaceListApps,
		Description: `List the URLs of Coder apps running in a workspace for a single agent.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"workspace": map[string]any{
					"type":        "string",
					"description": workspaceAgentDescription,
				},
			},
			Required: []string{"workspace"},
		},
	},
	UserClientOptional: true,
	Handler: func(ctx context.Context, deps Deps, args WorkspaceListAppsArgs) (WorkspaceListAppsResponse, error) {
		workspaceName := NormalizeWorkspaceInput(args.Workspace)
		_, workspaceAgent, err := findWorkspaceAndAgent(ctx, deps.coderClient, workspaceName)
		if err != nil {
			return WorkspaceListAppsResponse{}, xerrors.Errorf("failed to find workspace: %w", err)
		}

		var res WorkspaceListAppsResponse
		for _, app := range workspaceAgent.Apps {
			name := app.DisplayName
			if name == "" {
				name = app.Slug
			}
			res.Apps = append(res.Apps, WorkspaceListApp{
				Name: name,
				URL:  app.URL,
			})
		}

		return res, nil
	},
}

type CreateTaskArgs struct {
	Input                   string `json:"input"`
	TemplateVersionID       string `json:"template_version_id"`
	TemplateVersionPresetID string `json:"template_version_preset_id"`
	User                    string `json:"user"`
}

var CreateTask = Tool[CreateTaskArgs, codersdk.Task]{
	Tool: aisdk.Tool{
		Name:        ToolNameCreateTask,
		Description: `Create a task.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"input": map[string]any{
					"type":        "string",
					"description": "Input/prompt for the task.",
				},
				"template_version_id": map[string]any{
					"type":        "string",
					"description": "ID of the template version to create the task from.",
				},
				"template_version_preset_id": map[string]any{
					"type":        "string",
					"description": "Optional ID of the template version preset to create the task from.",
				},
				"user": map[string]any{
					"type":        "string",
					"description": userDescription("create a task"),
				},
			},
			Required: []string{"input", "template_version_id"},
		},
	},
	UserClientOptional: true,
	Handler: func(ctx context.Context, deps Deps, args CreateTaskArgs) (codersdk.Task, error) {
		if args.Input == "" {
			return codersdk.Task{}, xerrors.New("input is required")
		}

		tvID, err := uuid.Parse(args.TemplateVersionID)
		if err != nil {
			return codersdk.Task{}, xerrors.New("template_version_id must be a valid UUID")
		}

		var tvPresetID uuid.UUID
		if args.TemplateVersionPresetID != "" {
			tvPresetID, err = uuid.Parse(args.TemplateVersionPresetID)
			if err != nil {
				return codersdk.Task{}, xerrors.New("template_version_preset_id must be a valid UUID")
			}
		}

		if args.User == "" {
			args.User = codersdk.Me
		}

		task, err := deps.coderClient.CreateTask(ctx, args.User, codersdk.CreateTaskRequest{
			Input:                   args.Input,
			TemplateVersionID:       tvID,
			TemplateVersionPresetID: tvPresetID,
		})
		if err != nil {
			return codersdk.Task{}, xerrors.Errorf("create task: %w", err)
		}

		return task, nil
	},
}

type DeleteTaskArgs struct {
	TaskID string `json:"task_id"`
}

var DeleteTask = Tool[DeleteTaskArgs, codersdk.Response]{
	Tool: aisdk.Tool{
		Name:        ToolNameDeleteTask,
		Description: `Delete a task.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"task_id": map[string]any{
					"type":        "string",
					"description": taskIDDescription("delete"),
				},
			},
			Required: []string{"task_id"},
		},
	},
	UserClientOptional: true,
	Handler: func(ctx context.Context, deps Deps, args DeleteTaskArgs) (codersdk.Response, error) {
		if args.TaskID == "" {
			return codersdk.Response{}, xerrors.New("task_id is required")
		}

		task, err := deps.coderClient.TaskByIdentifier(ctx, args.TaskID)
		if err != nil {
			return codersdk.Response{}, xerrors.Errorf("resolve task: %w", err)
		}

		err = deps.coderClient.DeleteTask(ctx, task.OwnerName, task.ID)
		if err != nil {
			return codersdk.Response{}, xerrors.Errorf("delete task: %w", err)
		}

		return codersdk.Response{
			Message: "Task deleted successfully",
		}, nil
	},
}

type ListTasksArgs struct {
	Status codersdk.TaskStatus `json:"status"`
	User   string              `json:"user"`
}

type ListTasksResponse struct {
	Tasks []codersdk.Task `json:"tasks"`
}

var ListTasks = Tool[ListTasksArgs, ListTasksResponse]{
	Tool: aisdk.Tool{
		Name:        ToolNameListTasks,
		Description: `List tasks.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"status": map[string]any{
					"type":        "string",
					"description": "Optional filter by task status.",
				},
				"user": map[string]any{
					"type":        "string",
					"description": userDescription("list tasks"),
				},
			},
			Required: []string{},
		},
	},
	UserClientOptional: true,
	Handler: func(ctx context.Context, deps Deps, args ListTasksArgs) (ListTasksResponse, error) {
		if args.User == "" {
			args.User = codersdk.Me
		}

		tasks, err := deps.coderClient.Tasks(ctx, &codersdk.TasksFilter{
			Owner:  args.User,
			Status: args.Status,
		})
		if err != nil {
			return ListTasksResponse{}, xerrors.Errorf("list tasks: %w", err)
		}

		return ListTasksResponse{
			Tasks: tasks,
		}, nil
	},
}

type GetTaskStatusArgs struct {
	TaskID string `json:"task_id"`
}

type GetTaskStatusResponse struct {
	Status codersdk.TaskStatus      `json:"status"`
	State  *codersdk.TaskStateEntry `json:"state"`
}

var GetTaskStatus = Tool[GetTaskStatusArgs, GetTaskStatusResponse]{
	Tool: aisdk.Tool{
		Name:        ToolNameGetTaskStatus,
		Description: `Get the status of a task.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"task_id": map[string]any{
					"type":        "string",
					"description": taskIDDescription("get"),
				},
			},
			Required: []string{"task_id"},
		},
	},
	UserClientOptional: true,
	Handler: func(ctx context.Context, deps Deps, args GetTaskStatusArgs) (GetTaskStatusResponse, error) {
		if args.TaskID == "" {
			return GetTaskStatusResponse{}, xerrors.New("task_id is required")
		}

		task, err := deps.coderClient.TaskByIdentifier(ctx, args.TaskID)
		if err != nil {
			return GetTaskStatusResponse{}, xerrors.Errorf("resolve task %q: %w", args.TaskID, err)
		}

		return GetTaskStatusResponse{
			Status: task.Status,
			State:  task.CurrentState,
		}, nil
	},
}

type SendTaskInputArgs struct {
	TaskID string `json:"task_id"`
	Input  string `json:"input"`
}

var SendTaskInput = Tool[SendTaskInputArgs, codersdk.Response]{
	Tool: aisdk.Tool{
		Name:        ToolNameSendTaskInput,
		Description: `Send input to a running task.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"task_id": map[string]any{
					"type":        "string",
					"description": taskIDDescription("prompt"),
				},
				"input": map[string]any{
					"type":        "string",
					"description": "The input to send to the task.",
				},
			},
			Required: []string{"task_id", "input"},
		},
	},
	UserClientOptional: true,
	Handler: func(ctx context.Context, deps Deps, args SendTaskInputArgs) (codersdk.Response, error) {
		if args.TaskID == "" {
			return codersdk.Response{}, xerrors.New("task_id is required")
		}

		if args.Input == "" {
			return codersdk.Response{}, xerrors.New("input is required")
		}

		task, err := deps.coderClient.TaskByIdentifier(ctx, args.TaskID)
		if err != nil {
			return codersdk.Response{}, xerrors.Errorf("resolve task %q: %w", args.TaskID, err)
		}

		err = deps.coderClient.TaskSend(ctx, task.OwnerName, task.ID, codersdk.TaskSendRequest{
			Input: args.Input,
		})
		if err != nil {
			return codersdk.Response{}, xerrors.Errorf("send task input %q: %w", args.TaskID, err)
		}

		return codersdk.Response{
			Message: "Input sent to task successfully.",
		}, nil
	},
}

type GetTaskLogsArgs struct {
	TaskID string `json:"task_id"`
}

var GetTaskLogs = Tool[GetTaskLogsArgs, codersdk.TaskLogsResponse]{
	Tool: aisdk.Tool{
		Name:        ToolNameGetTaskLogs,
		Description: `Get the logs of a task.`,
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"task_id": map[string]any{
					"type":        "string",
					"description": taskIDDescription("query"),
				},
			},
			Required: []string{"task_id"},
		},
	},
	UserClientOptional: true,
	Handler: func(ctx context.Context, deps Deps, args GetTaskLogsArgs) (codersdk.TaskLogsResponse, error) {
		if args.TaskID == "" {
			return codersdk.TaskLogsResponse{}, xerrors.New("task_id is required")
		}

		task, err := deps.coderClient.TaskByIdentifier(ctx, args.TaskID)
		if err != nil {
			return codersdk.TaskLogsResponse{}, err
		}

		logs, err := deps.coderClient.TaskLogs(ctx, task.OwnerName, task.ID)
		if err != nil {
			return codersdk.TaskLogsResponse{}, xerrors.Errorf("get task logs %q: %w", args.TaskID, err)
		}

		return logs, nil
	},
}

// NormalizeWorkspaceInput converts workspace name input to standard format.
// Handles the following input formats:
//   - workspace                    → workspace
//   - workspace.agent              → workspace.agent
//   - owner/workspace              → owner/workspace
//   - owner--workspace             → owner/workspace
//   - owner/workspace.agent        → owner/workspace.agent
//   - owner--workspace.agent       → owner/workspace.agent
//   - agent.workspace.owner        → owner/workspace.agent (Coder Connect format)
func NormalizeWorkspaceInput(input string) string {
	// Handle the special Coder Connect format: agent.workspace.owner
	// This format uses only dots and has exactly 3 parts
	if strings.Count(input, ".") == 2 && !strings.Contains(input, "/") && !strings.Contains(input, "--") {
		parts := strings.Split(input, ".")
		if len(parts) == 3 {
			// Convert agent.workspace.owner → owner/workspace.agent
			return fmt.Sprintf("%s/%s.%s", parts[2], parts[1], parts[0])
		}
	}

	// Convert -- separator to / separator for consistency
	normalized := strings.ReplaceAll(input, "--", "/")

	return normalized
}

// resolveWorkspaceAgent finds workspace and agent by name with auto-start
// support. The workspace identifier must be in the format
// [owner/]workspace[.agent].
func resolveWorkspaceAgent(ctx context.Context, client *codersdk.Client, workspace string) (codersdk.WorkspaceAgent, error) {
	workspaceName := NormalizeWorkspaceInput(workspace)
	_, workspaceAgent, err := findWorkspaceAndAgent(ctx, client, workspaceName)
	if err != nil {
		return codersdk.WorkspaceAgent{}, xerrors.Errorf("failed to find workspace: %w", err)
	}

	return workspaceAgent, nil
}

func dialAgentConn(ctx context.Context, client *codersdk.Client, agentID uuid.UUID) (workspacesdk.AgentConn, error) {
	// Wait for agent to be ready.
	if err := cliui.Agent(ctx, io.Discard, agentID, cliui.AgentOptions{
		FetchInterval: 0,
		Fetch:         client.WorkspaceAgent,
		FetchLogs:     client.WorkspaceAgentLogsAfter,
		Wait:          true, // Always wait for startup scripts
	}); err != nil {
		return nil, xerrors.Errorf("agent not ready: %w", err)
	}

	wsClient := workspacesdk.New(client)

	conn, err := wsClient.DialAgent(ctx, agentID, &workspacesdk.DialAgentOptions{
		BlockEndpoints: false,
	})
	if err != nil {
		return nil, xerrors.Errorf("failed to dial agent: %w", err)
	}

	if !conn.AwaitReachable(ctx) {
		conn.Close()
		return nil, xerrors.New("agent connection not reachable")
	}
	return conn, nil
}

// newAgentConn returns a connection to the agent specified by the workspace,
// which must be in the format [owner/]workspace[.agent].
func newAgentConn(ctx context.Context, client *codersdk.Client, workspace string) (workspacesdk.AgentConn, error) {
	workspaceAgent, err := resolveWorkspaceAgent(ctx, client, workspace)
	if err != nil {
		return nil, err
	}

	return dialAgentConn(ctx, client, workspaceAgent.ID)
}

const workspaceDescription = "The workspace ID or name in the format [owner/]workspace. If an owner is not specified, the authenticated user is used."

const workspaceAgentDescription = "The workspace name in the format [owner/]workspace[.agent]. If an owner is not specified, the authenticated user is used."

func taskIDDescription(action string) string {
	return fmt.Sprintf("ID or workspace identifier in the format [owner/]workspace[.agent] for the task to %s. If an owner is not specified, the authenticated user is used.", action)
}

func userDescription(action string) string {
	return fmt.Sprintf("Username or ID of the user for which to %s. Omit or use the `me` keyword to %s for the authenticated user.", action, action)
}
