# Networking

Coder's network topology has three types of nodes:
workspaces, coder servers, and users.

The coder server must have an inbound address reachable by users and workspaces,
but otherwise, all topologies _just work_ with Coder.

When possible, we establish direct connections between users and workspaces.
Direct connections are as fast as connecting to the workspace outside of Coder.
When NAT traversal fails, connections are relayed through the coder server.
All user <-> workspace connections are end-to-end encrypted.

[Tailscale's open source](https://tailscale.com) backs our networking logic.

## coder server

Workspaces connect to the coder server via the server's external address,
set via [`ACCESS_URL`](./admin/configure.md#access-url). There must not be a
NAT between workspaces and coder server.

Users connect to the coder server's dashboard and API through its `ACCESS_URL`
as well. There must not be a NAT between users and the coder server.

Template admins can overwrite the site-wide access URL at the template level by
leveraging the `url` argument when [defining the Coder provider](https://registry.terraform.io/providers/coder/coder/latest/docs#url):

```terraform
provider "coder" {
  url = "https://coder.namespace.svc.cluster.local"
}
```

This is useful when debugging connectivity issues between the workspace agent and
the Coder server.

## Web Apps

The coder servers relays dashboard-initiated connections between the user and
the workspace. Web terminal <-> workspace connections are an exception and may be direct.

In general, [port forwarded](./networking/port-forwarding.md) web apps are
faster than dashboard-accessed web apps.

## 🌎 Geo-distribution

### Direct connections

Direct connections are a straight line between the user and workspace, so there
is no special geo-distribution configuration. To speed up direct connections,
move the user and workspace closer together.

### Relayed connections

Tailscale has graciously allowed us to use
[their global DERP relays](https://tailscale.com/kb/1118/custom-derp-servers/#what-are-derp-servers).

You can launch `coder server` with Tailscale's DERPs like so:

```bash
$ coder server --derp-config-url https://controlplane.tailscale.com/derpmap/default
```

#### Custom Relays

If you run Coder in air-gap mode or want lower latency than what Tailscale offers,
you may run custom DERP servers. Refer to
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

## Troubleshooting

The `coder speedtest <workspace>` command measures user <-> workspace throughput.
E.g.:

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

- Learn about [Port Forwarding](./networking/port-forwarding.md)
