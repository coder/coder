# Debug

## Debug Info Wireguard Coordinator

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/debug/coordinator \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /debug/coordinator`

### Responses

| Status | Meaning                                                 | Description | Schema |
| ------ | ------------------------------------------------------- | ----------- | ------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Debug Info Deployment Health

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/debug/health \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /debug/health`

### Parameters

| Name    | In    | Type    | Required | Description                |
| ------- | ----- | ------- | -------- | -------------------------- |
| `force` | query | boolean | false    | Force a healthcheck to run |

### Example responses

> 200 Response

```json
{
  "access_url": {
    "access_url": "string",
    "error": "string",
    "healthy": true,
    "healthz_response": "string",
    "reachable": true,
    "status_code": 0,
    "warnings": ["string"]
  },
  "coder_version": "string",
  "database": {
    "error": "string",
    "healthy": true,
    "latency": "string",
    "latency_ms": 0,
    "reachable": true,
    "threshold_ms": 0,
    "warnings": ["string"]
  },
  "derp": {
    "error": "string",
    "healthy": true,
    "netcheck": {
      "captivePortal": "string",
      "globalV4": "string",
      "globalV6": "string",
      "hairPinning": "string",
      "icmpv4": true,
      "ipv4": true,
      "ipv4CanSend": true,
      "ipv6": true,
      "ipv6CanSend": true,
      "mappingVariesByDestIP": "string",
      "oshasIPv6": true,
      "pcp": "string",
      "pmp": "string",
      "preferredDERP": 0,
      "regionLatency": {
        "property1": 0,
        "property2": 0
      },
      "regionV4Latency": {
        "property1": 0,
        "property2": 0
      },
      "regionV6Latency": {
        "property1": 0,
        "property2": 0
      },
      "udp": true,
      "upnP": "string"
    },
    "netcheck_err": "string",
    "netcheck_logs": ["string"],
    "regions": {
      "property1": {
        "error": "string",
        "healthy": true,
        "node_reports": [
          {
            "can_exchange_messages": true,
            "client_errs": [["string"]],
            "client_logs": [["string"]],
            "error": "string",
            "healthy": true,
            "node": {
              "canPort80": true,
              "certName": "string",
              "derpport": 0,
              "forceHTTP": true,
              "hostName": "string",
              "insecureForTests": true,
              "ipv4": "string",
              "ipv6": "string",
              "name": "string",
              "regionID": 0,
              "stunonly": true,
              "stunport": 0,
              "stuntestIP": "string"
            },
            "node_info": {
              "tokenBucketBytesBurst": 0,
              "tokenBucketBytesPerSecond": 0
            },
            "round_trip_ping": "string",
            "round_trip_ping_ms": 0,
            "stun": {
              "canSTUN": true,
              "enabled": true,
              "error": "string"
            },
            "uses_websocket": true,
            "warnings": ["string"]
          }
        ],
        "region": {
          "avoid": true,
          "embeddedRelay": true,
          "nodes": [
            {
              "canPort80": true,
              "certName": "string",
              "derpport": 0,
              "forceHTTP": true,
              "hostName": "string",
              "insecureForTests": true,
              "ipv4": "string",
              "ipv6": "string",
              "name": "string",
              "regionID": 0,
              "stunonly": true,
              "stunport": 0,
              "stuntestIP": "string"
            }
          ],
          "regionCode": "string",
          "regionID": 0,
          "regionName": "string"
        },
        "warnings": ["string"]
      },
      "property2": {
        "error": "string",
        "healthy": true,
        "node_reports": [
          {
            "can_exchange_messages": true,
            "client_errs": [["string"]],
            "client_logs": [["string"]],
            "error": "string",
            "healthy": true,
            "node": {
              "canPort80": true,
              "certName": "string",
              "derpport": 0,
              "forceHTTP": true,
              "hostName": "string",
              "insecureForTests": true,
              "ipv4": "string",
              "ipv6": "string",
              "name": "string",
              "regionID": 0,
              "stunonly": true,
              "stunport": 0,
              "stuntestIP": "string"
            },
            "node_info": {
              "tokenBucketBytesBurst": 0,
              "tokenBucketBytesPerSecond": 0
            },
            "round_trip_ping": "string",
            "round_trip_ping_ms": 0,
            "stun": {
              "canSTUN": true,
              "enabled": true,
              "error": "string"
            },
            "uses_websocket": true,
            "warnings": ["string"]
          }
        ],
        "region": {
          "avoid": true,
          "embeddedRelay": true,
          "nodes": [
            {
              "canPort80": true,
              "certName": "string",
              "derpport": 0,
              "forceHTTP": true,
              "hostName": "string",
              "insecureForTests": true,
              "ipv4": "string",
              "ipv6": "string",
              "name": "string",
              "regionID": 0,
              "stunonly": true,
              "stunport": 0,
              "stuntestIP": "string"
            }
          ],
          "regionCode": "string",
          "regionID": 0,
          "regionName": "string"
        },
        "warnings": ["string"]
      }
    },
    "warnings": ["string"]
  },
  "failing_sections": ["string"],
  "healthy": true,
  "time": "string",
  "websocket": {
    "body": "string",
    "code": 0,
    "error": "string",
    "healthy": true,
    "warnings": ["string"]
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                             |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [healthcheck.Report](schemas.md#healthcheckreport) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Debug Info Tailnet

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/debug/tailnet \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /debug/tailnet`

### Responses

| Status | Meaning                                                 | Description | Schema |
| ------ | ------------------------------------------------------- | ----------- | ------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
