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
    "dismissed": true,
    "error": "string",
    "healthy": true,
    "healthz_response": "string",
    "reachable": true,
    "severity": "ok",
    "status_code": 0,
    "warnings": [
      {
        "code": "EUNKNOWN",
        "message": "string"
      }
    ]
  },
  "coder_version": "string",
  "database": {
    "dismissed": true,
    "error": "string",
    "healthy": true,
    "latency": "string",
    "latency_ms": 0,
    "reachable": true,
    "severity": "ok",
    "threshold_ms": 0,
    "warnings": [
      {
        "code": "EUNKNOWN",
        "message": "string"
      }
    ]
  },
  "derp": {
    "dismissed": true,
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
            "severity": "ok",
            "stun": {
              "canSTUN": true,
              "enabled": true,
              "error": "string"
            },
            "uses_websocket": true,
            "warnings": [
              {
                "code": "EUNKNOWN",
                "message": "string"
              }
            ]
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
        "severity": "ok",
        "warnings": [
          {
            "code": "EUNKNOWN",
            "message": "string"
          }
        ]
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
            "severity": "ok",
            "stun": {
              "canSTUN": true,
              "enabled": true,
              "error": "string"
            },
            "uses_websocket": true,
            "warnings": [
              {
                "code": "EUNKNOWN",
                "message": "string"
              }
            ]
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
        "severity": "ok",
        "warnings": [
          {
            "code": "EUNKNOWN",
            "message": "string"
          }
        ]
      }
    },
    "severity": "ok",
    "warnings": [
      {
        "code": "EUNKNOWN",
        "message": "string"
      }
    ]
  },
  "failing_sections": ["DERP"],
  "healthy": true,
  "provisioner_daemons": {
    "dismissed": true,
    "error": "string",
    "provisioner_daemons": [
      {
        "api_version": "string",
        "created_at": "2019-08-24T14:15:22Z",
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "last_seen_at": "2019-08-24T14:15:22Z",
        "name": "string",
        "provisioners": ["string"],
        "tags": {
          "property1": "string",
          "property2": "string"
        },
        "version": "string"
      }
    ],
    "severity": "ok",
    "warnings": [
      {
        "code": "EUNKNOWN",
        "message": "string"
      }
    ]
  },
  "severity": "ok",
  "time": "string",
  "websocket": {
    "body": "string",
    "code": 0,
    "dismissed": true,
    "error": "string",
    "healthy": true,
    "severity": "ok",
    "warnings": ["string"]
  },
  "workspace_proxy": {
    "dismissed": true,
    "error": "string",
    "healthy": true,
    "severity": "ok",
    "warnings": [
      {
        "code": "EUNKNOWN",
        "message": "string"
      }
    ],
    "workspace_proxies": {
      "regions": [
        {
          "created_at": "2019-08-24T14:15:22Z",
          "deleted": true,
          "derp_enabled": true,
          "derp_only": true,
          "display_name": "string",
          "healthy": true,
          "icon_url": "string",
          "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
          "name": "string",
          "path_app_url": "string",
          "status": {
            "checked_at": "2019-08-24T14:15:22Z",
            "report": {
              "errors": ["string"],
              "warnings": ["string"]
            },
            "status": "ok"
          },
          "updated_at": "2019-08-24T14:15:22Z",
          "version": "string",
          "wildcard_hostname": "string"
        }
      ]
    }
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                             |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [healthcheck.Report](schemas.md#healthcheckreport) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get health settings

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/debug/health/settings \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /debug/health/settings`

### Example responses

> 200 Response

```json
{
  "dismissed_healthchecks": ["DERP"]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.HealthSettings](schemas.md#codersdkhealthsettings) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update health settings

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/debug/health/settings \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /debug/health/settings`

> Body parameter

```json
{
  "dismissed_healthchecks": ["DERP"]
}
```

### Parameters

| Name   | In   | Type                                                                     | Required | Description            |
| ------ | ---- | ------------------------------------------------------------------------ | -------- | ---------------------- |
| `body` | body | [codersdk.UpdateHealthSettings](schemas.md#codersdkupdatehealthsettings) | true     | Update health settings |

### Example responses

> 200 Response

```json
{
  "dismissed_healthchecks": ["DERP"]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                   |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.UpdateHealthSettings](schemas.md#codersdkupdatehealthsettings) |

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
