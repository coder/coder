# Deployment Health

Coder includes an operator-friendly deployment health page that provides a
number of details about the health of your Coder deployment.

You can view it at `https://${CODER_URL}/health`, or you can alternatively view
the [JSON response directly](../api/debug.md#debug-info-deployment-health).

The deployment health page is broken up into the following sections:

## Access URL

The Access URL section shows checks related to Coder's
[access URL](./configure.md#access-url).

Coder will periodically send a GET request to `${CODER_ACCESS_URL}/healthz` and
validate that the response is `200 OK`. The expected response body is also the
string `OK`.

If there is an issue, you may see one of the following errors reported:

### EACS01

_Access URL not set_

**Problem:** no access URL has been configured.

**Solution:** configure an [access URL](./configure.md#access-url) for Coder.

### EACS02

_Access URL invalid_

**Problem:** `${CODER_ACCESS_URL}/healthz` is not a valid URL.

**Solution:** Ensure that the access URL is a valid URL accepted by
[`url.Parse`](https://pkg.go.dev/net/url#Parse). Example:
`https://dev.coder.com/`.

> **Tip:** You can check this [here](https://go.dev/play/p/CabcJZyTwt9).

### EACS03

_Failed to fetch `/healthz`_

**Problem:** Coder was unable to execute a GET request to
`${CODER_ACCESS_URL}/healthz`.

This could be due to a number of reasons, including but not limited to:

- DNS lookup failure
- A misconfigured firewall
- A misconfigured reverse proxy
- Invalid or expired SSL certificates

**Solution:** Investigate and resolve the root cause of the connection issue.

To troubleshoot further, you can log into the machine running Coder and attempt
to run the following command:

```shell
curl -v ${CODER_ACCESS_URL}/healthz
# Expected output:
# *   Trying XXX.XXX.XXX.XXX:443
# * Connected to https://coder.company.com (XXX.XXX.XXX.XXX) port 443 (#0)
# [...]
# OK
```

The output of this command should aid further diagnosis.

### EACS04

_/healthz did not return 200 OK_

**Problem:** Coder was able to execute a GET request to
`${CODER_ACCESS_URL}/healthz`, but the response code was not `200 OK` as
expected.

This could mean, for instance, that:

- The request did not actually hit your Coder instance (potentially an incorrect
  DNS entry)
- The request hit your Coder instance, but on an unexpected path (potentially a
  misconfigured reverse proxy)

**Solution:** Inspect the `HealthzResponse` in the health check output. This
should give you a good indication of the root cause.

## Database

Coder continuously executes a short database query to validate that it can reach
its configured database, and also measures the median latency over 5 attempts.

### EDB01

_Database Ping Failed_

**Problem:** This error code is returned if any attempt to execute this database
query fails.

**Solution:** Investigate the health of the database.

### EDB02

_Database Latency High_

**Problem:** This code is returned if the median latency is higher than the
[configured threshold](../cli/server.md#--health-check-threshold-database). This
may not be an error as such, but is an indication of a potential issue.

**Solution:** Investigate the sizing of the configured database with regard to
Coder's current activity and usage. It may be necessary to increase the
resources allocated to Coder's database. Alternatively, you can raise the
configured threshold to a higher value (this will not address the root cause).

> [!TIP]
>
> - You can enable
>   [detailed database metrics](../cli/server.md#--prometheus-collect-db-metrics)
>   in Coder's Prometheus endpoint.
> - If you have [tracing enabled](../cli/server.md#--trace), these traces may
>   also contain useful information regarding Coder's database activity.

## DERP

Coder workspace agents may use
[DERP (Designated Encrypted Relay for Packets)](https://tailscale.com/blog/how-tailscale-works/#encrypted-tcp-relays-derp)
to communicate with Coder. This requires connectivity to a number of configured
[DERP servers](../cli/server.md#--derp-config-path) which are used to relay
traffic between Coder and workspace agents. Coder periodically queries the
health of its configured DERP servers and may return one or more of the
following:

### EDERP01

_DERP Node Uses Websocket_

**Problem:** When Coder attempts to establish a connection to one or more DERP
servers, it sends a specific `Upgrade: derp` HTTP header. Some load balancers
may block this header, in which case Coder will fall back to
`Upgrade: websocket`.

This is not necessarily a fatal error, but a possible indication of a
misconfigured reverse HTTP proxy. Additionally, while workspace users should
still be able to reach their workspaces, connection performance may be degraded.

> **Note:** This may also be shown if you have
> [forced websocket connections for DERP](../cli/server.md#--derp-force-websockets).

**Solution:** ensure that any proxies you use allow connection upgrade with the
`Upgrade: derp` header.

### EDERP02

_One or more DERP nodes are unhealthy_

**Problem:** This is shown if Coder is unable to reach one or more configured
DERP servers. Clients will fall back to use the remaining DERP servers, but
performance may be impacted for clients closest to the unhealthy DERP server.

**Solution:** Ensure that the DERP server is available and reachable over the
network, for example:

```shell
curl -v "https://coder.company.com/derp"
# Expected output:
# *   Trying XXX.XXX.XXX.XXX
# * Connected to https://coder.company.com (XXX.XXX.XXX.XXX) port 443 (#0)
# DERP requires connection upgrade
```

### ESTUN01

_No STUN servers available._

**Problem:** This is shown if no STUN servers are available. Coder will use STUN
to establish [direct connections](../networking/stun.md). Without at least one
working STUN server, direct connections may not be possible.

**Solution:** Ensure that the
[configured STUN severs](../cli/server.md#derp-server-stun-addresses) are
reachable from Coder and that UDP traffic can be sent/received on the configured
port.

### ESTUN02

_STUN returned different addresses; you may be behind a hard NAT._

**Problem:** This is a warning shown when multiple attempts to determine our
public IP address/port via STUN resulted in different `ip:port` combinations.
This is a sign that you are behind a "hard NAT", and may result in difficulty
establishing direct connections. However, it does not mean that direct
connections are impossible.

**Solution:** Engage with your network administrator.

## Websocket

Coder makes heavy use of [WebSockets](https://datatracker.ietf.org/doc/rfc6455/)
for long-lived connections:

- Between users interacting with Coder's Web UI (for example, the built-in
  terminal, or VSCode Web),
- Between workspace agents and `coderd`,
- Between Coder [workspace proxies](../admin/workspace-proxies.md) and `coderd`.

Any issues causing failures to establish WebSocket connections will result in
**severe** impairment of functionality for users. To validate this
functionality, Coder will periodically attempt to establish a WebSocket
connection with itself using the configured [Access URL](#access-url), send a
message over the connection, and attempt to read back that same message.

### EWS01

_Failed to establish a WebSocket connection_

**Problem:** Coder was unable to establish a WebSocket connection over its own
Access URL.

**Solution:** There are multiple possible causes of this problem:

1. Ensure that Coder's configured Access URL can be reached from the server
   running Coder, using standard troubleshooting tools like `curl`:

   ```shell
   curl -v "https://coder.company.com"
   ```

2. Ensure that any reverse proxy that is serving Coder's configured access URL
   allows connection upgrade with the header `Upgrade: websocket`.

### EWS02

_Failed to echo a WebSocket message_

**Problem:** Coder was able to establish a WebSocket connection, but was unable
to write a message.

**Solution:** There are multiple possible causes of this problem:

1. Validate that any reverse proxy servers in front of Coder's configured access
   URL are not prematurely closing the connection.
2. Validate that the network link between Coder and the workspace proxy is
   stable, e.g. by using `ping`.
3. Validate that any internal network infrastructure (for example, firewalls,
   proxies, VPNs) do not interfere with WebSocket connections.

## Workspace Proxy

If you have configured [Workspace Proxies](../admin/workspace-proxies.md), Coder
will periodically query their availability and show their status here.

### EWP01

_Error Updating Workspace Proxy Health_

**Problem:** Coder was unable to query the connected workspace proxies for their
health status.

**Solution:** This may be a transient issue. If it persists, it could signify a
connectivity issue.

### EWP02

_Error Fetching Workspace Proxies_

**Problem:** Coder was unable to fetch the stored workspace proxy health data
from the database.

**Solution:** This may be a transient issue. If it persists, it could signify an
issue with Coder's configured database.

### EWP04

_One or more Workspace Proxies Unhealthy_

**Problem:** One or more workspace proxies are not reachable.

**Solution:** Ensure that Coder can establish a connection to the configured
workspace proxies.

### EPD01

_No Provisioner Daemons Available_

**Problem:** No provisioner daemons are registered with Coder. No workspaces can
be built until there is at least one provisioner daemon running.

**Solution:**

If you are using
[External Provisioner Daemons](./provisioners.md#external-provisioners), ensure
that they are able to successfully connect to Coder. Otherwise, ensure
[`--provisioner-daemons`](../cli/server.md#provisioner-daemons) is set to a
value greater than 0.

> Note: This may be a transient issue if you are currently in the process of
> updating your deployment.

### EPD02

_Provisioner Daemon Version Mismatch_

**Problem:** One or more provisioner daemons are more than one major or minor
version out of date with the main deployment. It is important that provisioner
daemons are updated at the same time as the main deployment to minimize the risk
of API incompatibility.

**Solution:** Update the provisioner daemon to match the currently running
version of Coder.

> Note: This may be a transient issue if you are currently in the process of
> updating your deployment.

### EPD03

_Provisioner Daemon API Version Mismatch_

**Problem:** One or more provisioner daemons are using APIs that are marked as
deprecated. These deprecated APIs may be removed in a future release of Coder,
at which point the affected provisioner daemons will no longer be able to
connect to Coder.

**Solution:** Update the provisioner daemon to match the currently running
version of Coder.

> Note: This may be a transient issue if you are currently in the process of
> updating your deployment.

## EUNKNOWN

_Unknown Error_

**Problem:** This error is shown when an unexpected error occurred evaluating
deployment health. It may resolve on its own.

**Solution:** This may be a bug.
[File a GitHub issue](https://github.com/coder/coder/issues/new)!
