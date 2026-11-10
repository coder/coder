---
title: Migrate from Microsoft Dev Box to Coder
---

This guide is for platform engineers who run Microsoft Dev Box and want to move their developer environments to a self-hosted Coder deployment.
It maps each Dev Box concept to its Coder equivalent, then walks through the migration, with Windows workspaces on Azure as the main example.

Microsoft's [Dev Box retirement guide](https://learn.microsoft.com/azure/dev-box/dev-box-retirement-guide) states that the Dev Box closing-down period begins on September 14, 2026, and that the service retires on September 18, 2028.
The guide also states that Microsoft expects to delete remaining customer workloads after retirement.
Plan to move developer data before the retirement date.

## Before you begin

You need:

- Read access to your dev centers, projects, and pools in the Azure portal, or the Azure CLI with the `devcenter` extension.
- An Azure subscription where Coder can create virtual machines, disks, and network interfaces.
- Permission to create an app registration in Microsoft Entra ID.
- A Coder [Premium license](../../prepare/licensing.md) if you plan to use the features marked Premium in this guide.

## What carries over

Dev Box runs dev boxes in a Microsoft-owned subscription and connects them to your network through a network connection.
Coder has no vendor-hosted component.
You run the [control plane](../architecture.md), and each [template](../../../templates/index.md) provisions workspaces in your own Azure subscription with Terraform.

You can keep:

- Your Microsoft Entra ID tenant and its users and groups, which Coder uses through OpenID Connect (OIDC).
- Your Azure virtual networks and subnets, which templates attach workspaces to.
- Your images in Azure Marketplace or Azure Compute Gallery, which templates reference directly.
- The scripts behind your customization tasks, which templates or images can run.
- Your Git hosts, including Azure Repos, GitHub, and GitLab.

You rebuild:

- Pools, definitions, and image definitions, as Terraform templates.
- Autostop schedules and cost controls, as template and workspace schedule settings.
- Remote Desktop access, through Coder instead of the Dev Box developer portal.
- Device management, such as Microsoft Intune enrollment, if your policies require it.
  Refer to [Plan for Intune and device policies](#plan-for-intune-and-device-policies).

## Concept mapping

Use this table to find where each Dev Box concept lives in Coder.
The sections that follow cover each area in detail.

| Dev Box                                                                                                                                                            | Coder                                                                                                                                                                                                                   |
|--------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [Dev center](https://learn.microsoft.com/azure/dev-box/concept-dev-box-architecture#dev-center)                                                                    | Your Coder deployment, or an [organization](../../../admin/users/organizations.md) (Premium) for each dev center                                                                                                        |
| [Project](https://learn.microsoft.com/azure/dev-box/concept-dev-box-architecture#project)                                                                          | An [organization](../../../admin/users/organizations.md) (Premium), or a [group](../../../admin/users/groups-roles.md) (Premium) with [template permissions](../../../templates/template-permissions.md) (Premium)      |
| [Dev box pool](https://learn.microsoft.com/azure/dev-box/how-to-manage-dev-box-pools)                                                                              | A [template](../../../templates/index.md), with [parameters](../../../templates/extending-templates/parameters/index.md) or [workspace presets](../../../templates/extending-templates/parameters/workspace-presets.md) |
| [Dev box definition or image definition](https://learn.microsoft.com/azure/dev-box/concept-dev-box-architecture#image-definitions)                                 | The image, VM size, and disks in a template                                                                                                                                                                             |
| [Marketplace image or Azure Compute Gallery image](https://learn.microsoft.com/azure/dev-box/how-to-configure-azure-compute-gallery)                               | The same image, referenced in the template's virtual machine resource                                                                                                                                                   |
| [Team customizations](https://learn.microsoft.com/azure/dev-box/concept-what-are-dev-box-customizations)                                                           | [Startup scripts and modules](../../../templates/extending-templates/index.md) in the template, or a prebuilt image                                                                                                     |
| [User customizations](https://learn.microsoft.com/azure/dev-box/how-to-configure-user-customizations)                                                              | [Dotfiles](../../../workspaces/workspace-dotfiles.md)                                                                                                                                                                   |
| [Network connection](https://learn.microsoft.com/azure/dev-box/how-to-configure-network-connections)                                                               | The virtual network and subnet that the template attaches workspaces to                                                                                                                                                 |
| [Dev box](https://learn.microsoft.com/azure/dev-box/concept-dev-box-architecture#dev-box)                                                                          | [Workspace](../../../workspaces/workspace-management.md)                                                                                                                                                                |
| Developer portal                                                                                                                                                   | Coder dashboard                                                                                                                                                                                                         |
| [Azure CLI `devcenter` extension](https://learn.microsoft.com/azure/dev-box/how-to-install-dev-box-cli)                                                            | [Coder CLI](../../../reference/cli/index.md)                                                                                                                                                                            |
| [DevCenter Project Admin and Dev Box User roles](https://learn.microsoft.com/azure/dev-box/concept-dev-box-role-based-access-control)                              | [Roles](../../../admin/users/groups-roles.md), such as Template Admin, and [template permissions](../../../templates/template-permissions.md) (Premium)                                                                 |
| [Dev box limits](https://learn.microsoft.com/azure/dev-box/tutorial-dev-box-limits)                                                                                | [Quotas](../../../admin/users/quotas.md) (Premium)                                                                                                                                                                      |
| [Autostop schedule](https://learn.microsoft.com/azure/dev-box/how-to-configure-stop-schedule)                                                                      | [Template autostop defaults](../../../templates/managing-templates/schedule.md) and [workspace schedules](../../../workspaces/workspace-scheduling.md)                                                                  |
| [Stop on disconnect](https://learn.microsoft.com/azure/dev-box/how-to-configure-stop-on-disconnect)                                                                | [Activity detection](../../../workspaces/workspace-scheduling.md#activity-detection)                                                                                                                                    |
| [Hibernation](https://learn.microsoft.com/azure/dev-box/how-to-configure-dev-box-hibernation)                                                                      | No equivalent; a workspace stop keeps only the resources the template persists                                                                                                                                          |
| [Auto-delete (preview)](https://learn.microsoft.com/azure/dev-box/how-to-auto-delete-dev-box)                                                                      | [Dormancy threshold and dormancy auto-deletion](../../../templates/managing-templates/schedule.md#dormancy-threshold) (Premium)                                                                                         |
| Remote Desktop through Windows App or a browser ([connect to a dev box](https://learn.microsoft.com/azure/dev-box/quickstart-create-dev-box#connect-to-a-dev-box)) | [Remote desktops](../../../workspaces/workspace-access/remote-desktops.md) over Remote Desktop Protocol (RDP), from a desktop client or a browser                                                                       |
| [Dev tunnels for VS Code](https://learn.microsoft.com/azure/dev-box/how-to-set-up-dev-tunnels)                                                                     | [VS Code](../../../workspaces/workspace-access/vscode.md) and other editors over SSH                                                                                                                                    |
| Microsoft Entra ID sign-in to the developer portal                                                                                                                 | [OIDC with Microsoft Entra ID](../../../admin/users/oidc-auth/microsoft.md)                                                                                                                                             |
| [Intune enrollment](https://learn.microsoft.com/azure/dev-box/concept-dev-box-architecture#microsoft-intune-integration)                                           | No built-in equivalent; refer to [Plan for Intune and device policies](#plan-for-intune-and-device-policies)                                                                                                            |

## Inventory your Dev Box resources

Record the current configuration before you change anything.
You use this inventory to build templates and to confirm that nothing was left behind.

Collect the following from the Azure portal or the Azure CLI:

- **Dev centers and projects**: which teams each project serves, and who holds each role on it.
- **Pools**: the image, compute size, storage size, network connection, region, local administrator setting, and autostop schedule of each pool.
- **Images**: each image definition, dev box definition, Marketplace image, and Azure Compute Gallery image that pools use.
- **Customizations**: the catalogs attached to each dev center and project, and the tasks in each image definition file.
- **Network connections**: the virtual network, subnet, and join type (Microsoft Entra join or Microsoft Entra hybrid join) of each one.
- **Secrets**: each Azure Key Vault secret that customization files reference.
- **Cost controls**: hibernation, stop on disconnect, auto-delete, and dev box limits for each project and pool.

## Deploy the Coder control plane

Deploy Coder in Azure, close to the virtual networks that your network connections use.

1. Size the deployment with the [Coder validated architectures](../sizing/index.md).
1. Install the control plane on [Kubernetes](../../server/kubernetes/index.md), such as Azure Kubernetes Service (AKS), or on an [Azure VM](../../server/cloud/azure-vm.md).
1. Connect an [external PostgreSQL database](../../server/external-database.md), such as Azure Database for PostgreSQL.
1. Set the [access URL and wildcard access URL](../../../admin/setup/index.md).
1. Give the provisioners Azure credentials, as described in [Provider authentication](../../../templates/extending-templates/provider-authentication.md).

If your pools span several regions, deploy [workspace proxies](../../../admin/networking/workspace-proxies.md) (Premium) near each group of developers.

## Connect Microsoft Entra ID

Connect the same Entra ID tenant that Dev Box uses, so developers sign in with their existing accounts.

1. Configure [OIDC with Microsoft Entra ID](../../../admin/users/oidc-auth/microsoft.md).
1. To map Entra ID groups to Coder groups, configure [group sync](../../../admin/users/idp-sync.md) (Premium).
1. If you map each project to its own organization, configure [organization sync](../../../admin/users/idp-sync.md#organization-sync) (Premium).

Coder OIDC controls sign-in to Coder.
It doesn't sign developers in to Windows inside the workspace.
Refer to [Connect over Remote Desktop](#connect-over-remote-desktop) for Windows sign-in.

## Replace pools with templates

A Dev Box pool combines an image, a compute size, storage, a network connection, and schedule settings.
In Coder, a template's Terraform defines the first 4, and the template's schedule settings define the rest.

### Start from the Azure VM (Windows) template

The [Azure VM (Windows)](https://registry.coder.com/templates/coder/azure-windows) template in the Coder Registry is a starting point for Windows workspaces.
It uses the `azurerm` Terraform provider to create a Windows Server virtual machine, a network interface, and a data disk in a resource group for each workspace.
For Linux workspaces, start from the [Azure VM (Linux)](https://registry.coder.com/templates/coder/azure-linux) template instead.

Create one template for each group of pools that share an image and network.
To offer several compute sizes in one template, add a [parameter](../../../templates/extending-templates/parameters/index.md) for the VM size.
To give each team a preconfigured starting point, add a [workspace preset](../../../templates/extending-templates/parameters/workspace-presets.md) for each pool.

### Choose the image

Set the image in the template's `azurerm_windows_virtual_machine` resource:

- For a Marketplace image, set the `publisher`, `offer`, `sku`, and `version` values in `source_image_reference`.
- For an Azure Compute Gallery image, set `source_image_id` to the gallery image ID.

The starter template uses a Windows Server image, and Dev Box supports Windows 10 and Windows 11 client editions.
If you switch the template to a client edition, confirm with Microsoft licensing guidance that you can run it on Azure virtual machines.

### Attach workspaces to your network

The starter template creates a virtual network for each workspace.
To keep the network access that a Dev Box network connection provides, change the template to use your existing subnet.
For example, read the subnet with an `azurerm_subnet` data source and pass its ID to the network interface.

### Keep data across restarts

By default, the starter template deletes the virtual machine when the workspace stops and keeps the data disk mounted as `F:`.
Files outside the data disk don't persist.
The [template README](https://registry.coder.com/templates/coder/azure-windows) describes how to keep the whole virtual machine and deallocate it on stop instead.
Refer to [Resource persistence](../../../templates/extending-templates/resource-persistence.md) to choose what to keep.

### Control who uses each template

Dev Box grants access with the Dev Box User role on a project.
In Coder, use [template permissions](../../../templates/template-permissions.md) (Premium) to limit each template to the groups that need it.
To limit how many workspaces each developer runs, as Dev Box limits do, configure [quotas](../../../admin/users/quotas.md) (Premium).
Quotas give each developer a credit budget from the allowances of their groups, and each workspace spends credits from it.

## Translate customizations

Dev Box team customizations run WinGet, PowerShell, and Git clone tasks from an image definition file.
Coder has no equivalent file, so move each task to the image or to the template:

- **Tasks that rarely change**: bake them into a custom image in your Azure Compute Gallery.
  This keeps workspace startup fast.
- **Tasks that run on every start**: add a [`coder_script`](../../../templates/extending-templates/index.md) to the template.
  On Windows, the Coder agent runs scripts with PowerShell.
- **Developer-specific tasks**: ask developers to move their user customizations to a [dotfiles](../../../workspaces/workspace-dotfiles.md) repository.

Refer to [Speed up your workspaces](../../../workspaces/speed-up-templates.md) for more ways to reduce startup time.

## Configure Git authentication and secrets

Dev Box customization files can read secrets from Azure Key Vault to clone private repositories.
In Coder, developers authorize their own Git accounts through [external authentication](../../../admin/external-auth.md):

1. Create an OAuth application on each Git host, or an Entra ID app registration for Azure DevOps.
1. Add each provider to Coder with the `CODER_EXTERNAL_AUTH_<n>_*` environment variables.
1. For Azure Repos, use the [Azure DevOps (via Entra ID)](../../../admin/external-auth.md#azure-devops-via-entra-id) provider type.
1. Add the external authentication provider to each template.

Move other secrets as follows:

- **Shared secrets**: give the workspace VM a managed identity and read values from Azure Key Vault at startup.
  Do not put secret values in template code or parameters.
  Coder shows parameter values in cleartext to anyone who can view the workspace.
- **Personal secrets**: ask developers to recreate them as [user secrets](../../../workspaces/user-secrets.md).

Refer to [Secrets](../../../admin/security/secrets.md) for more options.

## Connect over Remote Desktop

Dev Box connects developers through Windows App or a browser.
Coder connects to Windows workspaces over RDP through the Coder agent, so workspaces need no public IP address.

The starter template includes the [Windows RDP](https://registry.coder.com/modules/coder/windows-rdp) module, which adds a button that opens an RDP session in the browser.
Developers who prefer a desktop RDP client can connect through Coder Desktop or `coder port-forward`.
Refer to [Remote desktops](../../../workspaces/workspace-access/remote-desktops.md) for each method.

The starter template creates a local administrator account with a generated password and shows both on the workspace page.
It doesn't join the VM to Entra ID, so developers don't sign in to Windows with their Entra ID account.
If your Dev Box pools gave developers standard user accounts, change the template to match.

## Set up editors

Developers who used Dev Box dev tunnels with VS Code can connect to Coder workspaces over SSH instead:

- [VS Code](../../../workspaces/workspace-access/vscode.md), through the Coder extension.
- [JetBrains IDEs](../../../workspaces/workspace-access/jetbrains/toolbox.md), through JetBrains Toolbox.
- [Other editors](../../../workspaces/workspace-access/index.md), after developers run `coder config-ssh`.

Developers who run Visual Studio or other desktop tools inside Windows keep using them over RDP.

## Replace schedules and hibernation

Recreate each cost control in your inventory with the matching Coder setting:

- **Autostop schedule**: Dev Box stops each pool at one time of day.
  In Coder, set a [default autostop](../../../templates/managing-templates/schedule.md#schedule) on the template, and let developers set [autostart and autostop](../../../workspaces/workspace-scheduling.md).
- **Stop on disconnect**: Coder extends the autostop deadline by the template's activity bump duration when it detects activity.
  It stops the workspace after the deadline passes with no activity.
  Traffic through the Coder agent counts as activity, including RDP sessions that connect through Coder.
  Refer to [activity detection](../../../workspaces/workspace-scheduling.md#activity-detection).
- **Hibernation**: Coder has no equivalent.
  Open applications don't survive a workspace stop, and the template decides which disks persist.
- **Auto-delete**: configure the [dormancy threshold](../../../templates/managing-templates/schedule.md#dormancy-threshold) and [dormancy auto-deletion](../../../templates/managing-templates/schedule.md#dormancy-auto-deletion) (Premium).
- **Required restarts**: to apply image and template updates on a schedule, configure an [autostop requirement](../../../templates/managing-templates/schedule.md#autostop-requirement) (Premium).

## Plan for Intune and device policies

Dev Box enrolls each dev box in Microsoft Intune and joins it to Entra ID or Active Directory through the network connection.
Coder doesn't enroll workspaces in Intune or join them to a directory.

If your security policies require managed devices, configure the device join and Intune enrollment in the image or the template.
For Entra ID join and Entra ID sign-in on Azure virtual machines, follow Microsoft's guide to [sign in to a Windows VM with Microsoft Entra ID](https://learn.microsoft.com/entra/identity/devices/howto-vm-sign-in-azure-ad-windows).
If you use conditional access policies that require a compliant device, test them from a pilot workspace before you move developers.

## Pilot and cut over

1. Run Coder and Dev Box side by side.
1. Move one team and one pool to Coder first.
1. Compare workspace startup time and developer feedback with the same pool on Dev Box.
1. Ask developers to commit their work and copy local files they need from each dev box.
1. Move the remaining pools one at a time.
1. After all developers move, delete your Dev Box resources by following the [Dev Box retirement guide](https://learn.microsoft.com/azure/dev-box/dev-box-retirement-guide).

For a summary comparison of the 2 products, visit the [Coder comparison page](https://coder.com/solutions/workspaces/compare).

## Learn more

- [Creating templates](../../../templates/creating-templates.md)
- [Remote desktops](../../../workspaces/workspace-access/remote-desktops.md)
- [OIDC with Microsoft Entra ID](../../../admin/users/oidc-auth/microsoft.md)
- [Workspace scheduling](../../../templates/managing-templates/schedule.md)
