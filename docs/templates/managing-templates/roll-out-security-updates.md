---
title: Roll out a security update to every workspace
---

This guide is for template admins and Coder deployment administrators who need to patch a package inside every workspace, such as an updated `ca-certificates` package after a certificate authority is compromised.
It covers the image, the template version, new workspaces, and existing workspaces, in the order you apply them.
For general image practices, refer to [Image management](./image-management.md).

A workspace build uses the template version it was started with.
When the template references the patched image by a new tag or digest, the image reaches a workspace only when the workspace builds on a template version with that reference.
A running workspace keeps its current container or VM until it stops and starts again.

## Before you begin

You need:

- The Template Admin or Organization Template Admin role to push and promote template versions.
- The Owner or Organization Admin role to update workspaces that other users own.
  Template Admins can view other users' workspaces, but can't start, stop, or update them.
- A Premium license for [template update policies](./index.md#template-update-policies) and the [autostop requirement](./schedule.md#autostop-requirement).
  The rest of this guide works without Premium.

## Step 1: Rebuild the image with the patched package

1. Rebuild the workspace image so it installs the patched package version.
1. Push the image to your registry under a new tag.
1. If you reference the image by digest, copy the digest that the push prints.
1. In the template, reference the new tag or the digest:

   ```tf
   resource "docker_container" "workspace" {
     # Pin the digest so every build uses the patched image.
     image = "registry.example.com/workspace-base@sha256:<digest>"
     # ...
   }
   ```

A reference that changes makes the fix explicit and traceable to a template version.
If you push the patched image under the same mutable tag, such as `ubuntu`, a host or node that already has the old image can keep using it.
Docker reuses a local image with the same name and tag.
Kubernetes pulls a cached tag again only when the container's `imagePullPolicy` is `Always`, the default for `latest` and untagged images.
Refer to the [Kubernetes image pull policy](https://kubernetes.io/docs/concepts/containers/images/#image-pull-policy) documentation for details.

## Step 2: Publish and promote a new template version

Publish the template change from Step 1 as a new version and make it the active version.
By default, new workspaces start on the active version.

Keep the parameters of a security release the same as the previous version.
If the new version adds a required parameter without a default, updates fail until each owner enters a value, and autostart stops for those workspaces.

<div class="tabs">

### Dashboard

1. Open the template.
1. From the template's more-options menu (the vertical ellipsis), select **Edit files**.
1. Update the image reference.
1. Select **Build**.
1. When the build succeeds, select **Publish**.
1. In the **Publish new version** dialog, keep **Promote to active version** selected.
1. Select **Publish**.

To promote a version that you already published:

1. Open the template's **Versions** tab.
1. On the version, select **Promote…**.
1. Select **Promote**.

### CLI

From the template directory, push the new version:

```sh
coder templates push <template-name> --directory . --name <version-name>
```

`coder templates push` makes the new version active by default.
If you pushed it with `--activate=false`, promote it afterward:

```sh
coder templates versions promote --template=<template-name> --template-version=<version-name>
```

</div>

For pipelines that push template versions, refer to [Change management](./change-management.md).

## Step 3: Make future starts use the active version

Promotion doesn't change existing workspaces.
By default, a workspace that stops and starts again stays on its current template version.
Choose how workspaces move to the active version on their next start.
You can combine these options.

### Require the active version for every start

> [!NOTE]
> Template update policies are a Premium feature.

1. Open the template.
1. From the template's more-options menu (the vertical ellipsis), select **Settings**.
1. On the **General** page, under **Operations**, select **Require workspaces automatically update when started.**
1. Select **Save**.

Manual starts and autostarts then use the active version.
The policy doesn't apply to users who can edit the template, such as Owners and template admins, who can still start a workspace on an older version.

From the CLI, run:

```sh
coder templates edit <template-name> --require-active-version
```

Refer to [Template update policies](./index.md#template-update-policies).

### Let owners opt in to automatic updates

Without a Premium license, ask workspace owners to turn on automatic updates.
In the workspace's **Settings** > **General** page, the owner sets **Update Policy** to **Always**.
The workspace then updates to the active version whenever it starts.
Refer to [Automatic updates](../../workspaces/workspace-management.md#automatic-updates).

### Restart long-running workspaces on a schedule

> [!NOTE]
> The autostop requirement is a Premium feature.

A workspace that never stops never picks up a new version on start.
Set the template's [autostop requirement](./schedule.md#autostop-requirement) so workspaces stop on a regular schedule, even when a connection is open.
Combine it with the template update policy, so those workspaces start on the active version.

## Step 4: Update existing workspaces

Update the workspaces that already exist, so the fix doesn't wait for each owner's next restart.

> [!WARNING]
> Updating a running workspace stops it and starts it on the active version.
> Anyone connected to the workspace is disconnected, unsaved work is lost, and data outside persistent storage is deleted.
> Tell workspace owners before you update their workspaces.

<div class="tabs">

### Dashboard

1. Open the **Workspaces** page.
1. Replace the filter with `template:<template-name> outdated:true`.
1. Select the checkbox in the table header to select every workspace on the page.
1. Select **Bulk actions** > **Update…**.
1. If the dialog shows **Running workspaces detected**, select **I acknowledge these risks.**
1. Select **Update**.

   If a selected workspace is starting or stopping, **Update** is unavailable until that build finishes.
1. Repeat these steps until the filter shows no workspaces, or only dormant ones.

The dialog lists the workspaces it updates and skips the ones already on the active version.
Bulk update also starts selected workspaces that are stopped, and it skips dormant workspaces.

Use **Update…**, not **Stop** followed by **Start**.
Bulk **Start** starts each workspace on the version it already uses, so it doesn't apply the new image.

### CLI

`coder update` stops a running workspace and starts it on the active version.
It also starts a stopped workspace.
To update every outdated workspace of one template, one at a time, run:

```sh
coder list --search "template:<template-name> outdated:true" --output json \
  | jq -r '.[] | select(.dormant_at == null) | "\(.owner_name)/\(.name)"' \
  | xargs -n 1 coder update -y
```

The `select` filter skips dormant workspaces, like the dashboard.
Without it, `coder update` activates and starts each dormant workspace.

`coder list` ignores `--search` when you also pass `--all`, so the command uses `--search` alone.
Refer to [Update workspaces](../../workspaces/workspace-management.md#update-workspaces) for the other `coder update` options.

</div>

Dormant workspaces stay on their current version.
When an owner activates one, update it, or turn on the template update policy so it starts on the active version.

## Step 5: Confirm every workspace is on the active version

<div class="tabs">

### Dashboard

On the **Workspaces** page, filter by `template:<template-name> outdated:true`.
An empty list, or a list of only dormant workspaces, means every active workspace of the template is on the active version.
Each outdated workspace also shows an outdated indicator next to its name.

### CLI

List the workspaces that are still outdated:

```sh
coder list --search "template:<template-name> outdated:true" --column workspace,status,"current version",outdated
```

When every workspace is updated, the command prints `No workspaces found! Create one:` instead of a table.
Otherwise, it prints one row for each outdated workspace:

```text
WORKSPACE                       STATUS   CURRENT VERSION   OUTDATED
  <owner>/<workspace-name>      Started  <version-name>    true
```

</div>

To check the package itself, connect to an updated workspace that you own and query the package version, for example on a Debian-based image:

```sh
coder ssh <workspace-name> -- dpkg-query -W ca-certificates
```

The output shows the package name, then the installed version:

```text
ca-certificates    <version>
```

## Where a rebuilt image doesn't change the files

An updated template version replaces what the image provides.
It doesn't replace files that live outside the image:

- **Dev containers:** a dev container builds from the `devcontainer.json` in the project repository, not from the template's image.
  Change the image reference in the repository's dev container configuration to the new tag or digest, then have developers select **Rebuild** on the dev container.
  Refer to [Rebuild dev containers](../../workspaces/devcontainers/working-with-dev-containers.md#rebuild-dev-containers).
- **`envbuilder` caches:** a template that uses a cache repository can start from a previously built image instead of building again.
  Change the base image reference in the dev container configuration, for example to a new digest, so the build doesn't reuse the cached image.
  Refer to [Layer and image caching](../../admin/integrations/devcontainers/envbuilder/envbuilder-security-caching.md#layer-and-image-caching) and the [`envbuilder` caching documentation](https://github.com/coder/envbuilder/blob/main/docs/caching.md).
- **Persistent home volumes:** files in a persistent home directory survive updates.
  A copy of a certificate bundle or a tool installed under the home directory, such as in a language virtual environment, keeps the old version until the owner updates it.
- **Persistent disks and VMs:** templates that keep a VM or its boot disk across stops don't get a new image on start.
  Patch those workspaces with the operating system's package manager, for example in the startup script, or change the template so the boot disk is recreated.
  Refer to [Resource persistence](../extending-templates/resource-persistence.md).

Prebuilt workspaces follow the active version.
When you promote a new version, Coder deletes prebuilt workspaces for older versions and creates new ones on the active version.
If you change an image without a new template version, [invalidate the prebuilds](../extending-templates/prebuilt-workspaces.md#invalidate-prebuilds) instead.

## Learn more

- [Image management](./image-management.md)
- [Change management](./change-management.md)
- [Bulk operations](../../workspaces/workspace-management.md#bulk-operations)
- [Workspace scheduling](./schedule.md)
- [Template updates and the prebuilt workspace lifecycle](../extending-templates/prebuilt-workspaces.md#template-updates-and-the-prebuilt-workspace-lifecycle)
