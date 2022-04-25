# Templates

Coder admins manage *templates* to define the infrastructure behind workspaces. A Coder deployment can have multiple templates for different workloads, such as "frontend development," "windows development," etc.

## Managing templates

Coder provides production-ready template [examples](../examples/), but they can be modified with Terraform.

```sh
# start from an example
coder templates init

# optional: modify the template
vim <template-name>/main.tf

# add the template to Coder deployment
coder templates <create/update> <template-name>
```

If you are commonly editing templates, we recommend source-controlling template code using GitOps/CI pipelines to make changes.

## Persistant and ephemeral resources

Coder supports ephemeral and persistant resources in workspaces. Ephemeral resources are be destroyed when a workspace is not in use (stopped). Persistant resources remain. See how this works for an example "frontend" template:

| Resource                     | Type       |
| :--------------------------- | :--------- |
| google_compute_disk.home_dir | persistent |
| kubernetes_pod.dev           | ephemeral  |
| └─ nodejs (linux, amd64)     |            |
| api_token.backend            | ephemeral  |

When a workspace is deleted, all related resources are destroyed.

## Variables

Templates often contain *variables*. In Coder, there are two types of variables.

**Admin variables** are set when a template is being created/updated. These are often cloud secrets, such as a ServiceAccount token. These are annotated with `sensitive =  true` in the template code.

**User variables** are set when a user creates a workspace. They are unique to each workspace, often personalization settings such as preferred region or workspace image.

---

Next: [Workspaces](./workspaces.md)
