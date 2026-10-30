---
title: Migrate from Ona (formerly Gitpod) to Coder
---

This guide is for platform engineers who run an Ona Enterprise organization with runners in AWS or Google Cloud and want to move it to a self-hosted Coder deployment.
It maps each part of an Ona deployment to its Coder equivalent, then walks through the migration in the order most teams do it.

Ona was previously named Gitpod, and some Ona paths, such as the `/home/gitpod` home directory, still use the old name.
This guide uses "Ona" throughout.

## Before you begin

You need:

- Organization admin access to Ona, so you can read runner, project, policy, and integration settings.
- A Coder [Premium license](../install/prepare/licensing.md) if you plan to use the features marked Premium in this guide.
  Most organization-wide policy, identity, and governance features that Ona Enterprise includes require Premium.
- A Kubernetes cluster or VM and a PostgreSQL database in the cloud account where you want the Coder control plane to run.
- Permission to create an OAuth application on each Git host your developers use.

## How the two architectures differ

Ona splits its platform into 2 planes.
Ona hosts the [management plane](https://ona.com/docs/ona/understanding/architecture), which handles authentication, organization settings, guardrails, and coordination.
Runners in your AWS or Google Cloud account provision environments, clone code, inject secrets, and run agents.

Coder has no vendor-hosted component.
You run the [control plane](../install/plan/architecture.md) and its PostgreSQL database, and Coder provisions workspaces from [templates](../admin/templates/index.md) that you write in Terraform.
The [Coder Agents](../ai-coder/agents/index.md) agent loop also runs in your control plane.

In practice, migrating means 2 things:

- Identity, organization settings, policies, audit data, and agent configuration move from Ona's management plane into a deployment you operate.
- Runner configuration, environment classes, and projects become Terraform templates.

## Concept mapping

Use this table to find where each Ona concept lives in Coder.
The sections that follow cover each area in detail.

| Ona                                                                                | Coder                                                                                                                                                                                        |
|------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Management plane                                                                   | [Coder control plane](../install/plan/architecture.md) and PostgreSQL, which you host                                                                                                        |
| [Runner](https://ona.com/docs/ona/runners/overview) (AWS or GCP)                   | The infrastructure a template provisions, plus [provisioners](../install/operate/provisioners/index.md) that run Terraform                                                                   |
| Environment class                                                                  | A template [parameter](../admin/templates/extending-templates/parameters/index.md) for instance size                                                                                         |
| [Project](https://ona.com/docs/ona/projects/overview)                              | A [template](../admin/templates/index.md), with one [workspace preset](../admin/templates/extending-templates/parameters/workspace-presets.md) per repository                                |
| Environment                                                                        | [Workspace](../user-guides/workspace-management.md)                                                                                                                                          |
| `devcontainer.json`                                                                | [Dev Containers integration](../admin/integrations/devcontainers/integration.md) or [Envbuilder](../admin/integrations/devcontainers/envbuilder/index.md)                                    |
| Tasks and services in `.ona/config.yaml`                                           | Dev container lifecycle commands, [`coder_script`, and `coder_app`](../admin/templates/extending-templates/index.md)                                                                         |
| [Prebuilds](https://ona.com/docs/ona/projects/prebuilds) and warm pools            | [Envbuilder caching](../admin/integrations/devcontainers/envbuilder/envbuilder-security-caching.md) and [prebuilt workspaces](../admin/templates/extending-templates/prebuilt-workspaces.md) |
| Organization and project secrets                                                   | Values that templates inject from your secret manager (refer to [Secrets](../admin/security/secrets.md))                                                                                     |
| User secrets                                                                       | [User secrets](../user-guides/user-secrets.md)                                                                                                                                               |
| Git providers configured on each runner                                            | [External authentication](../admin/external-auth.md), configured once for the deployment                                                                                                     |
| Codex Agent                                                                        | [Coder Agents](../ai-coder/agents/index.md), or Codex CLI running in a workspace                                                                                                             |
| [Automations](https://ona.com/docs/ona/automations/overview)                       | The [Chats API](../reference/api/chats.md), called from CI, schedulers, or webhooks                                                                                                          |
| Custom domain                                                                      | [Access URL and wildcard access URL](../admin/setup/index.md)                                                                                                                                |
| [Ona Terraform provider](https://ona.com/docs/ona/integrations/terraform-provider) | [`coderd` Terraform provider](https://registry.terraform.io/providers/coder/coderd/latest/docs)                                                                                              |

## Inventory your Ona organization

Record the current configuration before you change anything.
You use this inventory to build templates and to confirm that nothing was left behind.

Collect the following from the Ona dashboard, the [Ona CLI](https://ona.com/docs/ona/integrations/cli), or your Ona Terraform configuration:

- **Runners**: cloud, account, region, VPC, network mode (such as AWS PrivateLink or an internal load balancer), proxy, and custom CA settings.
- **Environment classes**: the instance type, disk size, and GPU settings for each runner.
- **Projects**: repository, default branch, Dev Container path, tasks and services path, allowed environment classes, recommended editors, and who the project is shared with.
- **Secrets**: the name, scope (organization, project, or user), and type (environment variable, file, or container registry) of each secret.
  Collect secret values from their source of truth, such as your secret manager, instead of from Ona.
- **Policies**: every setting under **Settings** > **Organization** > **Policies**.
- **Identity**: SSO providers, verified domains, SCIM configuration, groups, teams, organization roles, and service accounts.
- **Git providers**: each Git host configured on each runner, and whether it uses OAuth, personal access tokens, or both.
- **Agents**: model access configuration, LLM integrations, MCP integrations, and any `AGENTS.md` files and skills in your repositories.
- **Automations**: each Automation's trigger, steps, target repositories, and the identity it runs as.
  If you manage [Automations as code](https://ona.com/docs/ona/automations/automations-as-code), the YAML files in your repositories already capture this.

## Deploy the Coder control plane

Deploy Coder in the same cloud account and network as your Ona runners.
Workspaces then reach the same internal services, package registries, and databases that your Ona environments use today.

1. Size the deployment with the [Coder validated architectures](../install/plan/sizing/index.md).
1. Install the control plane on [Kubernetes](../install/server/kubernetes/index.md) or on a [cloud VM](../install/server/cloud/index.md).
1. Connect an [external PostgreSQL database](./external-database.md), such as Amazon RDS or Cloud SQL.
1. Set the [access URL and wildcard access URL](../admin/setup/index.md) to replace the Ona custom domain.
1. If developers depend on the deployment for daily work, run multiple replicas for [high availability](../admin/networking/high-availability.md).

If your Ona runners span several regions, deploy [workspace proxies](../admin/networking/workspace-proxies.md) (Premium) near each group of developers.
If your network has no outbound internet access, follow the [air-gapped deployment](../install/prepare/airgap.md) guide.

## Connect your identity provider

Connect the same identity provider that Ona uses so that developers sign in with their existing accounts.

| Ona                                                                   | Coder                                                                                                                                        |
|-----------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------|
| [SSO](https://ona.com/docs/ona/sso/overview) through an OIDC provider | [OpenID Connect](../admin/users/oidc-auth/index.md)                                                                                          |
| SCIM provisioning                                                     | [SCIM](../admin/users/oidc-auth/index.md#scim) (Premium)                                                                                     |
| Groups and teams                                                      | [Groups](../admin/users/groups-roles.md) (Premium), synced from your identity provider with [IdP sync](../admin/users/idp-sync.md) (Premium) |
| Organization roles                                                    | [Roles and custom roles](../admin/users/groups-roles.md) (Premium)                                                                           |
| Multiple Ona organizations                                            | [Organizations](../admin/users/organizations.md) (Premium)                                                                                   |
| Service accounts                                                      | [Service accounts](../admin/users/headless-auth.md) (Premium)                                                                                |

Create a Coder service account for each Ona service account that runs Automations or prebuilds.
You use their API tokens later, when you move Automations to the Chats API.

## Configure Git providers

Ona configures Git providers on each runner, and each developer authorizes their own account.
Coder configures Git providers once for the deployment through [external authentication](../admin/external-auth.md), and each developer still authorizes their own account.

1. Create an OAuth application on each Git host in your inventory.
1. Add each one to Coder with the `CODER_EXTERNAL_AUTH_<n>_*` environment variables.
1. For self-hosted GitHub Enterprise, set `API_BASE_URL` as described in [Git providers for Coder Agents](../ai-coder/agents/platform-controls/git-providers.md).
1. Add the external authentication provider to each template so that workspaces can clone and push.

Coder supports GitHub, GitLab, Bitbucket, and Azure DevOps, which covers the Git providers that Ona supports.

## Replace runners and environment classes with templates

An Ona runner decides where environments run, and its environment classes decide how large they are.
In Coder, the Terraform in each template makes both decisions.

### Choose where workspaces run

Ona environments run as EC2 instances on AWS runners and as Compute Engine instances on GCP runners.
To keep the same isolation model, start from a VM-based template for the same cloud.
The Coder Registry has starting points for [AWS](https://registry.coder.com/templates/coder/aws-envbuilder), [Google Cloud](https://registry.coder.com/templates/coder/gcp-envbuilder), and [Kubernetes](https://registry.coder.com/templates/coder/kubernetes-envbuilder).

The control plane runs Terraform with its built-in provisioners by default.
If different regions or accounts need separate credentials or network access, run [external provisioners](../install/operate/provisioners/index.md) in each one and route templates to them with provisioner tags.
Give provisioners cloud credentials through the environment they run in, as described in [Provider authentication](../admin/templates/extending-templates/provider-authentication.md).

### Recreate environment classes

Replace each environment class with an option on a template parameter.
This example matches the default AWS runner classes:

```tf
data "coder_parameter" "instance_type" {
  name         = "instance_type"
  display_name = "Instance type"
  type         = "string"
  default      = "m6i.xlarge"
  mutable      = false

  option {
    name  = "Small (2 vCPU, 8 GiB)"
    value = "m6i.large"
  }
  option {
    name  = "Regular (4 vCPU, 16 GiB)"
    value = "m6i.xlarge"
  }
  option {
    name  = "Large (8 vCPU, 32 GiB)"
    value = "m6i.2xlarge"
  }
}
```

Pass the value to the instance resource in your template, such as `instance_type = data.coder_parameter.instance_type.value`.

### Recreate projects

An Ona project combines a repository, a Dev Container path, and a set of allowed environment classes.
In Coder, you can represent each project as a [workspace preset](../admin/templates/extending-templates/parameters/workspace-presets.md) on a shared template:

```tf
data "coder_workspace_preset" "payments_api" {
  name = "payments-api"
  parameters = {
    repo_url      = "https://github.example.com/acme/payments-api"
    instance_type = "m6i.xlarge"
  }
}
```

If a project needs different infrastructure, such as GPUs or a separate network, create a separate template for it.
Use [template permissions](../admin/templates/template-permissions.md) (Premium) to limit who can use each template, as Ona project sharing does.

### Keep data across restarts

Ona environments keep their disk across stops and starts.
In Coder, what persists depends on the template.
Many templates keep the home directory on a persistent volume and recreate compute on each start.
Refer to [Resource persistence](../admin/templates/extending-templates/resource-persistence.md) to choose what to keep.

## Bring your Dev Container configuration

Ona builds every environment from a `devcontainer.json` file, so most repositories already have one.
Keep it in place and choose how Coder runs it:

- **[Dev Containers integration](../admin/integrations/devcontainers/integration.md)**: runs the `@devcontainers/cli` and Docker inside the workspace.
  Use it with VM-based templates, where Docker is available.
  Coder shows each dev container in the dashboard and supports multiple dev containers per workspace.
- **[Envbuilder](../admin/integrations/devcontainers/envbuilder/index.md)**: builds the workspace image itself from `devcontainer.json`, without a Docker daemon.
  Use it on Kubernetes, or wherever privileged containers aren't allowed.

This example clones the project repository and starts its dev container with the Dev Containers integration:

```tf
module "git-clone" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/git-clone/coder"
  agent_id = coder_agent.dev.id
  url      = data.coder_parameter.repo_url.value
}

module "devcontainers-cli" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/devcontainers-cli/coder"
  agent_id = coder_agent.dev.id
}

resource "coder_devcontainer" "project" {
  count            = data.coder_workspace.me.start_count
  agent_id         = coder_agent.dev.id
  workspace_folder = module.git-clone[0].repo_dir
}
```

Before you test, check each `devcontainer.json` and its scripts for these Ona-specific details:

- Hardcoded paths under `/home/gitpod` or `/workspaces/<repository-name>`, which are Ona's [default locations](https://ona.com/docs/ona/environments/persistent-storage).
  Replace them with paths that match your template, or with variables such as `${containerWorkspaceFolder}`.
- `network_mode: host` on Docker Compose services, which Ona requires.
  Keep it only if your services still need it.
- `forwardPorts` entries with `host:port` syntax for Docker Compose sidecar containers, which the Dev Containers integration doesn't support yet.
  Refer to the [Dev Containers limitations](../user-guides/devcontainers/index.md#limitations).

## Translate tasks and services

Ona runs tasks and services from `.ona/config.yaml`.
Coder has no equivalent file, so move each entry to the dev container lifecycle commands or to the template.

| Ona trigger             | When Ona runs it                                 | Coder equivalent                                                                                                                                 |
|-------------------------|--------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------|
| `prebuild`              | During prebuilds only                            | `onCreateCommand` or `updateContentCommand` in `devcontainer.json`                                                                               |
| `postDevcontainerStart` | After the dev container first starts or rebuilds | `postCreateCommand` in `devcontainer.json`                                                                                                       |
| `postEnvironmentStart`  | Every time the environment starts                | `postStartCommand` in `devcontainer.json`, or a `coder_script` with `run_on_start = true`                                                        |
| `manual`                | On demand from the UI or CLI                     | A `coder_app` with a `command`, or a [custom app](../user-guides/devcontainers/customizing-dev-containers.md#custom-apps) in `devcontainer.json` |

Ona services are long-running processes, such as databases and development servers.
In Coder, start them with a `coder_script` and expose their ports with a `coder_app`:

```tf
resource "coder_script" "postgres" {
  agent_id     = coder_agent.dev.id
  display_name = "PostgreSQL"
  run_on_start = true
  script       = "docker compose up -d postgres"
}
```

If the service runs inside a dev container, define it as a [custom app](../user-guides/devcontainers/customizing-dev-containers.md#custom-apps) with a health check in `devcontainer.json` instead.

## Speed up workspace startup

Ona [prebuilds](https://ona.com/docs/ona/projects/prebuilds) snapshot the Dev Container filesystem on a schedule, and [warm pools](https://ona.com/docs/ona/projects/warm-pools) keep instances running so environments start faster.
Coder offers 2 features for the same goal:

- **[Envbuilder caching](../admin/integrations/devcontainers/envbuilder/envbuilder-security-caching.md)**: pushes built image layers to a registry, so later builds of the same `devcontainer.json` skip unchanged steps.
- **[Prebuilt workspaces](../admin/templates/extending-templates/prebuilt-workspaces.md)** (Premium): keeps a pool of running workspaces for each preset.
  When a developer creates a workspace with that preset, Coder assigns them one from the pool, much like an Ona warm pool.

Ona runs prebuilds without user secrets.
In Coder, a prebuilt workspace belongs to the `prebuilds` system user until a developer claims it.
Run steps that need the developer's identity, such as cloning with their Git credentials, after the claim.

Refer to [Speed up your workspaces](./best-practices/speed-up-templates.md) for more ways to reduce startup time.

## Move secrets

Ona has secrets at 3 scopes.
Move each scope as follows:

- **Organization and project secrets**: give the workspace compute a cloud identity, such as an IAM role or service account, and read values from your secret manager at startup.
  Alternatively, use [dynamic secrets](../admin/security/secrets.md#dynamic-secrets) that the template creates for each workspace.
  Do not put secret values in template code or template parameters, because users with template or workspace access can read them.
- **User secrets**: ask developers to recreate them as Coder [user secrets](../user-guides/user-secrets.md).
  Each user secret can target an environment variable, a file path, or both, like Ona environment variable and file secrets.
- **Container registry secrets**: give the provisioner or workspace infrastructure access to the registry.
  For Kubernetes, use an [image pull secret](./image-pull-secret.md).
  For Envbuilder, follow its [security and caching](../admin/integrations/devcontainers/envbuilder/envbuilder-security-caching.md) guidance.

If your environments use Ona [OIDC tokens](https://ona.com/docs/ona/configuration/oidc) to reach AWS or other cloud services, attach a cloud identity to the workspace compute in your template instead.

## Recreate organization policies

Recreate each Ona policy in your inventory with the matching Coder setting.

| Ona policy                                                                                           | Coder setting                                                                                                                                                                                               |
|------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [Auto-stop timeout](https://ona.com/docs/ona/organizations/policies/environment-timeout)             | [Autostop and activity detection](../user-guides/workspace-scheduling.md), with defaults set in [template scheduling](../admin/templates/managing-templates/schedule.md)                                    |
| [Maximum environment lifetime](https://ona.com/docs/ona/organizations/policies/environment-lifetime) | [Autostop requirement](../admin/templates/managing-templates/schedule.md#autostop-requirement) (Premium)                                                                                                    |
| [Archive timing and auto-delete](https://ona.com/docs/ona/environments/archive-auto-delete)          | [Dormancy threshold](../admin/templates/managing-templates/schedule.md#dormancy-threshold) and [dormancy auto-deletion](../admin/templates/managing-templates/schedule.md#dormancy-auto-deletion) (Premium) |
| Environment limits                                                                                   | [Quotas](../admin/users/quotas.md) (Premium), which limit workspaces by a per-group budget                                                                                                                  |
| Project creation                                                                                     | [Template permissions](../admin/templates/template-permissions.md) and [roles](../admin/users/groups-roles.md) (Premium)                                                                                    |
| Editor restrictions                                                                                  | The IDE apps and modules that you include in each template                                                                                                                                                  |
| Port sharing                                                                                         | [Maximum port sharing level](../admin/networking/port-forwarding.md#configure-maximum-port-sharing-level) (Premium)                                                                                         |
| Default image                                                                                        | The base image or default `devcontainer.json` in each template                                                                                                                                              |
| [Security agents](https://ona.com/docs/ona/organizations/policies/security-agents)                   | The workspace image or template, where you install the security agent                                                                                                                                       |
| [Command deny list](https://ona.com/docs/ona/command-deny-list) and executable deny list             | [Agent Firewall](../ai-coder/agent-firewall/index.md) (Premium) for agent network access, and your workspace image for executables                                                                          |

## Set up editors and dotfiles

Ona connects editors to environments over SSH.
Coder does the same, so developers keep the editors they use today:

- [Visual Studio Code](../user-guides/workspace-access/vscode.md), through the Coder extension.
- [Cursor](../user-guides/workspace-access/cursor.md), through the [Cursor module](https://registry.coder.com/modules/coder/cursor).
- [JetBrains IDEs](../user-guides/workspace-access/jetbrains/toolbox.md), through JetBrains Toolbox.
- [VS Code in the browser](../user-guides/workspace-access/web-ides.md), through code-server or VS Code Web.
- [Zed](../user-guides/workspace-access/zed.md) and other SSH editors, after developers run `coder config-ssh`.

Developers can keep their Ona dotfiles repository.
Ona and Coder both clone the repository and run the first install script they find, such as `install.sh`, `bootstrap.sh`, or `setup.sh`.
Add the [dotfiles module](https://registry.coder.com/modules/coder/dotfiles) to your templates, then refer developers to [Dotfiles](../user-guides/workspace-dotfiles.md).

If your repositories have a "Build with Ona" README button, replace it with an [Open in Coder](../admin/templates/open-in-coder.md) link.

## Move agent workloads

Ona runs the Codex harness as its agent, with OpenAI model access through Ona Intelligence or an approved provider integration.
In Coder, choose one of 2 approaches, or use both:

- **Coder Agents**: the agent loop runs in the control plane, so workspaces need no agent software or LLM credentials.
  It works with Anthropic, OpenAI, Google, Azure, AWS Bedrock, and any OpenAI-compatible endpoint.
- **Codex CLI in a workspace**: install it with the [Codex module](https://registry.coder.com/modules/coder-labs/codex), and route its traffic through [AI Gateway](../ai-coder/ai-gateway/clients/codex.md) (Premium) for centralized keys and auditing.

### Configure Coder Agents

1. Add your LLM providers and models, as described in [Getting started](../ai-coder/agents/getting-started.md).
   If your Ona runners used Bedrock or a direct OpenAI integration, you can reuse that account.
1. Write clear template descriptions, because Coder Agents uses them to choose a template for each task.
1. Set a deployment system prompt and organization instructions in [Platform controls](../ai-coder/agents/platform-controls/index.md).
1. Register the MCP servers from your Ona organization integrations as [MCP servers](../ai-coder/agents/platform-controls/mcp-servers.md).
1. Keep the `AGENTS.md` files in your repositories.
1. Move skills to a location that Coder Agents scans, as listed in [Extending agents](../ai-coder/agents/extending-agents.md).

Community and Premium licenses allow up to 5 concurrently active agents.
If you plan to replace Ona Automations that run many agents at once, review [Licensing and usage](../ai-coder/agents/licensing-usage.md).

### Replace Automations

Ona Automations start agents from manual, pull request, schedule, and webhook triggers.
In Coder, start agents with the [Chats API](../reference/api/chats.md) from the system that owns the trigger:

| Ona trigger  | Where to call the Chats API                  |
|--------------|----------------------------------------------|
| Manual       | The Coder Agents chat interface, or a script |
| Pull request | A CI job that runs on pull request events    |
| Schedule     | A scheduled CI job or another scheduler      |
| Webhook      | The service that receives the webhook        |

Authenticate these calls with an API token for a [service account](../admin/users/headless-auth.md) (Premium).
Refer to [Use the API for programmatic automation](../ai-coder/agents/getting-started.md#use-the-api-for-programmatic-automation) for an example request.

To start agents from Slack or Linear, call the Chats API from those tools' webhook or automation features.

## Set up auditing

Turn on Coder [audit logs](../admin/security/audit-logs.md) (Premium) to replace Ona audit logs.
If you use [AI Gateway](../ai-coder/ai-gateway/index.md) (Premium), it records prompts, token usage, and tool invocations for each user.
[Agent Firewall](../ai-coder/agent-firewall/index.md) (Premium) also streams agent network activity to the control plane.

Before you close your Ona organization, export any Ona [audit logs](https://ona.com/docs/ona/audit-logs/overview) that you must retain.

## Pilot and cut over

1. Run Coder and Ona side by side.
1. Move one team and one project to Coder first.
1. Compare workspace startup time and developer feedback with the same project on Ona.
1. Move the remaining projects one at a time, starting with those that have the simplest `.ona/config.yaml`.
1. Move each Automation after the projects it targets.
1. After all developers and Automations move, remove your Ona runners and their cloud resources by following the [Ona runner documentation](https://ona.com/docs/ona/runners/overview).

## Learn more

- [Creating templates](../admin/templates/creating-templates.md)
- [Dev Containers](../admin/integrations/devcontainers/index.md)
- [Coder Agents](../ai-coder/agents/index.md)
- [Security best practices](./best-practices/security-best-practices.md)
