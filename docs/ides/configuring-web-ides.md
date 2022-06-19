# Configuring web IDEs

By default, Coder workspaces allow connections via:

- Web terminal
- SSH (plus any [SSH-compatible IDE](../ides.md))

It's common to also let developers to connect via web IDEs.

![Row of IDEs](../images/ide-row.png)

In Coder, web IDEs are defined as
[coder_app](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/app)
resources in the template. This gives you full control over the version,
behavior, and configuration for applications in your workspace.

## code-server

![code-server in a workspace](../images/code-server-ide.png)

[code-server](https://github.com/coder/coder) is our supported method of running VS Code in the web browser. A simple way to install code-server in Linux/MacOS workspaces is via the Coder agent in your template:

```sh
# edit your template
cd your-template/ 
vim main.tf
```

```hcl
resource "coder_agent" "dev" {
    arch          = "amd64"
    os            = "linux"
    startup_script = <<EOF
    #!/bin/sh
    # install and start code-server
    curl -fsSL https://code-server.dev/install.sh | sh
    code-server --auth none --port 13337
    EOF
}
```

For advanced use, we recommend installing code-server in your VM snapshot or container image. Here's a Dockerfile which leverages some special [code-server features](https://coder.com/docs/code-server/):

```Dockerfile
FROM codercom/enterprise-base:ubuntu

# install a specific code-server version
RUN curl -fsSL https://code-server.dev/install.sh | sh -s -- --version=4.3.0

# pre-install versions
RUN code-server --install-extension eamodio.gitlens 
```

You'll also need to specify a `coder_app` resource related to the agent. This is how code-server is displayed on the workspace page.

```hcl
resource "coder_app" "code-server" {
  agent_id = coder_agent.dev.id
  name     = "VS Code"
  url      = "http://localhost:13337/?folder=/home/coder"
  icon     = "/code.svg"
}
```

## VNC Desktop

![VNC Desktop in Coder](../images/vnc-desktop.png)

You may want a full desktop environment to develop with/preview specialized software. 

Workspace requirements:

- VNC server (e.g. [tigervnc](https://tigervnc.org/))
- VNC client (e.g. [novnc](https://novnc.com/info.html))

Installation instructions will vary depending on your workspace's operating system, platform, and build system.

> Coder-provided VNC clients are on the roadmap ([#2106](https://github.com/coder/coder/issues/2106)).

As a starting point, see the [desktop-container](https://github.com/bpmct/coder-templates/tree/main/desktop-container) community template. It builds & provisions a Dockerized workspace with the following software:

- Ubuntu 20.04
- TigerVNC server
- noVNC client
- XFCE Desktop 

## JetBrains Projector

[JetBrains Projector](https://jetbrains.github.io/projector-client/mkdocs/latest/) is a JetBrains Incubator project which renders JetBrains IDEs in the web browser.

![JetBrains Projector in Coder](../images/jetbrains-projector.png)

> It is common to see latency and performance issues with Projector. We recommend using [Jetbrains Gateway](https://youtrack.jetbrains.com/issues/GTW) whenever possible (also no Template edits required!)

Workspace requirements:

- JetBrains server
- IDE (e.g IntelliJ IDEA, pyCharm)

Installation instructions will vary depending on your workspace's operating system, platform, and build system.

As a starting point, see the [jetbrains-container](https://github.com/bpmct/coder-templates/tree/main/jetbrains-container) community template. It builds & provisions a Dockerized workspaces for the following IDEs:

- CLion
- pyCharm
- DataGrip
- IntelliJ IDEA Community
- IntelliJ IDEA Ultimate
- PhpStorm
- pyCharm Community
- PyCharm Professional
- Rider
- Rubymine
- WebStorm
- ➕ code-server (just in case!)

## Custom IDEs and applications

As long as the process is running on the specified port inside your resource, you support any application.

```sh
# edit your template
cd your-template/ 
vim main.tf
```

```hcl
resource "coder_app" "portainer" {
  agent_id      = coder_agent.dev.id
  name          = "portainer"
  icon          = "https://simpleicons.org/icons/portainer.svg"
  url           = "http://localhost:8000"
  relative_path = true
}
```

> The full `coder_app` schema is described in the 
> [Terraform provider](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/app).

```sh
# update your template
coder templates update your-template
```
