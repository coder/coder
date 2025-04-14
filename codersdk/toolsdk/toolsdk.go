package toolsdk

import (
	"archive/tar"
	"context"
	"io"

	"github.com/google/uuid"
	"github.com/kylecarbs/aisdk-go"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

// HandlerFunc is a function that handles a tool call.
type HandlerFunc[T any] func(ctx context.Context, args map[string]any) (T, error)

type Tool[T any] struct {
	aisdk.Tool
	Handler HandlerFunc[T]
}

// Generic returns a Tool[any] that can be used to call the tool.
func (t Tool[T]) Generic() Tool[any] {
	return Tool[any]{
		Tool: t.Tool,
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			return t.Handler(ctx, args)
		},
	}
}

var (
	// All is a list of all tools that can be used in the Coder CLI.
	// When you add a new tool, be sure to include it here!
	All = []Tool[any]{
		CreateTemplateVersion.Generic(),
		CreateTemplate.Generic(),
		CreateWorkspace.Generic(),
		CreateWorkspaceBuild.Generic(),
		DeleteTemplate.Generic(),
		GetAuthenticatedUser.Generic(),
		GetTemplateVersionLogs.Generic(),
		GetWorkspace.Generic(),
		GetWorkspaceAgentLogs.Generic(),
		GetWorkspaceBuildLogs.Generic(),
		ListWorkspaces.Generic(),
		ListTemplates.Generic(),
		ListTemplateVersionParameters.Generic(),
		ReportTask.Generic(),
		UploadTarFile.Generic(),
		UpdateTemplateActiveVersion.Generic(),
	}

	ReportTask = Tool[string]{
		Tool: aisdk.Tool{
			Name:        "coder_report_task",
			Description: "Report progress on a user task in Coder.",
			Schema: aisdk.Schema{
				Properties: map[string]any{
					"summary": map[string]any{
						"type":        "string",
						"description": "A concise summary of your current progress on the task. This must be less than 160 characters in length.",
					},
					"link": map[string]any{
						"type":        "string",
						"description": "A link to a relevant resource, such as a PR or issue.",
					},
					"emoji": map[string]any{
						"type":        "string",
						"description": "An emoji that visually represents your current progress. Choose an emoji that helps the user understand your current status at a glance.",
					},
					"state": map[string]any{
						"type":        "string",
						"description": "The state of your task. This can be one of the following: working, complete, or failure. Select the state that best represents your current progress.",
						"enum": []string{
							string(codersdk.WorkspaceAppStatusStateWorking),
							string(codersdk.WorkspaceAppStatusStateComplete),
							string(codersdk.WorkspaceAppStatusStateFailure),
						},
					},
				},
				Required: []string{"summary", "link", "emoji", "state"},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			agentClient, err := agentClientFromContext(ctx)
			if err != nil {
				return "", xerrors.New("tool unavailable as CODER_AGENT_TOKEN or CODER_AGENT_TOKEN_FILE not set")
			}
			appSlug, ok := workspaceAppStatusSlugFromContext(ctx)
			if !ok {
				return "", xerrors.New("workspace app status slug not found in context")
			}
			summary, ok := args["summary"].(string)
			if !ok {
				return "", xerrors.New("summary must be a string")
			}
			if len(summary) > 160 {
				return "", xerrors.New("summary must be less than 160 characters")
			}
			link, ok := args["link"].(string)
			if !ok {
				return "", xerrors.New("link must be a string")
			}
			emoji, ok := args["emoji"].(string)
			if !ok {
				return "", xerrors.New("emoji must be a string")
			}
			state, ok := args["state"].(string)
			if !ok {
				return "", xerrors.New("state must be a string")
			}

			if err := agentClient.PatchAppStatus(ctx, agentsdk.PatchAppStatus{
				AppSlug:            appSlug,
				Message:            summary,
				URI:                link,
				Icon:               emoji,
				NeedsUserAttention: false, // deprecated, to be removed later
				State:              codersdk.WorkspaceAppStatusState(state),
			}); err != nil {
				return "", err
			}
			return "Thanks for reporting!", nil
		},
	}

	GetWorkspace = Tool[codersdk.Workspace]{
		Tool: aisdk.Tool{
			Name: "coder_get_workspace",
			Description: `Get a workspace by ID.

This returns more data than list_workspaces to reduce token usage.`,
			Schema: aisdk.Schema{
				Properties: map[string]any{
					"workspace_id": map[string]any{
						"type": "string",
					},
				},
				Required: []string{"workspace_id"},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (codersdk.Workspace, error) {
			client, err := clientFromContext(ctx)
			if err != nil {
				return codersdk.Workspace{}, err
			}
			workspaceID, err := uuidFromArgs(args, "workspace_id")
			if err != nil {
				return codersdk.Workspace{}, err
			}
			return client.Workspace(ctx, workspaceID)
		},
	}

	CreateWorkspace = Tool[codersdk.Workspace]{
		Tool: aisdk.Tool{
			Name: "coder_create_workspace",
			Description: `Create a new workspace in Coder.

If a user is asking to "test a template", they are typically referring
to creating a workspace from a template to ensure the infrastructure
is provisioned correctly and the agent can connect to the control plane.
`,
			Schema: aisdk.Schema{
				Properties: map[string]any{
					"user": map[string]any{
						"type":        "string",
						"description": "Username or ID of the user to create the workspace for. Use the `me` keyword to create a workspace for the authenticated user.",
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
		Handler: func(ctx context.Context, args map[string]any) (codersdk.Workspace, error) {
			client, err := clientFromContext(ctx)
			if err != nil {
				return codersdk.Workspace{}, err
			}
			templateVersionID, err := uuidFromArgs(args, "template_version_id")
			if err != nil {
				return codersdk.Workspace{}, err
			}
			name, ok := args["name"].(string)
			if !ok {
				return codersdk.Workspace{}, xerrors.New("workspace name must be a string")
			}
			workspace, err := client.CreateUserWorkspace(ctx, "me", codersdk.CreateWorkspaceRequest{
				TemplateVersionID: templateVersionID,
				Name:              name,
			})
			if err != nil {
				return codersdk.Workspace{}, err
			}
			return workspace, nil
		},
	}

	ListWorkspaces = Tool[[]MinimalWorkspace]{
		Tool: aisdk.Tool{
			Name:        "coder_list_workspaces",
			Description: "Lists workspaces for the authenticated user.",
			Schema: aisdk.Schema{
				Properties: map[string]any{
					"owner": map[string]any{
						"type":        "string",
						"description": "The owner of the workspaces to list. Use \"me\" to list workspaces for the authenticated user. If you do not specify an owner, \"me\" will be assumed by default.",
					},
				},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) ([]MinimalWorkspace, error) {
			client, err := clientFromContext(ctx)
			if err != nil {
				return nil, err
			}
			owner, ok := args["owner"].(string)
			if !ok {
				owner = codersdk.Me
			}
			workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
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

	ListTemplates = Tool[[]MinimalTemplate]{
		Tool: aisdk.Tool{
			Name:        "coder_list_templates",
			Description: "Lists templates for the authenticated user.",
			Schema: aisdk.Schema{
				Properties: map[string]any{},
				Required:   []string{},
			},
		},
		Handler: func(ctx context.Context, _ map[string]any) ([]MinimalTemplate, error) {
			client, err := clientFromContext(ctx)
			if err != nil {
				return nil, err
			}
			templates, err := client.Templates(ctx, codersdk.TemplateFilter{})
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

	ListTemplateVersionParameters = Tool[[]codersdk.TemplateVersionParameter]{
		Tool: aisdk.Tool{
			Name:        "coder_template_version_parameters",
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
		Handler: func(ctx context.Context, args map[string]any) ([]codersdk.TemplateVersionParameter, error) {
			client, err := clientFromContext(ctx)
			if err != nil {
				return nil, err
			}
			templateVersionID, err := uuidFromArgs(args, "template_version_id")
			if err != nil {
				return nil, err
			}
			parameters, err := client.TemplateVersionRichParameters(ctx, templateVersionID)
			if err != nil {
				return nil, err
			}
			return parameters, nil
		},
	}

	GetAuthenticatedUser = Tool[codersdk.User]{
		Tool: aisdk.Tool{
			Name:        "coder_get_authenticated_user",
			Description: "Get the currently authenticated user, similar to the `whoami` command.",
			Schema: aisdk.Schema{
				Properties: map[string]any{},
				Required:   []string{},
			},
		},
		Handler: func(ctx context.Context, _ map[string]any) (codersdk.User, error) {
			client, err := clientFromContext(ctx)
			if err != nil {
				return codersdk.User{}, err
			}
			return client.User(ctx, "me")
		},
	}

	CreateWorkspaceBuild = Tool[codersdk.WorkspaceBuild]{
		Tool: aisdk.Tool{
			Name:        "coder_create_workspace_build",
			Description: "Create a new workspace build for an existing workspace. Use this to start, stop, or delete.",
			Schema: aisdk.Schema{
				Properties: map[string]any{
					"workspace_id": map[string]any{
						"type": "string",
					},
					"transition": map[string]any{
						"type":        "string",
						"description": "The transition to perform. Must be one of: start, stop, delete",
					},
				},
				Required: []string{"workspace_id", "transition"},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (codersdk.WorkspaceBuild, error) {
			client, err := clientFromContext(ctx)
			if err != nil {
				return codersdk.WorkspaceBuild{}, err
			}
			workspaceID, err := uuidFromArgs(args, "workspace_id")
			if err != nil {
				return codersdk.WorkspaceBuild{}, err
			}
			rawTransition, ok := args["transition"].(string)
			if !ok {
				return codersdk.WorkspaceBuild{}, xerrors.New("transition must be a string")
			}
			return client.CreateWorkspaceBuild(ctx, workspaceID, codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransition(rawTransition),
			})
		},
	}

	CreateTemplateVersion = Tool[codersdk.TemplateVersion]{
		Tool: aisdk.Tool{
			Name: "coder_create_template_version",
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

The agent MUST be installed and started using the init_script.

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
  service_account {
    email  = data.google_compute_default_service_account.default.email
    scopes = ["cloud-platform"]
  }
  # The startup script runs as root with no $HOME environment set up, so instead of directly
  # running the agent init script, create a user (with a homedir, default shell and sudo
  # permissions) and execute the init script as that user.
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
		Handler: func(ctx context.Context, args map[string]any) (codersdk.TemplateVersion, error) {
			client, err := clientFromContext(ctx)
			if err != nil {
				return codersdk.TemplateVersion{}, err
			}
			me, err := client.User(ctx, "me")
			if err != nil {
				return codersdk.TemplateVersion{}, err
			}
			fileID, err := uuidFromArgs(args, "file_id")
			if err != nil {
				return codersdk.TemplateVersion{}, err
			}
			var templateID uuid.UUID
			if args["template_id"] != nil {
				templateID, err = uuidFromArgs(args, "template_id")
				if err != nil {
					return codersdk.TemplateVersion{}, err
				}
			}
			templateVersion, err := client.CreateTemplateVersion(ctx, me.OrganizationIDs[0], codersdk.CreateTemplateVersionRequest{
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

	GetWorkspaceAgentLogs = Tool[[]string]{
		Tool: aisdk.Tool{
			Name: "coder_get_workspace_agent_logs",
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
		Handler: func(ctx context.Context, args map[string]any) ([]string, error) {
			client, err := clientFromContext(ctx)
			if err != nil {
				return nil, err
			}
			workspaceAgentID, err := uuidFromArgs(args, "workspace_agent_id")
			if err != nil {
				return nil, err
			}
			logs, closer, err := client.WorkspaceAgentLogsAfter(ctx, workspaceAgentID, 0, false)
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

	GetWorkspaceBuildLogs = Tool[[]string]{
		Tool: aisdk.Tool{
			Name: "coder_get_workspace_build_logs",
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
		Handler: func(ctx context.Context, args map[string]any) ([]string, error) {
			client, err := clientFromContext(ctx)
			if err != nil {
				return nil, err
			}
			workspaceBuildID, err := uuidFromArgs(args, "workspace_build_id")
			if err != nil {
				return nil, err
			}
			logs, closer, err := client.WorkspaceBuildLogsAfter(ctx, workspaceBuildID, 0)
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

	GetTemplateVersionLogs = Tool[[]string]{
		Tool: aisdk.Tool{
			Name:        "coder_get_template_version_logs",
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
		Handler: func(ctx context.Context, args map[string]any) ([]string, error) {
			client, err := clientFromContext(ctx)
			if err != nil {
				return nil, err
			}
			templateVersionID, err := uuidFromArgs(args, "template_version_id")
			if err != nil {
				return nil, err
			}

			logs, closer, err := client.TemplateVersionLogsAfter(ctx, templateVersionID, 0)
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

	UpdateTemplateActiveVersion = Tool[string]{
		Tool: aisdk.Tool{
			Name:        "coder_update_template_active_version",
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
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			client, err := clientFromContext(ctx)
			if err != nil {
				return "", err
			}
			templateID, err := uuidFromArgs(args, "template_id")
			if err != nil {
				return "", err
			}
			templateVersionID, err := uuidFromArgs(args, "template_version_id")
			if err != nil {
				return "", err
			}
			err = client.UpdateActiveTemplateVersion(ctx, templateID, codersdk.UpdateActiveTemplateVersion{
				ID: templateVersionID,
			})
			if err != nil {
				return "", err
			}
			return "Successfully updated active version!", nil
		},
	}

	UploadTarFile = Tool[codersdk.UploadResponse]{
		Tool: aisdk.Tool{
			Name:        "coder_upload_tar_file",
			Description: `Create and upload a tar file by key/value mapping of file names to file contents. Use this to create template versions. Reference the tool description of "create_template_version" to understand template requirements.`,
			Schema: aisdk.Schema{
				Properties: map[string]any{
					"mime_type": map[string]any{
						"type": "string",
					},
					"files": map[string]any{
						"type":        "object",
						"description": "A map of file names to file contents.",
					},
				},
				Required: []string{"mime_type", "files"},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (codersdk.UploadResponse, error) {
			client, err := clientFromContext(ctx)
			if err != nil {
				return codersdk.UploadResponse{}, err
			}

			files, ok := args["files"].(map[string]any)
			if !ok {
				return codersdk.UploadResponse{}, xerrors.New("files must be a map")
			}

			pipeReader, pipeWriter := io.Pipe()
			go func() {
				defer pipeWriter.Close()
				tarWriter := tar.NewWriter(pipeWriter)
				for name, content := range files {
					contentStr, ok := content.(string)
					if !ok {
						_ = pipeWriter.CloseWithError(xerrors.New("file content must be a string"))
						return
					}
					header := &tar.Header{
						Name: name,
						Size: int64(len(contentStr)),
						Mode: 0o644,
					}
					if err := tarWriter.WriteHeader(header); err != nil {
						_ = pipeWriter.CloseWithError(err)
						return
					}
					if _, err := tarWriter.Write([]byte(contentStr)); err != nil {
						_ = pipeWriter.CloseWithError(err)
						return
					}
				}
				if err := tarWriter.Close(); err != nil {
					_ = pipeWriter.CloseWithError(err)
				}
			}()

			resp, err := client.Upload(ctx, codersdk.ContentTypeTar, pipeReader)
			if err != nil {
				return codersdk.UploadResponse{}, err
			}
			return resp, nil
		},
	}

	CreateTemplate = Tool[codersdk.Template]{
		Tool: aisdk.Tool{
			Name:        "coder_create_template",
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
		Handler: func(ctx context.Context, args map[string]any) (codersdk.Template, error) {
			client, err := clientFromContext(ctx)
			if err != nil {
				return codersdk.Template{}, err
			}
			me, err := client.User(ctx, "me")
			if err != nil {
				return codersdk.Template{}, err
			}
			versionID, err := uuidFromArgs(args, "version_id")
			if err != nil {
				return codersdk.Template{}, err
			}
			name, ok := args["name"].(string)
			if !ok {
				return codersdk.Template{}, xerrors.New("name must be a string")
			}
			displayName, ok := args["display_name"].(string)
			if !ok {
				return codersdk.Template{}, xerrors.New("display_name must be a string")
			}
			description, ok := args["description"].(string)
			if !ok {
				return codersdk.Template{}, xerrors.New("description must be a string")
			}

			template, err := client.CreateTemplate(ctx, me.OrganizationIDs[0], codersdk.CreateTemplateRequest{
				Name:        name,
				DisplayName: displayName,
				Description: description,
				VersionID:   versionID,
			})
			if err != nil {
				return codersdk.Template{}, err
			}
			return template, nil
		},
	}

	DeleteTemplate = Tool[string]{
		Tool: aisdk.Tool{
			Name:        "coder_delete_template",
			Description: "Delete a template. This is irreversible.",
			Schema: aisdk.Schema{
				Properties: map[string]any{
					"template_id": map[string]any{
						"type": "string",
					},
				},
			},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			client, err := clientFromContext(ctx)
			if err != nil {
				return "", err
			}

			templateID, err := uuidFromArgs(args, "template_id")
			if err != nil {
				return "", err
			}
			err = client.DeleteTemplate(ctx, templateID)
			if err != nil {
				return "", err
			}
			return "Successfully deleted template!", nil
		},
	}
)

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

func clientFromContext(ctx context.Context) (*codersdk.Client, error) {
	client, ok := ctx.Value(clientContextKey{}).(*codersdk.Client)
	if !ok {
		return nil, xerrors.New("client required in context")
	}
	return client, nil
}

type clientContextKey struct{}

func WithClient(ctx context.Context, client *codersdk.Client) context.Context {
	return context.WithValue(ctx, clientContextKey{}, client)
}

type agentClientContextKey struct{}

func WithAgentClient(ctx context.Context, client *agentsdk.Client) context.Context {
	return context.WithValue(ctx, agentClientContextKey{}, client)
}

func agentClientFromContext(ctx context.Context) (*agentsdk.Client, error) {
	client, ok := ctx.Value(agentClientContextKey{}).(*agentsdk.Client)
	if !ok {
		return nil, xerrors.New("agent client required in context")
	}
	return client, nil
}

type workspaceAppStatusSlugContextKey struct{}

func WithWorkspaceAppStatusSlug(ctx context.Context, slug string) context.Context {
	return context.WithValue(ctx, workspaceAppStatusSlugContextKey{}, slug)
}

func workspaceAppStatusSlugFromContext(ctx context.Context) (string, bool) {
	slug, ok := ctx.Value(workspaceAppStatusSlugContextKey{}).(string)
	if !ok || slug == "" {
		return "", false
	}
	return slug, true
}

func uuidFromArgs(args map[string]any, key string) (uuid.UUID, error) {
	raw, ok := args[key].(string)
	if !ok {
		return uuid.Nil, xerrors.Errorf("%s must be a string", key)
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("failed to parse %s: %w", key, err)
	}
	return id, nil
}
