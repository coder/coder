# Coder Concepts

## Templates

Coder admins manage *templates* to define the infrastructure behind workspaces. A Coder deployment can have multiple templates for different workloads.

### ex. "Frontend" Template

| Resource name           | Status     |
| ----------------------- | ---------- |
| Kubernetes pod (NodeJS) | ephemeral  |
| API token (Backend)     | ephemeral  |
| Disk (Source code)      | persistant |

### ex. "Data Science" Template

| Resource name                          | Status     |
| -------------------------------------- | ---------- |
| Kubernetes pod (pyCharm + JupyterLab)  | ephemeral  |
| Readonly volume mount (shared dataset) | persistant |

### ex. "MacOS" Template

| Resource name      | Status     |
| ------------------ | ---------- |
| MacOS VM           | ephemeral  |
| Disk (source code) | persistant |

### ex. "Linux Debugging" Template

| Resource name            | Status     |
| ------------------------ | ---------- |
| EC2 VM (Debian 11.3 AMI) | persistant |

### Templates are managed via the CLI

Admins can use Coder's production-ready examples, or create/modify templates with standard Terraform.

```sh
# start from an example template
coder templates init

# optional: modify the template 
vim <template-name>/main.tf

# add the template to Coder
coder templates <create/update> <template-name>
```

## Workspaces

Coder users create *workspaces* to get a remote development environment. Depending on the template, 




