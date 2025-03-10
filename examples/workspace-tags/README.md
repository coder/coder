---
name: Sample Template with Workspace Tags
description: Review the sample template and introduce dynamic workspace tags to your template
tags: [local, docker, workspace-tags]
icon: /icon/docker.png
---

## Overview

This Coder template presents use of [Workspace Tags](https://coder.com/docs/admin/templates/extending-templates/workspace-tags) and [Coder Parameters](https://coder.com/docs/templates/parameters).

## Use case

Template administrators can use static tags to control workspace provisioning, limiting it to specific provisioner groups. However, this restricts workspace users from choosing their preferred workspace nodes.

By using `coder_workspace_tags` and `coder_parameter`s, template administrators can allow dynamic tag selection, avoiding the need to push the same template multiple times with different tags.

## Notes

- You will need to have an [external provisioner](https://coder.com/docs/admin/provisioners#external-provisioners) with the correct tagset running in order to import this template.
- When specifying values for the `coder_workspace_tags` data source, you are restricted to using a subset of Terraform's capabilities. See [here](https://coder.com/docs/admin/templates/extending-templates/workspace-tags) for more details.


## Development

Update the template and push it using the following command:

```shell
./scripts/coder-dev.sh templates push examples-workspace-tags \
  -d examples/workspace-tags \
  -y
```
