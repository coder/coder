# FAQs

Frequently asked questions on Coder OSS and Enterprise deployments. These FAQs
come from our community and enterprise customers, feel free to
[contribute to this page](https://github.com/coder/coder/edit/main/docs/faqs.md).

<details style="margin-bottom: 28px;">
  <summary style="font-size: larger; font-weight: bold;">How do I add an enterprise license?</summary>

Visit https://coder.com/trial or contact
[sales@coder.com](mailto:sales@coder.com?subject=License) to get a v2 enterprise
trial key.

You can add a license through the UI or CLI.

In the UI, click the Deployment tab -> Licenses and upload the `jwt` license
file.

> To add the license with the CLI, first
> [install the Coder CLI](./install/index.md#install-script) and server to the
> latest release.

If the license is a text string:

```sh
coder licenses add -l 1f5...765
```

If the license is in a file:

```sh
coder licenses add -f <path/filename>
```

</details>

<details style="margin-bottom: 28px;">
  <summary style="font-size: larger; font-weight: bold;">I'm experiencing networking issues, so want to disable Tailscale, STUN, Direct connections and force use of websockets</summary>

The primary developer use case is a local IDE connecting over SSH to a Coder
workspace.

Coder's networking stack has intelligence to attempt a peer-to-peer or
[Direct connection](https://coder.com/docs/v2/latest/networking#direct-connections)
between the local IDE and the workspace. However, this requires some additional
protocols like UDP and being able to reach a STUN server to echo the IP
addresses of the local IDE machine and workspace, for sharing using a Wireguard
Coordination Server. By default, Coder assumes Internet and attempts to reach
Google's STUN servers to perform this IP echo.

Operators experimenting with Coder may run into networking issues if UDP (which
STUN requires) or the STUN servers are unavailable, potentially resulting in
lengthy local IDE and SSH connection times as the Coder control plane attempts
to establish these direct connections.

Setting the following flags as shown disables this logic to simplify
troubleshooting.

| Flag                                                                                                           | Value       | Meaning                               |
| -------------------------------------------------------------------------------------------------------------- | ----------- | ------------------------------------- |
| [`CODER_BLOCK_DIRECT`](https://coder.com/docs/v2/latest/cli/server#--block-direct-connections)                 | `true`      | Blocks direct connections             |
| [`CODER_DERP_SERVER_STUN_ADDRESSES`](https://coder.com/docs/v2/latest/cli/server#--derp-server-stun-addresses) | `"disable"` | Disables STUN                         |
| [`CODER_DERP_FORCE_WEBSOCKETS`](https://coder.com/docs/v2/latest/cli/server#--derp-force-websockets)           | `true`      | Forces websockets over Tailscale DERP |

</details>

<details style="margin-bottom: 28px;">
  <summary style="font-size: larger; font-weight: bold;">How do I configure NGINX as the reverse proxy in front of Coder?</summary>

[This doc](https://github.com/coder/coder/tree/main/examples/web-server/nginx#configure-nginx)
in our repo explains in detail how to configure NGINX with Coder so that our
Tailscale Wireguard networking functions properly.

</details>

<details style="margin-bottom: 28px;">
  <summary style="font-size: larger; font-weight: bold;">How do I hide some of the default icons in a workspace like VS Code Desktop, Terminal, SSH, Ports?</summary>

The visibility of Coder apps is configurable in the template. To change the
default (shows all), add this block inside the
[`coder_agent`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/app)
of a template and configure as needed:

```hcl
  display_apps {
    vscode = false
    vscode_insiders = false
    ssh_helper = false
    port_forwarding_helper = false
    web_terminal = true
  }
```

This example will hide all built-in coder_app icons except the web terminal.

</details>

<details style="margin-bottom: 28px;">
  <summary style="font-size: larger; font-weight: bold;">I want to allow code-server to be accessible by other users in my deployment.</summary>

> It is **not** recommended to share a web IDE, but if required, the following
> deployment environment variable settings are required.

Set deployment (Kubernetes) to allow path app sharing

```yaml
# allow authenticated users to access path-based workspace apps
- name: CODER_DANGEROUS_ALLOW_PATH_APP_SHARING
  value: "true"
# allow Coder owner roles to access path-based workspace apps
- name: CODER_DANGEROUS_ALLOW_PATH_APP_SITE_OWNER_ACCESS
  value: "true"
```

In the template, set
[`coder_app`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/app)
[`share`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/app#share)
option to `authenticated` and when a workspace is built with this template, the
pretty globe shows up next to path-based `code-server`:

```hcl
resource "coder_app" "code-server" {
  ...
  share        = "authenticated"
  ...
}
```

</details>

<details style="margin-bottom: 28px;">
  <summary style="font-size: larger; font-weight: bold;">I installed Coder and created a workspace but the icons do not load.</summary>

An important concept to understand is that Coder creates workspaces which have
an agent that must be able to reach the `coder server`.

If the
[`CODER_ACCESS_URL`](https://coder.com/docs/v2/latest/admin/configure#access-url)
is not accessible from a workspace, the workspace may build, but the agent
cannot reach Coder, and thus the missing icons. e.g., Terminal, IDEs, Apps.

> By default, `coder server` automatically creates an Internet-accessible
> reverse proxy so that workspaces you create can reach the server.

If you are doing a standalone install, e.g., on a Macbook and want to build
workspaces in Docker Desktop, everything is self-contained and workspaces
(containers in Docker Desktop) can reach the Coder server.

```sh
coder server --access-url http://localhost:3000 --address 0.0.0.0:3000
```

> Even `coder server` which creates a reverse proxy, will let you use
> http://localhost to access Coder from a browser.

</details>

<details style="margin-bottom: 28px;">
  <summary style="font-size: larger; font-weight: bold;">I updated a template, and an existing workspace based on that template fails to start.</summary>

When updating a template, be aware of potential issues with input variables. For
example, if a template prompts users to choose options like a
[code-server](https://github.com/coder/code-server)
[VS Code](https://code.visualstudio.com/) IDE release, a
[container image](https://hub.docker.com/u/codercom), or a
[VS Code extension](https://marketplace.visualstudio.com/vscode), removing any
of these values can lead to existing workspaces failing to start. This issue
occurs because the Terraform state will not be in sync with the new template.

However, a lesser-known CLI sub-command,
[`coder update`](https://coder.com/docs/v2/latest/cli/update), can resolve this
issue. This command re-prompts users to re-enter the input variables,
potentially saving the workspace from a failed status.

```sh
coder update --always-prompt <workspace name>
```

</details>

<details style="margin-bottom: 28px;">
  <summary style="font-size: larger; font-weight: bold;">I'm running coder on a VM with systemd but latest release installed isn't showing up.</summary>

Take, for example, a Coder deployment on a VM with a 2 shared vCPU systemd
service. In this scenario, it's necessary to reload the daemon and then restart
the Coder service. This prevents the `systemd` daemon from trying to reference
the previous Coder release service since the unit file has changed.

The following commands can be used to update Coder and refresh the service:

```sh
curl -fsSL https://coder.com/install.sh | sh
sudo systemctl daemon-reload
sudo systemctl restart coder.service
```

</details>

<details style="margin-bottom: 28px;">
  <summary style="font-size: larger; font-weight: bold;">I'm using the built-in Postgres database and forgot admin email I set up.</summary>

1. Run the `coder server` command below to retrieve the `psql` connection URL
   which includes the database user and password.
2. `psql` into Postgres, and do a select query on the `users` table.
3. Restart the `coder server`, pull up the Coder UI and log in (you will still
   need your password)

```sh
coder server postgres-builtin-url
psql "postgres://coder@localhost:53737/coder?sslmode=disable&password=I2S...pTk"
```

</details>

<details style="margin-bottom: 28px;">
  <summary style="font-size: larger; font-weight: bold;">How to find out Coder's latest Terraform provider version?</summary>

[Coder is on the HashiCorp's Terraform registry](https://registry.terraform.io/providers/coder/coder/latest).
Check this frequently to make sure you are on the latest version.

Sometimes, the version may change and `resource` configurations will either
become deprecated or new ones will be added when you get warnings or errors
creating and pushing templates.

</details>

<details style="margin-bottom: 28px;">
  <summary style="font-size: larger; font-weight: bold;">How can I set up TLS for my deployment and not create a signed certificate?</summary>

Caddy is an easy-to-configure reverse proxy that also automatically creates
certificates from Let's Encrypt.
[Install docs here](https://caddyserver.com/docs/quick-starts/reverse-proxy) You
can start Caddy as a `systemd` service.

The Caddyfile configuration will appear like this where `127.0.0.1:3000` is your
`CODER_ACCESS_URL`:

```text
coder.example.com {

  reverse_proxy 127.0.0.1:3000

  tls {

    issuer acme {
      email user@example.com
    }

  }
}
```

</details>

<details style="margin-bottom: 28px;">
  <summary style="font-size: larger; font-weight: bold;">I'm using Caddy as my reverse proxy in front of Coder. How do I set up a wildcard domain for port forwarding?</summary>

Caddy requires your DNS provider's credentials to create wildcard certificates.
This involves building the Caddy binary
[from source](https://github.com/caddyserver/caddy) with the DNS provider plugin
added. e.g.,
[Google Cloud DNS provider here](https://github.com/caddy-dns/googleclouddns)

To compile Caddy, the host running Coder requires Go. Once installed, replace
the existing Caddy binary in `usr/bin` and restart the Caddy service.

The updated Caddyfile configuration will look like this:

```text
*.coder.example.com, coder.example.com {

  reverse_proxy 127.0.0.1:3000

  tls {
    issuer acme {
      email user@example.com
      dns googleclouddns {
        gcp_project my-gcp-project
      }
    }
  }

}
```

</details>

<details style="margin-bottom: 28px;">
  <summary style="font-size: larger; font-weight: bold;">Can I use local or remote Terraform Modules in Coder templates?</summary>

One way is to reference a Terraform module from a GitHub repo to avoid
duplication and then just extend it or pass template-specific
parameters/resources:

```hcl
# template1/main.tf
module "central-coder-module" {
  source = "github.com/yourorg/central-coder-module"
  myparam = "custom-for-template1"
}

resource "ebs_volume" "custom_template1_only_resource" {
}
```

```hcl
# template2/main.tf
module "central-coder-module" {
  source = "github.com/yourorg/central-coder-module"
  myparam = "custom-for-template2"
  myparam2 = "bar"
}

resource "aws_instance" "custom_template2_only_resource" {
}
```

Another way using local modules is to symlink the module directory inside the
template directory and then `tar` the template.

```sh
ln -s modules template_1/modules
tar -cvh -C ./template_1 | coder templates <push|create> -d - <name>
```

References:

- [Public Github Issue 6117](https://github.com/coder/coder/issues/6117)
- [Public Github Issue 5677](https://github.com/coder/coder/issues/5677)
- [Coder docs: Templates/Change Management](https://coder.com/docs/v2/latest/templates/change-management)
</details>

<details style="margin-bottom: 28px;">
  <summary style="font-size: larger; font-weight: bold;">Can I run Coder in an air-gapped or offline mode? (no Internet)?</summary>

Yes, Coder can be deployed in air-gapped or offline mode.
https://coder.com/docs/v2/latest/install/offline

Our product bundles with the Terraform binary so assume access to terraform.io
during installation. The docs outline rebuilding the Coder container with
Terraform built-in as well as any required Terraform providers.

Direct networking from local SSH to a Coder workspace needs a STUN server. Coder
defaults to Google's STUN servers, so you can either create your STUN server in
your network or disable and force all traffic through the control plane's DERP
proxy.

</details>

<details style="margin-bottom: 28px;">
  <summary style="font-size: larger; font-weight: bold;">Create a randomized computer_name for an Azure VM</summary>

Azure VMs have a 15 character limit for the `computer_name` which can lead to
duplicate name errors.

This code produces a hashed value that will be difficult to replicate.

```hcl
locals {
  concatenated_string = "${data.coder_workspace.me.name}+${data.coder_workspace.me.owner}"
  hashed_string = md5(local.concatenated_string)
  truncated_hash = substr(local.hashed_string, 0, 16)
}
```

</details>

<details style="margin-bottom: 28px;">
  <summary style="font-size: larger; font-weight: bold;">Do you have example JetBrains Gateway templates?</summary>

In August 2023, JetBrains certified the Coder plugin signifying enhanced
stability and reliability.

The Coder plugin will appear in the Gateway UI when opened.

Selecting the most suitable template depends on how the deployment manages
JetBrains IDE versions. If downloading from
[jetbrains.com](https://www.jetbrains.com/remote-development/gateway/) is
acceptable, see the example templates below which specifies the product code,
IDE version and build number in the
[`coder_app`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/app#share)
resource. This will present an icon in the workspace dashboard which when
clicked, will look for a locally installed Gateway, and open it. Alternatively,
the IDE can be baked into the container image and manually open Gateway (or
IntelliJ which has Gateway built-in), using a session token to Coder and then
open the IDE.

- [IntelliJ IDEA](https://github.com/sharkymark/v2-templates/tree/main/pod-idea)
- [IntelliJ IDEA with Icon](https://github.com/sharkymark/v2-templates/tree/main/pod-idea-icon)
</details>

<details style="margin-bottom: 28px;">
  <summary style="font-size: larger; font-weight: bold;">What options do I have for adding VS Code extensions into code-server, VS Code Desktop or Microsoft's Code Server?</summary>

Coder has an open-source project called
[`code-marketplace`](https://github.com/coder/code-marketplace) which is a
private VS Code extension marketplace. There is even integration with JFrog
Artifactory.

- [Blog post](https://coder.com/blog/running-a-private-vs-code-extension-marketplace)
- [OSS project](https://github.com/coder/code-marketplace)

[See this example template](https://github.com/sharkymark/v2-templates/blob/main/code-marketplace/main.tf#L229C1-L232C12)
where the agent specifies the URL and config environment variables which
code-server picks up and points the developer to.

Another option is to use Microsoft's code-server - which is like Coder's, but it
can connect to Microsoft's extension marketplace so Copilot and chat can be
retrieved there.
[See a sample template here](https://github.com/sharkymark/v2-templates/blob/main/vs-code-server/main.tf).

Another option is to use VS Code Desktop (local) and that connects to
Microsoft's marketplace.
https://github.com/sharkymark/v2-templates/blob/main/vs-code-server/main.tf

> Note: these are example templates with no SLAs on them and are not guaranteed
> for long-term support.

</details>

<details style="margin-bottom: 28px;">
  <summary style="font-size: larger; font-weight: bold;">I want to run Docker for my workspaces but not install Docker Desktop.</summary>

[Colima](https://github.com/abiosoft/colima) is a Docker Desktop alternative.

This example is meant for a users who want to try out Coder on a macOS device.

Install Colima and docker with:

```sh
brew install colima
brew install docker
```

Start Colima:

```sh
colima start
```

Start Colima with specific compute options:

```sh
colima start --cpu 4 --memory 8
```

Starting Colima on a M3 Macbook Pro:

```sh
colima start --arch x86_64  --cpu 4 --memory 8 --disk 10
```

Colima will show the path to the docker socket so we have a
[community template](https://github.com/sharkymark/v2-templates/tree/main/docker-code-server)
that prompts the Coder admin to enter the docker socket as a Terraform variable.

</details>

<details style="margin-bottom: 28px;">
  <summary style="font-size: larger; font-weight: bold;">How to make a `coder_app` optional?</summary>

An example use case is the user should decide if they want a browser-based IDE
like code-server when creating the workspace.

1. Add a `coder_parameter` with type `bool` to ask the user if they want the
   code-server IDE

```hcl
data "coder_parameter" "code_server" {
  name        = "Do you want code-server in your workspace?"
  description = "Use VS Code in a browser."
  type        = "bool"
  default     = false
  mutable     = true
  icon        = "/icon/code.svg"
  order       = 6
}
```

2. Add conditional logic to the `startup_script` to install and start
   code-server depending on the value of the added `coder_parameter`

```sh
# install and start code-server, VS Code in a browser

if [ ${data.coder_parameter.code_server.value} = true ]; then
  echo "🧑🏼‍💻 Downloading and installing the latest code-server IDE..."
  curl -fsSL https://code-server.dev/install.sh | sh
  code-server --auth none --port 13337 >/dev/null 2>&1 &
fi
```

3. Add a Terraform meta-argument
   [`count`](https://developer.hashicorp.com/terraform/language/meta-arguments/count)
   in the `coder_app` resource so it will only create the resource if the
   `coder_parameter` is `true`

```hcl
# code-server
resource "coder_app" "code-server" {
  count         = data.coder_parameter.code_server.value ? 1 : 0
  agent_id      = coder_agent.coder.id
  slug          = "code-server"
  display_name  = "code-server"
  icon          = "/icon/code.svg"
  url           = "http://localhost:13337?folder=/home/coder"
  subdomain = false
  share     = "owner"

  healthcheck {
    url       = "http://localhost:13337/healthz"
    interval  = 3
    threshold = 10
  }
}
```

</details>

<details style="margin-bottom: 28px;">
  <summary style="font-size: larger; font-weight: bold;">Why am I getting this "remote host doesn't meet VS Code Server's prerequisites" error when opening up VSCode remote in a Linux environment?</summary>

![VS Code Server prerequisite](https://github.com/coder/coder/assets/10648092/150c5996-18b1-4fae-afd0-be2b386a3239)

It is because, more than likely, the supported OS of either the container image
or VM/VPS doesn't have the proper C libraries to run the VS Code Server. For
instance, Alpine is not supported at all. If so, you need to find a container
image or supported OS for the VS Code Server. For more information on OS
prerequisites for Linux, please look at the VSCode docs.
https://code.visualstudio.com/docs/remote/linux#_local-linux-prerequisites

</details>

<details style="margin-bottom: 28px;">
  <summary style="font-size: larger; font-weight: bold;">How can I resolve disconnects when connected to Coder via JetBrains Gateway?</summary>

If you leave your JetBrains IDE open for some time while connected to Coder, you
may encounter a message similar to the below:

```console
No internet connection. Changes in the document might be lost. Trying to reconnect…
```

To resolve this, add this entry to your SSH host file on your local machine:

```console
Host coder-jetbrains--*
  ServerAliveInterval 5
```

Note that your SSH config file will be overwritten by the JetBrains Gateway
client if it is re-authenticated to your Coder deployment.
