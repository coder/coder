# Debug

## Get debug logs

### Code samples

```shell
curl $CODER_AGENT_DEBUG_ADDRESS/debug/logs
```

`GET /debug/logs`

Get the first 10MiB of data from `$CODER_AGENT_LOG_DIR/coder-agent.log`.

### Responses

| Status | Meaning                                                 | Description | Schema |
|--------|---------------------------------------------------------|-------------|--------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

## Get debug info for magicsock

### Code samples

```shell
curl $CODER_AGENT_DEBUG_ADDRESS/debug/magicsock
```

`GET /debug/magicsock`

See
[Tailscale's documentation](https://pkg.go.dev/tailscale.com/wgengine/magicsock#Conn.ServeHTTPDebug).

## Toggle debug logging for magicsock

### Code samples

```shell
curl $CODER_AGENT_DEBUG_ADDRESS/debug/magicsock/debug-logging/true
```

`GET /debug/magicsock/debug-logging/{state}`

Set whether debug logging is enabled. See
[Tailscale's documentation](https://pkg.go.dev/tailscale.com/wgengine/magicsock#Conn.SetDebugLoggingEnabled)
for more information.

### Parameters

| Name    | In   | Type    | Required | Description         |
|---------|------|---------|----------|---------------------|
| `state` | path | boolean | true     | Debug logging state |

### Responses

| Status | Meaning                                                 | Description | Schema |
|--------|---------------------------------------------------------|-------------|--------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

## Get debug manifest

### Code samples

```shell
curl $CODER_AGENT_DEBUG_ADDRESS/debug/manifest
```

`GET /debug/manifest`

Get the manifest the agent fetched from `coderd` upon startup.

### Responses

| Status | Meaning                                                 | Description | Schema                                             |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [agentsdk.Manifest](./schemas.md#agentsdkmanifest) |
