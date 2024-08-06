# Web IDEs

By default, Coder workspaces allow connections via:

- Web terminal
- SSH (plus any [SSH-compatible IDE](../ides.md))

It's common to also connect via web IDEs for uses cases like zero trust
networks, data science, contractors, and infrequent code contributors.

![Row of IDEs](../images/ide-row.png)

In Coder, web IDEs are defined as
[coder_app](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/app)
resources in the template. With our generic model, any web application can be
used as a Coder application. For example:

<!-- TODO: Better link -->

> To learn more about configuring IDEs in templates, see our docs on
> [template administration](../../admin/templates/README.md).

![External URLs](../../images/external-apps.png)

## code-server

[code-server](https://github.com/coder/code-server) is our supported method of
running VS Code in the web browser. You can read more in our
[documentation for code-server](https://coder.com/docs/code-server/latest).

![code-server in a workspace](../images/code-server-ide.png)

## VS Code Web

We also support Microsoft's official product for using VS Code in the browser.
Contact your template administrator to configure it

<!-- TODO: Add screenshot -->

## JupyterLab

In addition to Jupyter Notebook, you can use Jupyter lab in your workspace

[This](https://github.com/sharkymark/v2-templates/tree/main/pod-with-jupyter-path)
is a community template example.

![JupyterLab in Coder](../images/jupyter.png)

## RStudio

Configure your agent and `coder_app` like so to use RStudio. Notice the
`subdomain=true` configuration:

```hcl
resource "coder_agent" "coder" {
  os             = "linux"
  arch           = "amd64"
  dir            = "/home/coder"
  startup_script = <<EOT
#!/bin/bash
# start rstudio
/usr/lib/rstudio-server/bin/rserver --server-daemonize=1 --auth-none=1 &
EOT
}

# rstudio
resource "coder_app" "rstudio" {
  agent_id      = coder_agent.coder.id
  slug          = "rstudio"
  display_name  = "R Studio"
  icon          = "https://upload.wikimedia.org/wikipedia/commons/d/d0/RStudio_logo_flat.svg"
  url           = "http://localhost:8787"
  subdomain     = true
  share         = "owner"

  healthcheck {
    url       = "http://localhost:8787/healthz"
    interval  = 3
    threshold = 10
  }
}
```

If you cannot enable a
[wildcard subdomain](https://coder.com/docs/admin/configure#wildcard-access-url),
you can configure the template to run RStudio on a path using an NGINX reverse
proxy in the template. There is however
[security risk](https://coder.com/docs/reference/cli/server#--dangerous-allow-path-app-sharing)
running an app on a path and the template code is more complicated with coder
value substitution to recreate the path structure.

[This](https://github.com/sempie/coder-templates/tree/main/rstudio) is a
community template example.

![RStudio in Coder](../images/rstudio-port-forward.png)

## Airflow

Configure your agent and `coder_app` like so to use Airflow. Notice the
`subdomain=true` configuration:

```hcl
resource "coder_agent" "coder" {
  os   = "linux"
  arch = "amd64"
  dir  = "/home/coder"
  startup_script = <<EOT
#!/bin/bash
# install and start airflow
pip3 install apache-airflow
/home/coder/.local/bin/airflow standalone &
EOT
}

resource "coder_app" "airflow" {
  agent_id      = coder_agent.coder.id
  slug          = "airflow"
  display_name  = "Airflow"
  icon          = "https://upload.wikimedia.org/wikipedia/commons/d/de/AirflowLogo.png"
  url           = "http://localhost:8080"
  subdomain     = true
  share         = "owner"

  healthcheck {
    url       = "http://localhost:8080/healthz"
    interval  = 10
    threshold = 60
  }
}
```

![Airflow in Coder](../images/airflow-port-forward.png)

## File Browser

Show and manipulate the contents of the `/home/coder` directory in a browser.

```hcl
resource "coder_agent" "coder" {
  os   = "linux"
  arch = "amd64"
  dir  = "/home/coder"
  startup_script = <<EOT
#!/bin/bash

curl -fsSL https://raw.githubusercontent.com/filebrowser/get/master/get.sh | bash
filebrowser --noauth --root /home/coder --port 13339 >/tmp/filebrowser.log 2>&1 &

EOT
}

resource "coder_app" "filebrowser" {
  agent_id     = coder_agent.coder.id
  display_name = "file browser"
  slug         = "filebrowser"
  url          = "http://localhost:13339"
  icon         = "https://raw.githubusercontent.com/matifali/logos/main/database.svg"
  subdomain    = true
  share        = "owner"

  healthcheck {
    url       = "http://localhost:13339/healthz"
    interval  = 3
    threshold = 10
  }
}
```

![File Browser](../images/file-browser.png)

## SSH Fallback

If you prefer to run web IDEs in localhost, you can port forward using
[SSH](../ides.md#ssh) or the Coder CLI `port-forward` sub-command. Some web IDEs
may not support URL base path adjustment so port forwarding is the only
approach.
