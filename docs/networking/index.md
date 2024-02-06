# Networking

Coder's network topology has three types of nodes: workspaces, coder servers,
and users.

The coder server must have an inbound address reachable by users and workspaces,
but otherwise, all topologies _just work_ with Coder.

When possible, we establish direct connections between users and workspaces.
Direct connections are as fast as connecting to the workspace outside of Coder.
When NAT traversal fails, connections are relayed through the coder server. All
user <-> workspace connections are end-to-end encrypted.

[Tailscale's open source](https://tailscale.com) backs our networking logic.

## Requirements

In order for clients and workspaces to be able to connect:

- All clients and agents must be able to establish a connection to the Coder
  server (`CODER_ACCESS_URL`) over HTTP/HTTPS.
- Any reverse proxy or ingress between the Coder control plane and clients must
  support WebSockets.

In order for clients to be able to establish direct connections:

> **Note:** Direct connections via the web browser are not supported. To improve
> latency for browser-based applications running inside Coder workspaces,
> consider deploying one or more
> [workspace proxies](../admin/workspace-proxies.md).

- The client is connecting using the CLI (e.g. `coder ssh` or
  `coder port-forward`).
- The client and workspace agent are both able to connect to [a specific STUN server](#stun-in-a-natshell).
- All outbound UDP traffic must be allowed for both the client and the agent
  on **all ports** to each others' respective networks.
  - To establish a direct connection, both agent and client use STUN. This involves sending UDP packets outbound on `udp/3478` to the configured [STUN server](../cli/server.md#--derp-server-stun-addresses). If either the agent or the client are unable to send and receive UDP packets to a STUN server, then direct connections will not be possible.
  - Both agents and clients will then establish a [WireGuard](https://www.wireguard.com/)️ tunnel and send UDP traffic on ephemeral (high) ports. If a firewall between the client and the agent blocks this UDP traffic, direct connections will not be possible.

### STUN in a NATshell

> [Session Traversal Utilities for NAT (STUN)](https://www.rfc-editor.org/rfc/rfc8489.html) is a protocol used to assist applications in establishing peer-to-peer communications across Network Address Translations (NATs) or firewalls.
>
> [Network Address Translation (NAT)](https://en.wikipedia.org/wiki/Network_address_translation) is commonly used in private networks to allow mulitple devices to share a single public IP address. The vast majority of ISPs today use at least one level of NAT.

Normally, both Coder agent and client will be running in different _private_ networks (e.g. `192.168.1.0/24`). In order for them to communicate with each other, they will each need their counterpart's public IP address and port.

Inside of that network, packets from the agent or client will show up as having source address `192.168.1.X:12345`. Howver, outside of this private network, the source address will show up differently (for example, `12.3.4.56:54321`). In order for the Coder client and agent to establish a direct connection with each other, one of them needs to know the `ip:port` pair under which their counterpart can be reached.

In order to accomplish this, both client and agent can use a STUN server:

> The below glosses over a lot of the complexity of traversing NATs.
> For a more in-depth technical explanation, see [How NAT traversal works (tailscale.com)](https://tailscale.com/blog/how-nat-traversal-works).

- **Discovery:** Both the client and agent will send UDP traffic to a configured STUN server. This STUN server is generally located on the public internet, and responds with the public IP address and port from which the request came.
- **Port Mapping:** The client and agent then exchange this information through the Coder server. They will then construct packets that should be able to successfully traverse their counterpart's NATs successfully.
- **NAT Traversal:** The client and agent then send these crafted packets to their counterpart's public addresses. If all goes well, the NATs on the other end should route these packets to the correct internal address.
- **Connection:** Once the packets reach the other side, they send a response back to the source `ip:port` from the packet. Again, the NATs should recognize these responses as belonging to an ongoing communication, and forward them accordingly.

At this point, both the client and agent should be able to send traffic directly to each other.

## coder server

Workspaces connect to the coder server via the server's external address, set
via [`ACCESS_URL`](../admin/configure.md#access-url). There must not be a NAT
between workspaces and coder server.

Users connect to the coder server's dashboard and API through its `ACCESS_URL`
as well. There must not be a NAT between users and the coder server.

Template admins can overwrite the site-wide access URL at the template level by
leveraging the `url` argument when
[defining the Coder provider](https://registry.terraform.io/providers/coder/coder/latest/docs#url):

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

If a direct connection is not available (e.g. client or server is behind NAT),
Coder will use a relayed connection. By default,
[Coder uses Google's public STUN server](../cli/server.md#--derp-server-stun-addresses),
but this can be disabled or changed for
[offline deployments](../install/offline.md).

### Relayed connections

By default, your Coder server also runs a built-in DERP relay which can be used
for both public and [offline deployments](../install/offline.md).

However, Tailscale has graciously allowed us to use
[their global DERP relays](https://tailscale.com/kb/1118/custom-derp-servers/#what-are-derp-servers).
You can launch `coder server` with Tailscale's DERPs like so:

```bash
$ coder server --derp-config-url https://controlplane.tailscale.com/derpmap/default
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
$ coder server --derp-config-path derpmap.json
```

### Dashboard connections

The dashboard (and web apps opened through the dashboard) are served from the
coder server, so they can only be geo-distributed with High Availability mode in
our Enterprise Edition. [Reach out to Sales](https://coder.com/contact) to learn
more.

## Browser-only connections (enterprise)

Some Coder deployments require that all access is through the browser to comply
with security policies. In these cases, pass the `--browser-only` flag to
`coder server` or set `CODER_BROWSER_ONLY=true`.

With browser-only connections, developers can only connect to their workspaces
via the web terminal and [web IDEs](../ides/web-ides.md).

## Troubleshooting

The `coder ping -v <workspace>` will ping a workspace and return debug logs for
the connection. We recommend running this command and inspecting the output when
debugging SSH connections to a workspace. For example:

```console
$ coder ping -v my-workspace

2023-06-21 17:50:22.412 [debu] wgengine: ping(fd7a:115c:a1e0:49d6:b259:b7ac:b1b2:48f4): sending disco ping to [cFYPo] ...
pong from my-workspace proxied via DERP(Denver) in 90ms
2023-06-21 17:50:22.503 [debu] wgengine: magicsock: closing connection to derp-13 (conn-close), age 5s
2023-06-21 17:50:22.503 [debu] wgengine: magicsock: 0 active derp conns
2023-06-21 17:50:22.504 [debu] wgengine: wg: [v2] Routine: receive incoming v6 - stopped
2023-06-21 17:50:22.504 [debu] wgengine: wg: [v2] Device closed
```

The `coder speedtest <workspace>` command measures user <-> workspace
throughput. E.g.:

```
$ coder speedtest dev
29ms via coder
Starting a 5s download test...
INTERVAL       TRANSFER         BANDWIDTH
0.00-1.00 sec  630.7840 MBits   630.7404 Mbits/sec
1.00-2.00 sec  913.9200 MBits   913.8106 Mbits/sec
2.00-3.00 sec  943.1040 MBits   943.0399 Mbits/sec
3.00-4.00 sec  933.3760 MBits   933.2143 Mbits/sec
4.00-5.00 sec  848.8960 MBits   848.7019 Mbits/sec
5.00-5.02 sec  13.5680 MBits    828.8189 Mbits/sec
----------------------------------------------------
0.00-5.02 sec  4283.6480 MBits  853.8217 Mbits/sec
```

## Up next

- Learn about [Port Forwarding](./port-forwarding.md)
