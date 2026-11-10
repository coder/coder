---
title: Migrate from Microsoft Dev Box to Coder
---

This guide is for platform engineers who run Microsoft Dev Box and want to move their developer environments to a self-hosted Coder deployment.
It maps each Dev Box concept to its Coder equivalent, then walks through the migration, with Windows workspaces on Azure as the main example.

Microsoft's [Dev Box retirement guide](https://learn.microsoft.com/azure/dev-box/dev-box-retirement-guide) states that the Dev Box closing-down period began on September 14, 2026, and that the service retires on September 18, 2028.
The guide also states that Microsoft expects to delete remaining customer workloads after retirement.
Plan to move developer data before the retirement date.

## Before you begin

You need:

- Read access to your dev centers, projects, and pools in the Azure portal, or the Azure CLI with the `devcenter` extension.
- An Azure subscription where Coder can create resource groups, virtual machines, disks, virtual networks, network interfaces, and storage accounts.
  The starter template creates all of these for each workspace.
- Permission to create an app registration in Microsoft Entra ID.
- A Coder [Premium license](../../prepare/licensing.md) if you plan to use the features marked Premium in this guide.

## What carries over

Dev Box runs dev boxes in a Microsoft-owned subscription and connects them to your network through a network connection.
Coder has no vendor-hosted control plane.
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
  Refer to [Step 10: Plan for Intune and device policies](#step-10-plan-for-intune-and-device-policies).

## Concept mapping

Use this table to find where each Dev Box concept lives in Coder.
The sections that follow cover each area in detail.

| Dev Box                                                                                                                                                            | Coder                                                                                                                                                                                                                                    |
|--------------------------------------------------------------------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [Dev center](https://learn.microsoft.com/azure/dev-box/concept-dev-box-architecture#dev-center)                                                                    | Your Coder deployment, or an [organization](../../../admin/users/organizations.md) (Premium) for each dev center                                                                                                                         |
| [Project](https://learn.microsoft.com/azure/dev-box/concept-dev-box-architecture#project)                                                                          | An [organization](../../../admin/users/organizations.md) (Premium), or a [group](../../../admin/users/groups-roles.md) (Premium) with [template permissions](../../../templates/template-permissions.md) (Premium)                       |
| [Dev box pool](https://learn.microsoft.com/azure/dev-box/how-to-manage-dev-box-pools)                                                                              | A [template](../../../templates/index.md), with [parameters](../../../templates/extending-templates/parameters/index.md) or [workspace presets](../../../templates/extending-templates/parameters/workspace-presets.md)                  |
| [Dev box definition or image definition](https://learn.microsoft.com/azure/dev-box/concept-dev-box-architecture#image-definitions)                                 | The image, VM size, and disks in a template                                                                                                                                                                                              |
| [Marketplace image or Azure Compute Gallery image](https://learn.microsoft.com/azure/dev-box/how-to-configure-azure-compute-gallery)                               | The same image, referenced in the template's virtual machine resource                                                                                                                                                                    |
| [Team customizations](https://learn.microsoft.com/azure/dev-box/concept-what-are-dev-box-customizations)                                                           | [Startup scripts and modules](../../../templates/extending-templates/index.md) in the template, or a prebuilt image                                                                                                                      |
| [User customizations](https://learn.microsoft.com/azure/dev-box/how-to-configure-user-customizations)                                                              | [Dotfiles](../../../workspaces/workspace-dotfiles.md)                                                                                                                                                                                    |
| [Network connection](https://learn.microsoft.com/azure/dev-box/how-to-configure-network-connections)                                                               | The virtual network and subnet that the template attaches workspaces to                                                                                                                                                                  |
| [Dev box](https://learn.microsoft.com/azure/dev-box/concept-dev-box-architecture#dev-box)                                                                          | [Workspace](../../../workspaces/workspace-management.md)                                                                                                                                                                                 |
| Developer portal                                                                                                                                                   | Coder dashboard                                                                                                                                                                                                                          |
| [Azure CLI `devcenter` extension](https://learn.microsoft.com/azure/dev-box/how-to-install-dev-box-cli)                                                            | [Coder CLI](../../../reference/cli/index.md)                                                                                                                                                                                             |
| [DevCenter Project Admin and Dev Box User roles](https://learn.microsoft.com/azure/dev-box/concept-dev-box-role-based-access-control)                              | [Organization roles](../../../admin/users/groups-roles.md#organization-roles), such as Organization Template Admin, for Project Admin, and [template permissions](../../../templates/template-permissions.md) (Premium) for Dev Box User |
| [Dev box limits](https://learn.microsoft.com/azure/dev-box/tutorial-dev-box-limits)                                                                                | [Quotas](../../../admin/users/quotas.md) (Premium)                                                                                                                                                                                       |
| [Autostop schedule](https://learn.microsoft.com/azure/dev-box/how-to-configure-stop-schedule)                                                                      | [Autostop requirement](../../../templates/managing-templates/schedule.md#autostop-requirement) and [quiet hours](../../../templates/managing-templates/schedule.md#user-quiet-hours) (Premium)                                           |
| [Stop on disconnect](https://learn.microsoft.com/azure/dev-box/how-to-configure-stop-on-disconnect)                                                                | [Activity detection](../../../workspaces/workspace-scheduling.md#activity-detection)                                                                                                                                                     |
| [Hibernation](https://learn.microsoft.com/azure/dev-box/how-to-configure-dev-box-hibernation)                                                                      | No equivalent; a workspace stop keeps only the resources the template persists                                                                                                                                                           |
| [Auto-delete (preview)](https://learn.microsoft.com/azure/dev-box/how-to-auto-delete-dev-box)                                                                      | [Dormancy threshold and dormancy auto-deletion](../../../templates/managing-templates/schedule.md#dormancy-threshold) (Premium)                                                                                                          |
| Remote Desktop through Windows App or a browser ([connect to a dev box](https://learn.microsoft.com/azure/dev-box/quickstart-create-dev-box#connect-to-a-dev-box)) | [Remote desktops](../../../workspaces/workspace-access/remote-desktops.md) over Remote Desktop Protocol (RDP), from a desktop client or a browser                                                                                        |
| [Dev tunnels for VS Code](https://learn.microsoft.com/azure/dev-box/how-to-set-up-dev-tunnels)                                                                     | [VS Code](../../../workspaces/workspace-access/vscode.md) and other editors over SSH                                                                                                                                                     |
| Microsoft Entra ID sign-in to the developer portal                                                                                                                 | [OIDC with Microsoft Entra ID](../../../admin/users/oidc-auth/microsoft.md)                                                                                                                                                              |
| [Intune enrollment](https://learn.microsoft.com/azure/dev-box/concept-dev-box-architecture#microsoft-intune-integration)                                           | No built-in equivalent; refer to [Step 10: Plan for Intune and device policies](#step-10-plan-for-intune-and-device-policies)                                                                                                            |

## Step 1: Inventory your Dev Box resources

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

## Step 2: Deploy the Coder control plane

Deploy Coder in Azure, close to the virtual networks that your network connections use.

1. Size the deployment with the [Coder validated architectures](../sizing/index.md).
1. Install the control plane on [Kubernetes](../../server/kubernetes/index.md), such as Azure Kubernetes Service (AKS), or on an [Azure VM](../../server/cloud/azure-vm.md).
1. Connect an [external PostgreSQL database](../../server/external-database.md), such as Azure Database for PostgreSQL.
1. Set the [access URL and wildcard access URL](../../../admin/setup/index.md).
1. Give the provisioners Azure credentials, as described in [Provider authentication](../../../templates/extending-templates/provider-authentication.md).

If your pools span several regions, deploy [workspace proxies](../../../admin/networking/workspace-proxies.md) (Premium) near each group of developers.

## Step 3: Connect Microsoft Entra ID

Connect the same Entra ID tenant that Dev Box uses, so developers sign in with their existing accounts.

1. Configure [OIDC with Microsoft Entra ID](../../../admin/users/oidc-auth/microsoft.md).
1. To map Entra ID groups to Coder groups, configure [group sync](../../../admin/users/idp-sync.md) (Premium).
1. If you map each project to its own organization, configure [organization sync](../../../admin/users/idp-sync.md#organization-sync) (Premium).

Coder OIDC controls sign-in to Coder.
It doesn't sign developers in to Windows inside the workspace.
Refer to [Step 7: Connect over Remote Desktop](#step-7-connect-over-remote-desktop) for Windows sign-in.

## Step 4: Replace pools with templates

A Dev Box pool combines an image, a compute size, storage, a network connection, and schedule settings.
In Coder, a template's Terraform defines the first 4, and the template's schedule settings define the rest.

### Start from the Azure VM (Windows) template

The [Azure VM (Windows)](https://registry.coder.com/templates/coder/azure-windows) template in the Coder Registry is a starting point for Windows workspaces.
It uses the `azurerm` Terraform provider to create a Windows Server virtual machine, a network interface, and a data disk in a resource group for each workspace.
For Linux workspaces, start from the [Azure VM (Linux)](https://registry.coder.com/templates/coder/azure-linux) template instead.

If the template's `windows_rdp` module block doesn't set `agent_name`, add `agent_name = "main"`, the name of the template's `coder_agent` resource.
Without it, the template fails to build and reports that `agent_name must be set when enable_native_rdp is true`.

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
The Azure credentials that the provisioners use also need permission to join that subnet.

Workspaces on the subnet need outbound access to your Coder access URL, which the Coder agent uses to connect.
The [Windows RDP](https://registry.coder.com/modules/coder/windows-rdp) module also downloads Devolutions Gateway from the internet each time the workspace starts.
Refer to [Networking](../../../admin/networking/index.md) for the connections that the agent makes.

Pools that use a Microsoft-hosted network have no subnet to reuse, so keep the starter template's virtual network for those pools.

### Keep data across restarts

By default, the starter template deletes the virtual machine when the workspace stops and keeps the data disk mounted as `F:`.
Files outside the data disk don't persist.
The [template README](https://registry.coder.com/templates/coder/azure-windows) describes how to keep the whole virtual machine and deallocate it on stop instead.
That approach needs the Azure CLI (`az`) on the provisioner's `PATH`, and the official Coder images don't include it.
Refer to [Resource persistence](../../../templates/extending-templates/resource-persistence.md) to choose what to keep.

### Control who uses each template

Dev Box grants access with the Dev Box User role on a project.
In Coder, use [template permissions](../../../templates/template-permissions.md) (Premium) to limit each template to the groups that need it.
To limit how many workspaces each developer runs, as Dev Box limits do, configure [quotas](../../../admin/users/quotas.md) (Premium).
Quotas give each developer a credit budget from the allowances of their groups.
Coder calculates each developer's budget separately in each organization.
A workspace spends credits only for resources that have a `daily_cost`, and the starter template sets none, so quotas don't limit its workspaces until you add one.

To allow each developer a set number of workspaces, give the data disk a cost of 1 credit, and give each developer's groups allowances that add up to the limit:

```tf
resource "coder_metadata" "data_disk" {
  resource_id = azurerm_managed_disk.data.id
  daily_cost  = 1
}
```

The data disk persists while the workspace is stopped, so stopped workspaces also count toward the limit.

## Step 5: Translate customizations

Dev Box team customizations run WinGet, PowerShell, and Git clone tasks from an image definition file.
Coder has no equivalent file, so move each task to the image or to the template:

- **Tasks that rarely change**: bake them into a custom image in your Azure Compute Gallery.
  This keeps workspace startup fast.
- **Tasks that run on every start**: add a [`coder_script`](../../../templates/extending-templates/index.md#run-scripts-on-workspace-lifecycle-events) to the template.
  On Windows, the Coder agent runs scripts with PowerShell.
- **Developer-specific tasks**: ask developers to move their user customizations to a [dotfiles](../../../workspaces/workspace-dotfiles.md) repository.
  The [Dotfiles module](https://registry.coder.com/modules/coder/dotfiles) runs a Bash script, so it doesn't run on Windows workspaces.
  On Windows, `coder dotfiles` runs the first of `install.ps1`, `bootstrap.ps1`, or `setup.ps1` that it finds in the repository root or its `script` folder, with PowerShell.
  It clones the repository with Git, so install Git in the image.

Refer to [Speed up your workspaces](../../../workspaces/speed-up-templates.md) for more ways to reduce startup time.

## Step 6: Configure Git authentication and secrets

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

## Step 7: Connect over Remote Desktop

Dev Box connects developers through Windows App or a browser.
Coder connects to Windows workspaces over RDP through the Coder agent, so workspaces need no public IP address.

The starter template includes the [Windows RDP](https://registry.coder.com/modules/coder/windows-rdp) module, which adds these buttons to the workspace page:

- **Web RDP**: opens an RDP session in the browser.
  It needs the wildcard access URL from [Step 2](#step-2-deploy-the-coder-control-plane).
- **RDP Desktop**: opens the session in a desktop RDP client through [Coder Desktop](../../../workspaces/desktop/index.md).

Developers without Coder Desktop can forward the RDP port with `coder port-forward` and connect with any RDP client.
Refer to [Remote desktops](../../../workspaces/workspace-access/remote-desktops.md) for each method.

The starter template creates a local administrator account with a generated password, and shows both on the workspace page while the workspace runs.
It doesn't join the VM to Entra ID, so developers don't sign in to Windows with their Entra ID account.
If your Dev Box pools gave developers standard user accounts, change the template to match.

## Step 8: Set up editors

Developers who used Dev Box dev tunnels with VS Code can connect to Coder workspaces over SSH instead:

- [VS Code](../../../workspaces/workspace-access/vscode.md), through the Coder extension.
- [JetBrains IDEs](../../../workspaces/workspace-access/jetbrains/toolbox.md), through JetBrains Toolbox.
- [Other editors](../../../workspaces/workspace-access/index.md), after developers run `coder config-ssh`.

Developers who run Visual Studio or other desktop tools inside Windows keep using them over RDP.

## Step 9: Replace schedules and hibernation

Recreate each cost control in your inventory with the matching Coder setting:

- **Autostop schedule**: Dev Box stops every dev box in a pool at one time of day.
  To stop workspaces at a set time in Coder, configure a daily [autostop requirement](../../../templates/managing-templates/schedule.md#autostop-requirement) (Premium) on the template.
  Workspaces then stop at the start of each developer's [quiet hours](../../../templates/managing-templates/schedule.md#user-quiet-hours) (Premium), even if they're in use.
  Set the default quiet hours with `CODER_QUIET_HOURS_DEFAULT_SCHEDULE`, and set `CODER_ALLOW_CUSTOM_QUIET_HOURS=false` so that developers can't change them.
  Unlike a Dev Box schedule, the default quiet hours apply to the whole deployment, not to one template.
  To stop idle workspaces instead, set a [default autostop](../../../templates/managing-templates/schedule.md#schedule) on the template, and let developers set [autostart and autostop](../../../workspaces/workspace-scheduling.md).
- **Stop on disconnect**: Coder extends the autostop deadline by the template's activity bump duration when it detects activity.
  It stops the workspace after the deadline passes with no activity.
  Traffic through the Coder agent counts as activity, including RDP sessions that connect through Coder.
  Refer to [activity detection](../../../workspaces/workspace-scheduling.md#activity-detection).
- **Hibernation**: Coder has no equivalent.
  Open applications don't survive a workspace stop, and the template decides which disks persist.
- **Auto-delete**: configure the [dormancy threshold](../../../templates/managing-templates/schedule.md#dormancy-threshold) and [dormancy auto-deletion](../../../templates/managing-templates/schedule.md#dormancy-auto-deletion) (Premium).
- **Required restarts**: to apply image and template updates on a schedule, configure an [autostop requirement](../../../templates/managing-templates/schedule.md#autostop-requirement) (Premium).

## Step 10: Plan for Intune and device policies

Dev Box enrolls each dev box in Microsoft Intune and joins it to Entra ID or Active Directory through the network connection.
Coder doesn't enroll workspaces in Intune or join them to a directory.

If your security policies require managed devices, configure the device join and Intune enrollment in the image or the template.
Intune's list of [supported operating systems](https://learn.microsoft.com/intune/fundamentals/ref-supported-platforms) includes Windows client editions, such as Windows 11 Enterprise, but not Windows Server, which the starter template uses.
To enroll workspaces, first switch the template to a client edition image, as described in [Choose the image](#choose-the-image).
For Entra ID join and Entra ID sign-in on Azure virtual machines, follow Microsoft's guide to [sign in to a Windows VM with Microsoft Entra ID](https://learn.microsoft.com/entra/identity/devices/howto-vm-sign-in-azure-ad-windows).
If you use conditional access policies that require a compliant device, test them from a pilot workspace before you move developers.

## Step 11: Pilot and cut over

1. Run Coder and Dev Box side by side.
1. Move one team and one pool to Coder first.
1. Compare workspace startup time and developer feedback with the same pool on Dev Box.
1. Ask developers to commit their work to Git.
1. Ask developers to copy the local files they need from each dev box.
1. Move the remaining pools one at a time.
1. After all developers move, delete your Dev Box resources by following the [Dev Box retirement guide](https://learn.microsoft.com/azure/dev-box/dev-box-retirement-guide).

## Learn more

- [Creating templates](../../../templates/creating-templates.md)
- [Remote desktops](../../../workspaces/workspace-access/remote-desktops.md)
- [OIDC with Microsoft Entra ID](../../../admin/users/oidc-auth/microsoft.md)
- [Workspace scheduling](../../../templates/managing-templates/schedule.md)
