# Debug

## Get debug logs

### Code samples

```shell
curl http://127.0.0.1:2113/debug/logs
```

`GET /debug/logs`

### Responses

| Status | Meaning                                                 | Description | Schema |
| ------ | ------------------------------------------------------- | ----------- | ------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

## Get debug info for magicsock

### Code samples

```shell
curl http://127.0.0.1:2113/debug/magicsock
```

`GET /debug/magicsock`

### Responses

See
[Tailscale's documentation](https://pkg.go.dev/tailscale.com/wgengine/magicsock#Conn.ServeHTTPDebug)
for response format.

## Toggle debug logging for magicsock

### Code samples

```shell
curl http://127.0.0.1:2113/debug/magicsock/debug-logging/true
```

`GET /debug/magicsock/debug-logging/{state}`

### Parameters

| Name    | In   | Type    | Required | Description         |
| ------- | ---- | ------- | -------- | ------------------- |
| `state` | path | boolean | true     | Debug logging state |

### Responses

| Status | Meaning                                                 | Description | Schema |
| ------ | ------------------------------------------------------- | ----------- | ------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

## Get debug manifest

### Code samples

```shell
curl http://127.0.0.1:2113/debug/manifest
```

`GET /debug/manifest`

### Responses

| Status | Meaning                                                 | Description | Schema                                          |
| ------ | ------------------------------------------------------- | ----------- | ----------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [agentsdk.Manifest](./schemas#agentsdkmanifest) |
