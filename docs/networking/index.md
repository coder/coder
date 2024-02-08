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
- Any reverse proxy or ingress between the Coder control plane and
  clients/agents must support WebSockets.

In order for clients to be able to establish direct connections:

> **Note:** Direct connections via the web browser are not supported. To improve
> latency for browser-based applications running inside Coder workspaces in
> regions far from the Coder control plane, consider deploying one or more
> [workspace proxies](../admin/workspace-proxies.md).

- The client is connecting using the CLI (e.g. `coder ssh` or
  `coder port-forward`). Note that the
  [VSCode extension](https://marketplace.visualstudio.com/items?itemName=coder.coder-remote)
  and [JetBrains Plugin](https://plugins.jetbrains.com/plugin/19620-coder/), and
  [`ssh coder.<workspace>`](../cli/config-ssh.md) all utilize the CLI to
  establish a workspace connection.
- Either the client or workspace agent are able to discover a reachable
  `ip:port` of their counterpart. If the agent and client are able to
  communicate with each other using their locally assigned IP addresses, then a
  direct connection can be established immediately. Otherwise, the client and
  agent will contact
  [the configured STUN servers](../cli/server.md#derp-server-stun-addresses) to
  try and determine which `ip:port` can be used to communicate with their
  counterpart. See [below](#stun-in-a-natshell) for more details on how this
  process works.
- All outbound UDP traffic must be allowed for both the client and the agent on
  **all ports** to each others' respective networks.
  - To establish a direct connection, both agent and client use STUN. This
    involves sending UDP packets outbound on `udp/3478` to the configured
    [STUN server](../cli/server.md#--derp-server-stun-addresses). If either the
    agent or the client are unable to send and receive UDP packets to a STUN
    server, then direct connections will not be possible.
  - Both agents and clients will then establish a
    [WireGuard](https://www.wireguard.com/)️ tunnel and send UDP traffic on
    ephemeral (high) ports. If a firewall between the client and the agent
    blocks this UDP traffic, direct connections will not be possible.

### STUN in a NATshell

In order for one application to connect to another across a network, the
connecting application needs to know the IP address and port under which the
target application is reachable. If both applications reside on the same
network, then they can most likely connect directly to each other. In the
context of a Coder workspace agent and client, this is generally not the case,
as both agent and client will most likely be running in different _private_
networks (e.g. `192.168.1.0/24`). In this case, at least one of the two will
need to know an IP address and port under which they can reach their
counterpart.

This problem is often referred to as NAT traversal, and Coder uses a standard
protocol named STUN to address this.

> [Session Traversal Utilities for NAT (STUN)](https://www.rfc-editor.org/rfc/rfc8489.html)
> is a protocol used to assist applications in establishing peer-to-peer
> communications across Network Address Translations (NATs) or firewalls.
>
> [Network Address Translation (NAT)](https://en.wikipedia.org/wiki/Network_address_translation)
> is commonly used in private networks to allow multiple devices to share a
> single public IP address. The vast majority of ISPs today use at least one
> level of NAT.

Inside of that network, packets from the agent or client will show up as having
source address `192.168.1.X:12345`. However, outside of this private network,
the source address will show up differently (for example, `12.3.4.56:54321`). In
order for the Coder client and agent to establish a direct connection with each
other, one of them needs to know the `ip:port` pair under which their
counterpart can be reached. Once communication succeeds in one direction, we can
inspect the source address of the received packet to determine the return
address.

At a high level, STUN works like this:

> The below glosses over a lot of the complexity of traversing NATs. For a more
> in-depth technical explanation, see
> [How NAT traversal works (tailscale.com)](https://tailscale.com/blog/how-nat-traversal-works).

- **Discovery:** Both the client and agent will send UDP traffic to one or more
  configured STUN servers. These STUN servers are generally located on the
  public internet, and respond with the public IP address and port from which
  the request came.
- **Port Mapping:** The client and agent then exchange this information through
  the Coder server. They will then construct packets that should be able to
  successfully traverse their counterpart's NATs successfully.
- **NAT Traversal:** The client and agent then send these crafted packets to
  their counterpart's public addresses. If all goes well, the NATs on the other
  end should route these packets to the correct internal address.
- **Connection:** Once the packets reach the other side, they send a response
  back to the source `ip:port` from the packet. Again, the NATs should recognize
  these responses as belonging to an ongoing communication, and forward them
  accordingly.

At this point, both the client and agent should be able to send traffic directly
to each other.

Below are some example scenarios:

1. Direct connections without NAT or STUN

    ```mermaid
    flowchart LR
        subgraph corpnet["Private Network\ne.g. Corp. LAN"]
        A[Client Workstation\n192.168.21.47:38297]
        C[Workspace Agent\n192.168.21.147:41563]
        A <--> C
        end
    ```

   In this example, both the client and agent are located on the network `192.168.21.0/24`. Assuming no firewalls are blocking packets in either direction, both client and agent are able to communicate directly with each other's locally assigned IP address.

2. Direct connections with one layer of NAT

    ```mermaid
    flowchart LR
      subgraph homenet["Network A"]
        client["Client workstation\n192.168.1.101:38297"]
        homenat["NAT\n??.??.??.??:?????"]
      end
      subgraph internet["Public Internet"]
        stun1["STUN server"]
      end
      subgraph corpnet["Network B"]
        agent["Workspace agent\n10.21.43.241:56812"]
        corpnat["NAT\n??.??.??.??:?????"]
      end

      client --- homenat
      agent --- corpnat
      corpnat -- "[I see 12.34.56.7:41563]" --> stun1
      homenat -- "[I see 65.4.3.21:29187]" --> stun1
    ```

    In this example, client and agent are located on different networks and connect to each other over the public Internet. Both client and agent connect to a configured STUN server located on the public Internet to determine the public IP address and port on which they can be reached. They then exchange this information through Coder server, and can then communicate directly with each other through their respective NATs.

    ```mermaid
    flowchart LR
      subgraph homenet["Home Network"]
        direction LR
        client["Client workstation\n192.168.1.101:38297"]
        homenat["Home Router/NAT\n65.4.3.21:29187"]
      end
      subgraph corpnet["Corp Network"]
        direction LR
        agent["Workspace agent\n10.21.43.241:56812"]
        corpnat["Corp Router/NAT\n12.34.56.7:41563"]
      end

      subgraph internet["Public Internet"]
      end

      client -- "[12.34.56.7:41563]" --- homenat
      homenat -- "[12.34.56.7:41563]" --- internet
      internet -- "[12.34.56.7:41563]" --- corpnat
      corpnat -- "[10.21.43.241:56812]" --> agent
    ```

3. Direct connections with VPN.

    In this example, the client workstation must use a VPN to connect to the corporate network. All traffic from the client will enter through the VPN entry node and exit at the VPN exit node inside the corporate network. Traffic from the client inside the corporate network will appear to be coming from the IP address of the VPN exit node `172.16.1.2`. Traffic from the client to the public internet will appear to have the public IP address of the corporate router `12.34.56.7`.

    The workspace agent is running on a Kubernetes cluster inside the corporate network, which is behind its own layer of NAT. To anyone inside the corporate network but outside the cluster network, its traffic will appear to be coming from `172.16.1.254`. However, traffic from the agent  to services on the public Internet will also see traffic originating from the public IP address assigned to the corporate router.

    If the client and agent both use the public STUN server, the addresses discoverd by STUN will both be the public IP address of the corporate router. To correctly route the traffic backwards, the corporate router must correctly map packets sent from the client to the cluster router, and from the agent to the VPN exit node. This behaviour is known as "hairpinning", and does not work in all cases.

    In this configuration, deploying an internal STUN server can aid establishing direct connections between client and agent. When the agent and client query this internal STUN server, they will be able to determine the addresses on the corporate network from which their traffic appears to originate. Using these internal addresses is much more likely to result in a successful direct connection.

    ```mermaid
    flowchart TD
      subgraph homenet["Home Network"]
        client["Client workstation\n192.168.1.101"]
        homenat["Home Router/NAT\n65.4.3.21"]
      end

      subgraph internet["Public Internet"]
        stun1["Public STUN"]
        vpn1["VPN entry node"]
      end

      subgraph corpnet["Corp Network 172.16.1.0/24"]
        corpnat["Corp Router/NAT\n172.16.1.1\n12.34.56.7"]
        vpn2["VPN exit node\n172.16.1.2"]
        stun2["Private STUN"]

        subgraph cluster["Cluster Network 10.11.12.0/16"]
          clusternat["Cluster Router/NAT\n10.11.12.1\n172.16.1.254"]
          agent["Workspace agent\n10.11.12.34"]
        end
      end

      vpn1 === vpn2
      vpn2 --> stun2
      client === homenat
      homenat === vpn1
      homenat x-.-x stun1
      agent --- clusternat
      clusternat --- corpnat
      corpnat --> stun1
      corpnat --> stun2
    ```

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

Establishing a direct connection can be an involved process because both the
client and workspace agent will likely be behind at least one level of NAT,
meaning that we need to use STUN to learn the IP address and port under which
the client and agent can both contact each other. See
[above](#stun-in-a-natshell) for more information on how this process works.

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
