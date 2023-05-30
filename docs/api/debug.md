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

### Example responses

> 200 Response

```json
{
  "access_url": {
    "error": null,
    "healthy": true,
    "healthzResponse": "string",
    "reachable": true,
    "statusCode": 0
  },
  "derp": {
    "error": null,
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
    "netcheck_err": null,
    "netcheck_logs": ["string"],
    "regions": {
      "property1": {
        "error": null,
        "healthy": true,
        "node_reports": [
          {
            "can_exchange_messages": true,
            "client_errs": [[null]],
            "client_logs": [["string"]],
            "error": null,
            "healthy": true,
            "node": {
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
            "round_trip_ping": 0,
            "stun": {
              "canSTUN": true,
              "enabled": true,
              "error": null
            },
            "uses_websocket": true
          }
        ],
        "region": {
          "avoid": true,
          "embeddedRelay": true,
          "nodes": [
            {
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
        }
      },
      "property2": {
        "error": null,
        "healthy": true,
        "node_reports": [
          {
            "can_exchange_messages": true,
            "client_errs": [[null]],
            "client_logs": [["string"]],
            "error": null,
            "healthy": true,
            "node": {
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
            "round_trip_ping": 0,
            "stun": {
              "canSTUN": true,
              "enabled": true,
              "error": null
            },
            "uses_websocket": true
          }
        ],
        "region": {
          "avoid": true,
          "embeddedRelay": true,
          "nodes": [
            {
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
        }
      }
    }
  },
  "pass": true,
  "time": "string",
  "websocket": {
    "error": null,
    "response": {
      "body": "string",
      "code": 0
    }
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                             |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [healthcheck.Report](schemas.md#healthcheckreport) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
