---
name: Develop in a Proxmox VM
description: Get started with a development environment on Proxmox.
tags: [self-hosted, proxmox, vm, code-server]
---

# proxmox-vm

## Getting started

Pick this template in `coder templates init` and follow instructions.

## Authentication

Coder will authenticate with the [PM_API_ environment variables](https://registry.terraform.io/providers/Telmate/proxmox/latest/docs#creating-the-connection-via-username-and-api-token). Ensure you have these set before you run `coder server` or in
`/etc/coder.d/coder.env` if you are running Coder as a system service.

## How it works

This example assumes you have a `ubuntu-2004-cloudinit-template` VM template in your Proxmox cluster. You can create the exact template
by following [this guide](https://austinsnerdythings.com/2021/08/30/how-to-create-a-proxmox-ubuntu-cloud-init-image/)

You can also edit the template to specify any cloud-init compatible VM on your cluster, such as one built with Packer and additional
dependencies (Java, IntelliJ IDEA, VNC, noVNC). 

## Add additional web app

If you have additional web applications installed on your VM image (e.g noVNC, JupyterHub), you can add a `coder_app` template
to the template.

```hcl
resource "coder_app" "novnc" {
  agent_id      = coder_agent.dev.id
  name          = "VNC Desktop"
  icon          = "https://novnc.com/noVNC/app/images/icons/novnc-32x32.png"
  url           = "http://localhost:13338"
  relative_path = true
}
```

If applications need to be started, they can be added to the `startup_script` of the coder agent:

```diff
resource "coder_agent" "dev" {
  arch           = "amd64"
  auth           = "token"
  dir            = "/home/${lower(data.coder_workspace.me.owner)}"
  os             = "linux"
  startup_script = <<EOT
#!/bin/sh
# install and start code-server
curl -fsSL https://code-server.dev/install.sh | sh
code-server --auth none --port 13337 &
EOT

+# install and start novnc
+ sudo snap install novnc
+ novnc --listen 13338
}
```

You can, of course, install/start all your applications with a system manager or even build
into the VM template with Packer.

> You'll also need a VNC server running on your workspace/image for this example to work.
> I couldn't find a great one-line command to demonstrate this, but any VNC server will do.
