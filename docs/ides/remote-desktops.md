# Remote Desktops

> Built-in remote desktop is on the roadmap ([#2106](https://github.com/coder/coder/issues/2106)).

## VNC Desktop

The common way to use remote desktops with Coder is through VNC.

![VNC Desktop in Coder](../images/vnc-desktop.png)

Workspace requirements:

- VNC server (e.g. [tigervnc](https://tigervnc.org/))
- VNC client (e.g. [novnc](https://novnc.com/info.html))

Installation instructions vary depending on your workspace's operating
system, platform, and build system.

As a starting point, see the [desktop-container](https://github.com/bpmct/coder-templates/tree/main/desktop-container) community template. It builds and provisions a Dockerized workspace with the following software:

- Ubuntu 20.04
- TigerVNC server
- noVNC client
- XFCE Desktop
