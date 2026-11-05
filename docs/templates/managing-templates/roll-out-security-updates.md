---
title: Roll out a security update to every workspace
---

This guide is for template admins and Coder deployment administrators who need to patch a package inside every workspace, such as an updated `ca-certificates` package after a certificate authority is compromised.
It covers the image, the template version, new workspaces, and existing workspaces, in the order you apply them.
For general image practices, refer to [Image management](./image-management.md).

A workspace build uses the template version it was started with.
A rebuilt image reaches a workspace only when a template version that uses the image is active and the workspace builds on that version.
A running workspace keeps its current container or VM until it stops and starts again.

## Before you begin

You need:

- The Template Admin role (or above) to push and promote template versions.
- The Owner or Organization Admin role to update workspaces that other users own.
  Template Admins can view other users' workspaces, but can't start, stop, or update them.
- A Premium license for [Template update policies](./index.md#template-update-policies) and the [autostop requirement](./schedule.md#autostop-requirement).
  The rest of this guide works without Premium.

## Step 1: Rebuild the image with the patched package

1. Rebuild the workspace image so it installs the patched package version.
1. Push the image to your registry under a new tag, or record the digest that the push prints.
1. In the template, reference the new tag or the digest:

   ```tf
   resource "docker_container" "workspace" {
     # Pin the digest so every build uses the patched image.
     image = "registry.example.com/workspace-base@sha256:<digest>"
     # ...
   }
   ```

A reference that changes makes the fix explicit and traceable to a template version.
If you push the patched image under the same mutable tag, such as `latest` or `ubuntu`, a host or node that already has the old image can keep using it.
Docker reuses a local image with the same name, and Kubernetes only pulls a cached tag again when the pod's `imagePullPolicy` is `Always`.
Refer to the [Kubernetes image pull policy](https://kubernetes.io/docs/concepts/containers/images/#image-pull-policy) documentation for details.

## Step 2: Publish and promote a new template version

Publish the template change from Step 1 as a new version and make it the active version.
New workspaces always start on the active version.

### Dashboard

1. Open the template.
1. Open the template's more-options menu (the vertical ellipsis), then select **Edit files**.
1. Update the image reference.
1. Select **Build**.
1. When the build succeeds, select **Publish**.
1. In the **Publish new version** dialog, keep **Promote to active version** selected.
1. Select **Publish**.

To promote a version that you already published, open the template's **Versions** tab, select **Promote…** on the version, then confirm.

### CLI

From the template directory, push the new version:

```sh
coder templates push <template-name> --directory . --name <version-name>
```

`coder templates push` makes the new version active by default.
If you pushed it with `--activate=false`, promote it afterwards:

```sh
coder templates versions promote --template=<template-name> --template-version=<version-name>
```

For pipelines that push template versions, refer to [Change management](./change-management.md).

Keep the parameters of a security release the same as the previous version.
If the new version adds a required parameter without a default, updates fail until each owner enters a value, and autostart stops for those workspaces.

## Step 3: Make future starts use the active version

Promotion doesn't change existing workspaces.
By default, a workspace that stops and starts again stays on its current template version.
Choose how workspaces move to the active version on their next start.
You can combine these options.

### Require the active version for every start

> [!NOTE]
> Template update policies are a Premium feature.

1. Open the template.
1. Open the template's more-options menu (the vertical ellipsis), then select **Settings**.
1. On the **General** page, under **Operations**, select **Require workspaces automatically update when started.**
1. Select **Save**.

Manual starts and autostarts by template users then use the active version.
The policy doesn't apply to template admins, who can still start a workspace on an older version.

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

### Dashboard

1. Open the **Workspaces** page.
1. Replace the filter with `template:<template-name> outdated:true`.
1. Select the checkbox in the table header to select every workspace on the page.
1. Select **Bulk actions** > **Update…**.
1. If the dialog shows **Running workspaces detected**, select **I acknowledge these risks.**
1. Select **Update**.

The dialog lists the workspaces it updates and skips the ones already on the active version.
It also starts selected workspaces that are stopped, and it skips dormant workspaces.
Repeat the steps for each page of results.

Use **Update…**, not **Stop** followed by **Start**.
Bulk **Start** starts each workspace on the version it already uses, so it doesn't apply the new image.

### CLI

`coder update` stops a running workspace and starts it on the active version.
It also starts a stopped workspace.
To update every outdated workspace of one template, run:

```sh
coder list --search "template:<template-name> outdated:true" --output json \
  | jq -r '.[] | "\(.owner_name)/\(.name)"' \
  | xargs -n 1 coder update -y
```

`coder list` ignores `--search` when you also pass `--all`, so the command uses `--search` alone.
Refer to [Update workspaces](../../workspaces/workspace-management.md#update-workspaces) for the other `coder update` options.

## Step 5: Confirm every workspace is on the active version

1. On the **Workspaces** page, filter by `template:<template-name> outdated:true`.

   An empty list means every workspace of the template is on the active version.
   Each outdated workspace also shows an outdated indicator next to its name.
1. From the CLI, list the remaining workspaces:

   ```sh
   coder list --search "template:<template-name> outdated:true" --column workspace,status,"current version",outdated
   ```

1. Connect to an updated workspace and check the package version, for example on a Debian-based image:

   ```sh
   coder ssh <owner>/<workspace-name> -- dpkg-query -W ca-certificates
   ```

   The output shows the package name and the installed version.

## Where a rebuilt image doesn't change the files

An updated template version replaces what the image provides.
It doesn't replace files that live outside the image:

- **Dev containers:** a dev container builds from the `devcontainer.json` in the project repository, not from the template's image.
  Update the base image the repository's dev container configuration uses, then have developers select **Rebuild** on the dev container.
  Refer to [Rebuild dev containers](../../workspaces/devcontainers/working-with-dev-containers.md#rebuild-dev-containers).
- **Envbuilder caches:** a template that uses a cache repository can start from a previously built image instead of building again.
  Change the base image reference in the dev container configuration, for example to a new digest, so the build doesn't reuse the cached image.
  Refer to [Envbuilder layer and image caching](../../admin/integrations/devcontainers/envbuilder/envbuilder-security-caching.md#layer-and-image-caching).
- **Persistent home volumes:** files in a persistent home directory survive updates.
  A copy of a certificate bundle or a tool installed under the home directory, such as in a language virtual environment, keeps the old version until the owner updates it.
- **Persistent disks and VMs:** templates that keep a VM or its boot disk across stops don't get a new image on start.
  Patch those workspaces with the operating system's package manager, for example in the startup script, or change the template so the boot disk is recreated.
  Refer to [Resource persistence](../extending-templates/resource-persistence.md).
- **Prebuilt workspaces:** when you promote a new version, Coder deletes prebuilt workspaces for older versions and creates new ones on the active version.
  Refer to [Template updates and the prebuilt workspace lifecycle](../extending-templates/prebuilt-workspaces.md#template-updates-and-the-prebuilt-workspace-lifecycle).

## Learn more

- [Image management](./image-management.md)
- [Change management](./change-management.md)
- [Bulk operations](../../workspaces/workspace-management.md#bulk-operations)
- [Workspace scheduling](./schedule.md)
