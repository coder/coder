# Templates

Templates define the infrastructure underlying workspaces. Each Coder deployment
can have multiple templates for different workloads, such as ones for front-end
development, Windows development, and so on.

Coder manage templates, including sharing them and rolling out updates
to everybody. Users can also manually update their workspaces.

## Manage templates

Coder provides production-ready [sample templates](../examples/templates/), but you can
modify the templates with Terraform.

```sh
# start from an example
coder templates init

# optional: modify the template
vim <template-name>/main.tf

# add the template to Coder deployment
coder templates <create/update> <template-name>
```

> We recommend source controlling your templates.

## Persistent and ephemeral resources

Coder supports both ephemeral and persistent resources in workspaces. Ephemeral
resources are destroyed when a workspace is not in use (e.g., when it is
stopped). Persistent resources remain. See how this works for a sample front-end
template:

| Resource                     | Type       |
| :--------------------------- | :--------- |
| google_compute_disk.home_dir | persistent |
| kubernetes_pod.dev           | ephemeral  |
| └─ nodejs (linux, amd64)     |            |
| api_token.backend            | ephemeral  |

When a workspace is deleted, all resources are destroyed.

## Parameters

Templates often contain *parameters*. In Coder, there are two types of parameters:

- **Admin parameters** are set when a template is created/updated. These values
  are often cloud secrets, such as a `ServiceAccount` token, and are annotated
  with `sensitive =  true` in the template code.

**User parameters** are set when a user creates a workspace. They are unique to
each workspace, often personalization settings such as "preferred
region" or "workspace image".

---

Next: [Workspaces](./workspaces.md)
