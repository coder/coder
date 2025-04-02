# Networking

Coder's network topology has three types of nodes: workspaces, coder servers,
and users.

The coder server must have an inbound address reachable by users and workspaces,
but otherwise, all topologies _just work_ with Coder.

When possible, we establish direct connections between users and workspaces.
Direct connections are as fast as connecting to the workspace outside of Coder.
When NAT traversal fails, connections are relayed through the coder server. All
user-workspace connections are end-to-end encrypted.

[Tailscale's open source](https://tailscale.com) backs our websocket/HTTPS
networking logic.

## Requirements

In order for clients and workspaces to be able to connect:

> [!NOTE]
> We strongly recommend that clients connect to Coder and their
> workspaces over a good quality, broadband network connection. The following
> are minimum requirements:
>
> - better than 400ms round-trip latency to the Coder server and to their
>   workspace
> - better than 0.5% random packet loss

- All clients and agents must be able to establish a connection to the Coder
  server (`CODER_ACCESS_URL`) over HTTP/HTTPS.
- Any reverse proxy or ingress between the Coder control plane and
  clients/agents must support WebSockets.

In order for clients to be able to establish direct connections:

> [!NOTE]
> Direct connections via the web browser are not supported. To improve
> latency for browser-based applications running inside Coder workspaces in
> regions far from the Coder control plane, consider deploying one or more
> [workspace proxies](./workspace-proxies.md).

- The client is connecting using the CLI (e.g. `coder ssh` or
  `coder port-forward`). Note that the
  [VSCode extension](https://marketplace.visualstudio.com/items?itemName=coder.coder-remote)
  and [JetBrains Plugin](https://plugins.jetbrains.com/plugin/19620-coder/), and
  [`ssh coder.<workspace>`](../../reference/cli/config-ssh.md) all utilize the
  CLI to establish a workspace connection.
- Either the client or workspace agent are able to discover a reachable
  `ip:port` of their counterpart. If the agent and client are able to
  communicate with each other using their locally assigned IP addresses, then a
  direct connection can be established immediately. Otherwise, the client and
  agent will contact
  [the configured STUN servers](../../reference/cli/server.md#--derp-server-stun-addresses)
  to try and determine which `ip:port` can be used to communicate with their
  counterpart. See [STUN and NAT](./stun.md) for more details on how this
  process works.
- All outbound UDP traffic must be allowed for both the client and the agent on
  **all ports** to each others' respective networks.
  - To establish a direct connection, both agent and client use STUN. This
    involves sending UDP packets outbound on `udp/3478` to the configured
    [STUN server](../../reference/cli/server.md#--derp-server-stun-addresses).
    If either the agent or the client are unable to send and receive UDP packets
    to a STUN server, then direct connections will not be possible.
  - Both agents and clients will then establish a
    [WireGuard](https://www.wireguard.com/)️ tunnel and send UDP traffic on
    ephemeral (high) ports. If a firewall between the client and the agent
    blocks this UDP traffic, direct connections will not be possible.

## coder server

Workspaces connect to the coder server via the server's external address, set
via [`ACCESS_URL`](../../admin/setup/index.md#access-url). There must not be a
NAT between workspaces and coder server.

Users connect to the coder server's dashboard and API through its `ACCESS_URL`
as well. There must not be a NAT between users and the coder server.

Template admins can overwrite the site-wide access URL at the template level by
leveraging the `url` argument when
[defining the Coder provider](https://registry.terraform.io/providers/coder/coder/latest/docs#url-1):

```terraform
provider "coder" {
  url = "https://coder.namespace.svc.cluster.local"
}
```

This is useful when debugging connectivity issues between the workspace agent
and the Coder server.

## Web Apps

The coder servers relays dashboard-initiated connections between the user and
the workspace. Web terminal <-> workspace connections are an exception and may
be direct.

In general, [port forwarded](./port-forwarding.md) web apps are faster than
dashboard-accessed web apps.

## 🌎 Geo-distribution

### Direct connections

Direct connections are a straight line between the user and workspace, so there
is no special geo-distribution configuration. To speed up direct connections,
move the user and workspace closer together.

Establishing a direct connection can be an involved process because both the
client and workspace agent will likely be behind at least one level of NAT,
meaning that we need to use STUN to learn the IP address and port under which
the client and agent can both contact each other. See [STUN and NAT](./stun.md)
for more information on how this process works.

If a direct connection is not available (e.g. client or server is behind NAT),
Coder will use a relayed connection. By default,
[Coder uses Google's public STUN server](../../reference/cli/server.md#--derp-server-stun-addresses),
but this can be disabled or changed for
[offline deployments](../../install/offline.md).

### Relayed connections

By default, your Coder server also runs a built-in DERP relay which can be used
for both public and [offline deployments](../../install/offline.md).

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
coder server, so they can only be geo-distributed with High Availability mode in
our Premium Edition. [Reach out to Sales](https://coder.com/contact) to learn
more.

## Browser-only connections

> [!NOTE]
> Browser-only connections is a Premium feature.
> [Learn more](https://coder.com/pricing#compare-plans).

Some Coder deployments require that all access is through the browser to comply
with security policies. In these cases, pass the `--browser-only` flag to
`coder server` or set `CODER_BROWSER_ONLY=true`.

With browser-only connections, developers can only connect to their workspaces
via the web terminal and
[web IDEs](../../user-guides/workspace-access/web-ides.md).

### Workspace Proxies

> [!NOTE]
> Workspace proxies are a Premium feature.
> [Learn more](https://coder.com/pricing#compare-plans).

Workspace proxies are a Coder Premium feature that allows you to provide
low-latency browser experiences for geo-distributed teams.

To learn more, see [Workspace Proxies](./workspace-proxies.md).

## Up next

- Learn about [Port Forwarding](./port-forwarding.md)
- Troubleshoot [Networking Issues](./troubleshooting.md)
