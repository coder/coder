---
name: Sample Template with Workspace Tags
description: Review the sample template and introduce dynamic workspace tags to your template
tags: [local, docker, workspace-tags]
icon: /icon/docker.png
---

# Overview

This Coder template presents use of [Workspace Tags](https://coder.com/docs/v2/latest/templates/workspace-tags) [Coder Parameters](https://coder.com/docs/v2/latest/templates/parameters).

# Use case

Template administrators can use static tags to control workspace provisioning, limiting it to specific provisioner groups. However, this restricts workspace users from choosing their preferred workspace nodes.

By using `coder_workspace_tags` and `coder_parameter`s, template administrators can allow dynamic tag selection, avoiding the need to push the same template multiple times with different tags.

## Development

Update the template and push it using the following command:

```
./scripts/coder-dev.sh templates push examples-workspace-tags \
  -d examples/workspace-tags \
  -y
```
