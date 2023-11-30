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

### <a name="EACS01">EACS01: Access URL not set</a>

**Problem:** no access URL has been configured.

**Solution:** configure an [access URL](./configure.md#access-url) for Coder.

### <a name="EACS02">EACS02: Access URL invalid</a>

**Problem:** `${CODER_ACCESS_URL}/healthz` is not a valid URL.

**Solution:** Ensure that the access URL is a valid URL accepted by
[`url.Parse`](https://pkg.go.dev/net/url#Parse). Example:
`https://dev.coder.com/`.

> **Tip:** You can check this [here](https://go.dev/play/p/CabcJZyTwt9).

### <a name="EACS03">EACS03: Failed to fetch `/healthz`</a>

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

### <a name="EACS04">EACS04: /healthz did not return 200 OK</a>

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

### <a name="EDB01">EDB01: Database Ping Failed</a>

**Problem:** This error code is returned if any attempt to execute this database
query fails.

**Solution:** Investigate the health of the database.

### <a name="EDB02">EDB02: Database Latency High</a>

**Problem:** This code is returned if the median latency is higher than the
[configured threshold](../cli/server.md#--health-check-threshold-database). This
may not be an error as such, but is an indication of a potential issue.

**Solution:** Investigate the sizing of the configured database with regard to
Coder's current activity and usage. It may be necessary to increase the
resources allocated to Coder's database. Alternatively, you can raise the
configured threshold to a higher value (this will not address the root cause).

> **Tip:**
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

### <a name="EDERP01">EDERP01: DERP Node Uses Websocket</a>

**Problem:** When Coder attempts to establish a connection to one or more DERP
servers, it sends a specific `Upgrade: derp` HTTP header. Some load balancers
may block this header, in which case Coder will fall back to
`Upgrade: websocket`.

This is not necessarily a fatal error, but a possible indication of a
misconfigured reverse HTTP proxy. Additionally, while workspace users should
still be able to reach their workspaces, connection performance may be degraded.

> **Note:** This may also be shown if you have
> [forced websocket connections for DERP](../cli/server.md#--derp-force-websockets).

**Solution:** ensure that any configured reverse proxy does not strip the
`Upgrade: derp` header.

### <a name="EDERP02">EDERP02: One or more DERP nodes are unhealthy</a>

**Problem:** This is shown if Coder is unable to reach one or more configured
DERP servers. Clients will fall back to use the remaining DERP servers, but
performance may be impacted for clients closest to the unhealthy DERP server.

**Solution:** Ensure that the DERP server is available and reachable over the
network on port 443, for example:

```shell
curl -v "https://coder.company.com:443/derp"
# Expected output:
# *   Trying XXX.XXX.XXX.XXX:443
# * Connected to https://coder.company.com (XXX.XXX.XXX.XXX) port 443 (#0)
# DERP requires connection upgrade
```

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

### <a name="EWS01">EWS01: Failed to establish a WebSocket connection</a>

**Problem:** Coder was unable to establish a WebSocket connection over its own
Access URL.

**Solution:** There are multiple possible causes of this problem:

1. Ensure that Coder's configured Access URL can be reached from the server
   running Coder, using standard troubleshooting tools like `curl`:

   ```shell
   curl -v "https://coder.company.com:443/"
   ```

2. Ensure that any reverse proxy that is sitting in front of Coder's configured
   access URL is not stripping the HTTP header `Upgrade: websocket`.

### <a name="EWS02">EWS02: Failed to echo a WebSocket message</a>

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

### <a name="EWP01">EWP01: Error Updating Workspace Proxy Health</a>

**Problem:** Coder was unable to query the connected workspace proxies for their
health status.

**Solution:** This may be a transient issue. If it persists, it could signify a
connectivity issue.

### <a name="EWP02">EWP02: Error Fetching Workspace Proxies</a>

**Problem:** Coder was unable to fetch the stored workspace proxy health data
from the database.

**Solution:** This may be a transient issue. If it persists, it could signify an
issue with Coder's configured database.

### <a name="EWP03">EWP03: Workspace Proxy Version Mismatch</a>

**Problem:** One or more workspace proxies are more than one major or minor
version out of date with the main deployment. It is important that workspace
proxies are updated at the same time as the main deployment to minimize the risk
of API incompatibility.

**Solution:** Update the workspace proxy to match the currently running version
of Coder.

### <a name="EWP04">EWP04: One or more Workspace Proxies Unhealthy</a>

**Problem:** One or more workspace proxies are not reachable.

**Solution:** Ensure that Coder can establish a connection to the configured
workspace proxies on port 443.

## <a name="EUNKNOWN">Unknown Error</a>

**Problem:** This error is shown when an unexpected error occurred evaluating
deployment health. It may resolve on its own.

**Solution:** This may be a bug.
[File a GitHub issue](https://github.com/coder/coder/issues/new)!
