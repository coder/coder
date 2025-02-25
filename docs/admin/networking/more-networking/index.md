# Connections and geo-distribution

## SSH and browser connections

Coder workspaces have SSH support which allows the use of desktop editors such
as VS Code remote connections and JetBrains Gateway.

SSH connections require that you open port 443 on the server.

Coder does not require workspaces to have port 22 open, an OpenSSH server running,
nor SSH keys.
Instead, Coder mimics SSH over an HTTPS tunnel and uses the user’s session token
through the CLI to authenticate.
This is more secure and portable than SSH relays, bastion hosts, and other methods
because it ensures that only the proper user session can establish SSH connections.

Administrators can disable this by enabling [browser-only mode](#browser-only-connections),
allowing only connections to workspaces through the browser like code-server,
web terminal, web RDP, and others.

### Browser-only connections

<blockquote class="info">

Browser-only connections is an Enterprise and Premium feature.
[Learn more](https://coder.com/pricing#compare-plans).

</blockquote>

Some Coder deployments require that all access is through the browser to comply
with security policies. In these cases, pass the `--browser-only` flag to
`coder server` or set `CODER_BROWSER_ONLY=true`.

With browser-only connections, developers can only connect to their workspaces
via the web terminal and
[web IDEs](../../../user-guides/workspace-access/web-ides.md).

## 🌎 Geo-distribution

Workspace proxies and provisioners can be deployed for low-latency access to
workspaces for distributed teams.

By default, Coder will attempt to create direct peer-to-peer connections between
the client (developer laptop) and workspace.
If this doesn’t work, the end result will be transparent to the end user because
Coder will fall back to connections relayed to the control plane.

Since Coder supports deploying resources in multiple regions, developers will want a fast connection to those workspaces. Workspace proxies are designed to relay traffic to workspaces without having to route traffic to the central Coder server. Both web traffic and SSH traffic is relayed through workspace proxies.

### Workspace Proxies

[Workspace Proxies](../workspace-proxies.md) are a
[Premium](https://coder.com/pricing#compare-plans) feature that allows you to
provide low-latency browser experiences for geo-distributed teams.

### Direct connections

Direct connections reduce latency and improve upload and download speeds for developers.
However, there are many scenarios where direct connections cannot be established,
such as when the Coder [administrators disable direct connections](../../../reference/cli/server.md#--block-direct-connections).

Consult the [direct connections section](../troubleshooting.md#common-problems-with-direct-connections)
of the troubleshooting guide for more information.
The troubleshooting guide also explains how to identify if a connection is direct
or not via the `coder ping` command.

Ideally, to speed up direct connections, move the user and workspace closer together.

Establishing a direct connection can be an involved process because both the
client and workspace agent will likely be behind at least one level of NAT,
meaning that we need to use STUN to learn the IP address and port under which
the client and agent can both contact each other. See [STUN and NAT](../stun.md)
for more information on how this process works.

If a direct connection is not available (e.g. client or server is behind NAT),
Coder will use a relayed connection. By default,
[Coder uses Google's public STUN server](../../../reference/cli/server.md#--derp-server-stun-addresses),
but this can be disabled or changed for
[offline deployments](../../../install/offline.md).

In order for clients to be able to establish direct connections:

> **Note:** Direct connections via the web browser are not supported. To improve
> latency for browser-based applications running inside Coder workspaces in
> regions far from the Coder control plane, consider deploying one or more
> [workspace proxies](../workspace-proxies.md).

- The client is connecting using the CLI (e.g. `coder ssh` or
  `coder port-forward`). Note that the
  [VSCode extension](https://marketplace.visualstudio.com/items?itemName=coder.coder-remote)
  and [JetBrains Plugin](https://plugins.jetbrains.com/plugin/19620-coder/), and
  [`ssh coder.<workspace>`](../../../reference/cli/config-ssh.md) all utilize the
  CLI to establish a workspace connection.
- Either the client or workspace agent are able to discover a reachable
  `ip:port` of their counterpart. If the agent and client are able to
  communicate with each other using their locally assigned IP addresses, then a
  direct connection can be established immediately. Otherwise, the client and
  agent will contact
  [the configured STUN servers](../../../reference/cli/server.md#--derp-server-stun-addresses)
  to try and determine which `ip:port` can be used to communicate with their
  counterpart. See [STUN and NAT](../stun.md) for more details on how this
  process works.
- All outbound UDP traffic must be allowed for both the client and the agent on
  **all ports** to each others' respective networks.
  - To establish a direct connection, both agent and client use STUN. This
    involves sending UDP packets outbound on `udp/3478` to the configured
    [STUN server](../../../reference/cli/server.md#--derp-server-stun-addresses).
    If either the agent or the client are unable to send and receive UDP packets
    to a STUN server, then direct connections will not be possible.
  - Both agents and clients will then establish a
    [WireGuard](https://www.wireguard.com/)️ tunnel and send UDP traffic on
    ephemeral (high) ports. If a firewall between the client and the agent
    blocks this UDP traffic, direct connections will not be possible.

### Relayed connections

By default, your Coder server also runs a built-in DERP relay which can be used
for both public and [offline deployments](../../../install/offline.md).

However, our Wireguard integration through Tailscale has graciously allowed us
to use
[their global DERP relays](https://tailscale.com/kb/1118/custom-derp-servers/#what-are-derp-servers).
You can launch `coder server` with Tailscale's DERPs like so:

```bash
coder server --derp-config-url https://controlplane.tailscale.com/derpmap/default
```

#### Custom Relays

If you want lower latency than what Tailscale offers or want additional DERP
relays for offline deployments, you may run custom DERP servers. Refer to
[Tailscale's documentation](https://tailscale.com/kb/1118/custom-derp-servers/#why-run-your-own-derp-server)
to learn how to set them up.

After you have custom DERP servers, you can launch Coder with them like so:

```json
# derpmap.json
{
  "Regions": {
    "1": {
      "RegionID": 1,
      "RegionCode": "myderp",
      "RegionName": "My DERP",
      "Nodes": [
        {
          "Name": "1",
          "RegionID": 1,
          "HostName": "your-hostname.com"
        }
      ]
    }
  }
}
```

```bash
coder server --derp-config-path derpmap.json
```

### Dashboard connections

The dashboard (and web apps opened through the dashboard) are served from the
Coder server, so they can only be geo-distributed with High Availability mode in
our Premium Edition. [Reach out to Sales](https://coder.com/contact) to learn
more.
