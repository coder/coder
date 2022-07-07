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

# directly start code-server with the agent's startup_script (see above),
# or use a proccess manager like supervisord
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

> If the code-server integrated terminal fails to load, (i.e., xterm fails to load), go to DevTools to ensure xterm is loaded, refresh the browser, or clear your browser cache.

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

As a starting point, see the [projector-container](https://github.com/bpmct/coder-templates/tree/main/projector-container) community template. It builds & provisions a Dockerized workspaces for the following IDEs:

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

## SSH port forwarding of browser IDEs

Coder OSS currently does not have dev URL functionality required to open certain web IDEs,e.g.,  JupyterLab, RStudio or Airflow, so developers can either use `coder port-forward <workspace name> --tcp <port>:<port>` or `ssh -L <port>:localhost:<port> coder.<workspace name>` to open the IDEs from `http://localhost:<port>`

> You must install JupyterLab, RStudio, or Airflow in the image first. Note the JupyterLab and Airflow examples that install the IDEs in the `coder_script`

### JupyterLab

```hcl
resource "coder_agent" "coder" {
  os   = "linux"
  arch = "amd64"
  dir = "/home/coder"
  startup_script = <<EOT
#!/bin/bash
# install and start jupyterlab
pip3 install jupyterlab
jupyter lab --ServerApp.token='' --ServerApp.ip='*' &
EOT
}
```

From your local machine, enter a terminal and start the ssh port forwarding and open localhost with the specified port.
```console
ssh -L 8888:localhost:8888 coder.<JupyterLab workspace name>
```

### RStudio

```hcl
resource "coder_agent" "coder" {
  os   = "linux"
  arch = "amd64"
  dir = "/home/coder"
  startup_script = <<EOT
#!/bin/bash
# start rstudio
/usr/lib/rstudio-server/bin/rserver --server-daemonize=1 --auth-none=1 &
EOT
}
```

From your local machine, enter a terminal and start the ssh port forwarding and open localhost with the specified port.
```console
ssh -L 8787:localhost:8787 coder.<RStudio workspace name>
```

As a starting point, see a [RStudio Dockerfile](https://github.com/mark-theshark/dockerfiles/blob/main/rstudio/no-args/Dockerfile) for creating an RStudio image.

### Airflow

```hcl
resource "coder_agent" "coder" {
  os   = "linux"
  arch = "amd64"
  dir = "/home/coder"
  startup_script = <<EOT
#!/bin/bash
# install and start airflow
pip3 install apache-airflow 2>&1 | tee airflow-install.log
/home/coder/.local/bin/airflow standalone  2>&1 | tee airflow-run.log &
EOT
}
```

From your local machine, enter a terminal and start the ssh port forwarding and open localhost with the specified port.
```console
ssh -L 8080:localhost:8080 coder.<Airflow workspace name>
```



> The full `coder_app` schema is described in the 
> [Terraform provider](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/app).

```sh
# update your template
coder templates update your-template
```
