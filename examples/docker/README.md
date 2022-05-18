---
name: Develop in Docker
description: Run workspaces on a Docker host using registry images 
tags: [local, docker]
---

# docker

## Getting started

Pick this template in `coder templates init` and follow instructions. 

## Adding/removing images

After building and pushing an image to an image registry (e.g DockerHub), you can make the
image available to users in the template. 

Edit the template:

```sh
vim main.tf
```
variable "docker_image" {
  description = "What docker image would you like to use for your workspace?"
  default     = "codercom/enterprise-base:ubuntu"
  validation {
-    condition     = contains(["codercom/enterprise-base:ubuntu", "codercom/enterprise-node:ubuntu", "codercom/enterprise-intellij:ubuntu"], var.docker_image)
+    condition     = contains(["codercom/enterprise-base:ubuntu", "codercom/enterprise-node:ubuntu", "codercom/enterprise-intellij:ubuntu", "codercom/enterprise-golang:ubuntu"], var.docker_image)

    error_message = "Invalid Docker Image!"
  }
}
```

Update the template:

```sh
coder template update docker
```

Images can also be removed from the validation list. Workspaces using older template versions will continue using
the removed image until the workspace is updated to the latest version.

## Updating images

To reduce drift, we recommend versioning images in your registry via tags. Update the image tag in the template:

```sh
variable "docker_image" {
  description = "What docker image would you like to use for your workspace?"
  default     = "codercom/enterprise-base:ubuntu"
  validation {
-    condition     = contains(["my-org/base-development:v1.1", "myorg-java-development:v1.1"], var.docker_image)
+    condition     = contains(["my-org/base-development:v1.1", "myorg-java-development:v1.2"], var.docker_image)

    error_message = "Invalid Docker image!"
  }
}
```

Optional: Update workspaces to the latest template version:

```sh
coder ls
coder update [workspace name]
```

## Extending this template

See the [kreuzwerker/docker](https://registry.terraform.io/providers/kreuzwerker/docker) Terraform provider documentation to
add the following features to your Coder template:

- SSH/TCP docker host
- Registry authentication
- Build args
- Volume mounts
- Custom container spec
- More

Contributions are also welcome!

