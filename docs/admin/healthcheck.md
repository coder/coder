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
validate that the response is `200 OK`.

If there is an issue, you may see one of the following errors reported:

### <a name="EACS01">EACS01: Access URL not set</a>

**Problem:** no access URL has been configured.

**Solution:** configure an [access URL](./configure.md#access-url) for Coder.

### <a name="EACS02">EACS02: Access URL invalid</a>

**Problem:** `${CODER_ACCESS_URL}/healthz` is not a valid URL.

**Solution:** Ensure that the access URL is a valid URL accepted by
[`url.Parse`](https://pkg.go.dev/net/url#Parse).

### <a name="EACS03">EACS03: Failed to fetch /healthz</a>

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
curl -v ${CODER_ACCESS_URL}
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

Coder continuously executes a short database query to validate that it can reach its configured database, and also
measures the median latency over 5 attempts.

### <a name="EDB01">EDB01: Database Ping Failed</a>

**Problem:** This error code is returned if any attempt to execute this database query fails.

**Solution:** Investigate the health of the database.

### <a name="EDB02">EDB02: Database Ping High</a>

**Problem:** This code is returned if the median latency is higher than the [configured threshold](../cli/server.md#--health-check-threshold-database).
This may not be an error as such, but is an indication of a potential issue.

**Solution:** Investigate the sizing of the configured database with regard to Coder's current activity and usage. It
may be necessary to increase the resources allocated to Coder's database. Alternatively, you can raise the configured
threshold to a higher value (this will not address the root cause).

> [!TIP]
> - You can enable [detailed database metrics](../cli/server.md#--prometheus-collect-db-metrics) in Coder's
> Prometheus endpoint.
> - Fif you have [tracing enabled](../cli/server.md#--trace), these traces may also contain useful information regarding
> Coder's database activity.

## DERP

### <a name="EDERP01">EDERP01: TODO</a>

TODO

### <a name="EDERP02">EDERP02: TODO</a>

TODO

## Websocket

### <a name="EWS01">EWS01: TODO</a>

TODO

### <a name="EWS02">EWS02: TODO</a>

TODO

## Workspace Proxy

### <a name="EWP01">EWP01: TODO</a>

TODO

### <a name="EWP02">EWP02: TODO</a>

TODO

### <a name="EWP03">EWP03: TODO</a>

TODO

### <a name="EWP04">EWP04: TODO</a>

TODO

## <a name="EUNKNOWN">Unknown Error</a>

**Problem:** lazy dev

**Solution:** motivate them with cheese
