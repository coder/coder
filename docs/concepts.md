# Coder Concepts

## Templates

Coder admins manage *templates* to define the infrastructure behind workspaces. A Coder deployment can have templates for different workloads.

### ex. "Frontend" Template

| Resource                | Type       |
| :---------------------- | :--------- |
| Kubernetes pod (NodeJS) | ephemeral  |
| API token (Backend)     | ephemeral  |
| Disk (Source code)      | persistant |

### ex. "Data Science" Template

| Resource                               | Type       |
| :------------------------------------- | :--------- |
| Kubernetes pod (pyCharm + JupyterLab)  | ephemeral  |
| Readonly volume mount (shared dataset) | persistant |

### ex. "MacOS" Template

| Resource           | Type       |
| :----------------- | :--------- |
| MacOS VM           | ephemeral  |
| Disk (source code) | persistant |

### ex. "Linux Debugging" Template

| Resource                 | Type       |
| :----------------------- | :--------- |
| EC2 VM (Debian 11.3 AMI) | persistant |

### Ephemeral vs persistant

Coder supports ephemeral and persistant resources in workspaces. Ephemeral resources will be destroyed when a workspace is stopped/not in use. Persistant resources are not.

When a workspace is deleted, all resources are destroyed.

### Managing templates

Admins can use Coder's production-ready examples, or create/modify templates with standard Terraform.

```sh
# start from an example
coder templates init

# optional: modify the template
vim <template-name>/main.tf

# add the template to Coder deployment
coder templates <create/update> <template-name>
```

If you are commonly editing templates, we recommend source-controlling template code using GitOps/CI pipelines to make changes.

### Variables

Templates often contain *variables*. In Coder, there are two types of variables.

**Admin variables** are set when a template is being created/updated. These are often cloud secrets, such as a ServiceAccount token. These are annotated with `sensitive =  true` in the template code.

**User variables** are set when a user creates a workspace. They are unique to each workspace, often personalization settings such as preferred region or workspace image.

## Workspaces

Coder users create *workspaces* to get a remote development environment. Depending on the template, 

### Supported IDEs

