---
name: Develop in Docker with a dotfiles URL
description: Develop inside Docker containers using your local daemon
tags: [local, docker]
icon: /icon/docker.png
---

# docker-with-dotfiles

This is an example for deploying workspaces with a prompt for the users' dotfiles repo URI.

## Getting started

Run `coder templates init` and select this template. Follow the instructions that appear.

## How it works

During workspace creation, Coder prompts you to specify a dotfiles URL via a Terraform variable. Once the workspace starts, the Coder agent runs `coder dotfiles` via the startup script:

```hcl
variable "dotfiles_uri" {
  description = <<-EOF
  Dotfiles repo URI (optional)

  see https://dotfiles.github.io
  EOF
    # The codercom/enterprise-* images are only built for amd64
  default = ""
}

resource "coder_agent" "main" {
  ...
  startup_script = var.dotfiles_uri != "" ? "/tmp/tmp.coder*/coder dotfiles -y ${var.dotfiles_uri}" : null
}
```

# Managing images and workspaces

Refer to the documentation in the [Docker template](../docker/README.md).
