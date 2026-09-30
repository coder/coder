# Operate and maintain

Day-two operations cover the work that keeps a deployment healthy after it's installed: upgrades, provisioning capacity, and eventual removal.

## What this phase covers

- [External provisioners](./provisioners/index.md) to isolate workspace builds from the control plane and to add build capacity.
- [Upgrading](./upgrade.md), including [upgrade best practices](./upgrade/best-practices.md) and the Extended Support Release upgrade paths.
- [Uninstall](./uninstall.md), for decommissioning a deployment and removing the resources it created.

## What lives elsewhere

Ongoing administration isn't part of this section.
Authentication, user and group management, templates, networking, monitoring, and security are covered in [Administration](../../admin/index.md).
Revisit [Plan your deployment](../plan/index.md) when your user count outgrows the size you originally chose, and [Validate your deployment](../validate/index.md) after any change large enough to alter the load profile.

<children></children>
