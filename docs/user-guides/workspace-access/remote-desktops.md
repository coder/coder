# Remote Desktops

## VNC Desktop

The common way to use remote desktops with Coder is through VNC.

![VNC Desktop in Coder](../../images/vnc-desktop.png)

Workspace requirements:

- VNC server (e.g. [tigervnc](https://tigervnc.org/))
- VNC client (e.g. [novnc](https://novnc.com/info.html))

Installation instructions vary depending on your workspace's operating system,
platform, and build system.

As a starting point, see the
[enterprise-desktop](https://github.com/coder/images/tree/main/images/desktop)
image. It can be used to provision a Dockerized workspace with the
following software:

- Ubuntu 24.04
- XFCE Desktop
- KasmVNC Server and Web Client

## RDP Desktop

To use RDP with Coder, you'll need to install an
[RDP client](https://docs.microsoft.com/en-us/windows-server/remote/remote-desktop-services/clients/remote-desktop-clients)
on your local machine, and enable RDP on your workspace.

<div class="tabs">

### RDP with Coder Desktop

[Coder Desktop](../desktop/index.md)'s Coder Connect feature creates a connection to your workspaces in the background.
There is no need for port forwarding when it is enabled.

Use your favorite RDP client to connect to `<workspace-name>.coder` instead of `localhost:3399`.

You can also use a URI handler to launch an RDP session directly.

The URI format is:

```text
coder://<your Coder server name>/v0/open/ws/<workspace name>/agent/<agent name>/rdp?username=<username>&password=<password>
```

For example:

```text
coder://coder.example.com/v0/open/ws/myworkspace/agent/main/rdp?username=Administrator&password=coderRDP!
```

To include a Coder Desktop button on the workspace dashboard page, add a `coder_app` resource to the template:

```tf
locals {
  server_name = regex("https?:\\/\\/([^\\/]+)", data.coder_workspace.me.access_url)[0]
}

resource "coder_app" "rdp-coder-desktop" {
  agent_id     = resource.coder_agent.main.id
  slug         = "rdp-desktop"
  display_name = "RDP with Coder Desktop"
  url          = "coder://${local.server_name}/v0/open/ws/${data.coder_workspace.me.name}/agent/main/rdp?username=Administrator&password=coderRDP!"
  icon         = "/icon/desktop.svg"
  external     = true
}
```

> [!NOTE]
> Some versions of Windows, including Windows Server 2022, do not communicate correctly over UDP
> when using Coder Connect because they do not respect the maximum transmission unit (MTU) of the link.
> When this happens, the RDP client will appear to connect, but displays a blank screen.
>
> To avoid this error, Coder's [Windows RDP](https://registry.coder.com/modules/windows-rdp) module
> [disables RDP over UDP automatically](https://github.com/coder/registry/blob/b58bfebcf3bcdcde4f06a183f92eb3e01842d270/registry/coder/modules/windows-rdp/powershell-installation-script.tftpl#L22).
>
> To disable RDP over UDP, run the following in PowerShell:
>
> ```powershell
> New-ItemProperty -Path 'HKLM:\SOFTWARE\Policies\Microsoft\Windows NT\Terminal Services' -Name "SelectTransport" -Value 1 -PropertyType DWORD -Force
> Restart-Service -Name "TermService" -Force
> ```

### CLI

Use the following command to forward the RDP port to your local machine:

```console
coder port-forward <workspace-name> --tcp 3399:3389
```

Then, connect to your workspace via RDP at `localhost:3399`.
![windows-rdp](../../images/ides/windows_rdp_client.png)

</div>

> [!NOTE]
> The default username is `Administrator` and the password is `coderRDP!`.

## RDP Web

Our [Windows RDP](https://registry.coder.com/modules/windows-rdp) module in the Coder
Registry adds a one-click button to open an RDP session in the browser. This
requires just a few lines of Terraform in your template, see the documentation
on our registry for setup.

![Windows RDP Module in a Workspace](../../images/user-guides/web-rdp-demo.png)

## Amazon DCV Windows

Our [Amazon DCV Windows](https://registry.coder.com/modules/amazon-dcv-windows)
module adds a one-click button to open an Amazon DCV session in the browser.
This requires just a few lines of Terraform in your template, see the
documentation on our registry for setup.

![Amazon DCV Windows Module in a Workspace](../../images/user-guides/amazon-dcv-windows-demo.png)
