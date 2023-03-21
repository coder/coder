# General

## API root handler

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/ \
  -H 'Accept: application/json'
```

`GET /`

### Example responses

> 200 Response

```json
{
  "detail": "string",
  "message": "string",
  "validations": [
    {
      "detail": "string",
      "field": "string"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                           |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Response](schemas.md#codersdkresponse) |

## Build info

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/buildinfo \
  -H 'Accept: application/json'
```

`GET /buildinfo`

### Example responses

> 200 Response

```json
{
  "external_url": "string",
  "version": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                             |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.BuildInfoResponse](schemas.md#codersdkbuildinforesponse) |

## Report CSP violations

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/csp/reports \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /csp/reports`

> Body parameter

```json
{
  "csp-report": {}
}
```

### Parameters

| Name   | In   | Type                                                 | Required | Description      |
| ------ | ---- | ---------------------------------------------------- | -------- | ---------------- |
| `body` | body | [coderd.cspViolation](schemas.md#coderdcspviolation) | true     | Violation report |

### Responses

| Status | Meaning                                                 | Description | Schema |
| ------ | ------------------------------------------------------- | ----------- | ------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get deployment config

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/deployment/config \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /deployment/config`

### Example responses

> 200 Response

```json
{
  "config": {
    "access_url": {
      "forceQuery": true,
      "fragment": "string",
      "host": "string",
      "omitHost": true,
      "opaque": "string",
      "path": "string",
      "rawFragment": "string",
      "rawPath": "string",
      "rawQuery": "string",
      "scheme": "string",
      "user": {}
    },
    "address": {
      "host": "string",
      "port": "string"
    },
    "agent_fallback_troubleshooting_url": {
      "forceQuery": true,
      "fragment": "string",
      "host": "string",
      "omitHost": true,
      "opaque": "string",
      "path": "string",
      "rawFragment": "string",
      "rawPath": "string",
      "rawQuery": "string",
      "scheme": "string",
      "user": {}
    },
    "agent_stat_refresh_interval": 0,
    "audit_logging": true,
    "autobuild_poll_interval": 0,
    "browser_only": true,
    "cache_directory": "string",
    "config": "string",
    "config_ssh": {
      "deploymentName": "string",
      "sshconfigOptions": ["string"]
    },
    "dangerous": {
      "allow_path_app_sharing": true,
      "allow_path_app_site_owner_access": true
    },
    "derp": {
      "config": {
        "path": "string",
        "url": "string"
      },
      "server": {
        "enable": true,
        "region_code": "string",
        "region_id": 0,
        "region_name": "string",
        "relay_url": {
          "forceQuery": true,
          "fragment": "string",
          "host": "string",
          "omitHost": true,
          "opaque": "string",
          "path": "string",
          "rawFragment": "string",
          "rawPath": "string",
          "rawQuery": "string",
          "scheme": "string",
          "user": {}
        },
        "stun_addresses": ["string"]
      }
    },
    "disable_password_auth": true,
    "disable_path_apps": true,
    "disable_session_expiry_refresh": true,
    "experiments": ["string"],
    "git_auth": {
      "value": [
        {
          "auth_url": "string",
          "client_id": "string",
          "id": "string",
          "no_refresh": true,
          "regex": "string",
          "scopes": ["string"],
          "token_url": "string",
          "type": "string",
          "validate_url": "string"
        }
      ]
    },
    "http_address": "string",
    "in_memory_database": true,
    "logging": {
      "human": "string",
      "json": "string",
      "stackdriver": "string"
    },
    "max_session_expiry": 0,
    "max_token_lifetime": 0,
    "metrics_cache_refresh_interval": 0,
    "oauth2": {
      "github": {
        "allow_everyone": true,
        "allow_signups": true,
        "allowed_orgs": ["string"],
        "allowed_teams": ["string"],
        "client_id": "string",
        "client_secret": "string",
        "enterprise_base_url": "string"
      }
    },
    "oidc": {
      "allow_signups": true,
      "client_id": "string",
      "client_secret": "string",
      "email_domain": ["string"],
      "group_mapping": {},
      "groups_field": "string",
      "icon_url": {
        "forceQuery": true,
        "fragment": "string",
        "host": "string",
        "omitHost": true,
        "opaque": "string",
        "path": "string",
        "rawFragment": "string",
        "rawPath": "string",
        "rawQuery": "string",
        "scheme": "string",
        "user": {}
      },
      "ignore_email_verified": true,
      "issuer_url": "string",
      "scopes": ["string"],
      "sign_in_text": "string",
      "username_field": "string"
    },
    "pg_connection_url": "string",
    "pprof": {
      "address": {
        "host": "string",
        "port": "string"
      },
      "enable": true
    },
    "prometheus": {
      "address": {
        "host": "string",
        "port": "string"
      },
      "enable": true
    },
    "provisioner": {
      "daemon_poll_interval": 0,
      "daemon_poll_jitter": 0,
      "daemons": 0,
      "force_cancel_interval": 0
    },
    "proxy_trusted_headers": ["string"],
    "proxy_trusted_origins": ["string"],
    "rate_limit": {
      "api": 0,
      "disable_all": true
    },
    "redirect_to_access_url": true,
    "scim_api_key": "string",
    "secure_auth_cookie": true,
    "ssh_keygen_algorithm": "string",
    "strict_transport_security": 0,
    "strict_transport_security_options": ["string"],
    "support": {
      "links": {
        "value": [
          {
            "icon": "string",
            "name": "string",
            "target": "string"
          }
        ]
      }
    },
    "swagger": {
      "enable": true
    },
    "telemetry": {
      "enable": true,
      "trace": true,
      "url": {
        "forceQuery": true,
        "fragment": "string",
        "host": "string",
        "omitHost": true,
        "opaque": "string",
        "path": "string",
        "rawFragment": "string",
        "rawPath": "string",
        "rawQuery": "string",
        "scheme": "string",
        "user": {}
      }
    },
    "tls": {
      "address": {
        "host": "string",
        "port": "string"
      },
      "cert_file": ["string"],
      "client_auth": "string",
      "client_ca_file": "string",
      "client_cert_file": "string",
      "client_key_file": "string",
      "enable": true,
      "key_file": ["string"],
      "min_version": "string",
      "redirect_http": true
    },
    "trace": {
      "capture_logs": true,
      "enable": true,
      "honeycomb_api_key": "string"
    },
    "update_check": true,
    "verbose": true,
    "wildcard_access_url": {
      "forceQuery": true,
      "fragment": "string",
      "host": "string",
      "omitHost": true,
      "opaque": "string",
      "path": "string",
      "rawFragment": "string",
      "rawPath": "string",
      "rawQuery": "string",
      "scheme": "string",
      "user": {}
    },
    "write_config": true
  },
  "options": [
    {
      "annotations": {
        "property1": "string",
        "property2": "string"
      },
      "default": "string",
      "description": "string",
      "env": "string",
      "flag": "string",
      "flag_shorthand": "string",
      "group": {
        "children": [
          {
            "children": [],
            "description": "string",
            "name": "string",
            "parent": {}
          }
        ],
        "description": "string",
        "name": "string",
        "parent": {
          "children": [{}],
          "description": "string",
          "name": "string",
          "parent": {}
        }
      },
      "hidden": true,
      "name": "string",
      "use_instead": [{}],
      "value": null,
      "yaml": "string"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                           |
| ------ | ------------------------------------------------------- | ----------- | ---------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.DeploymentConfig](schemas.md#codersdkdeploymentconfig) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## SSH Config

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/deployment/ssh \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /deployment/ssh`

### Example responses

> 200 Response

```json
{
  "hostname_prefix": "string",
  "ssh_config_options": {
    "property1": "string",
    "property2": "string"
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                             |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.SSHConfigResponse](schemas.md#codersdksshconfigresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get deployment stats

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/deployment/stats \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /deployment/stats`

### Example responses

> 200 Response

```json
{
  "aggregated_from": "2019-08-24T14:15:22Z",
  "collected_at": "2019-08-24T14:15:22Z",
  "next_update_at": "2019-08-24T14:15:22Z",
  "session_count": {
    "jetbrains": 0,
    "reconnecting_pty": 0,
    "ssh": 0,
    "vscode": 0
  },
  "workspaces": {
    "building": 0,
    "connection_latency_ms": {
      "p50": 0,
      "p95": 0
    },
    "failed": 0,
    "pending": 0,
    "running": 0,
    "rx_bytes": 0,
    "stopped": 0,
    "tx_bytes": 0
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                         |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.DeploymentStats](schemas.md#codersdkdeploymentstats) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get experiments

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/experiments \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /experiments`

### Example responses

> 200 Response

```json
["template_editor"]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                        |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Experiment](schemas.md#codersdkexperiment) |

<h3 id="get-experiments-responseschema">Response Schema</h3>

Status Code **200**

| Name           | Type  | Required | Restrictions | Description |
| -------------- | ----- | -------- | ------------ | ----------- |
| `[array item]` | array | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update check

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/updatecheck \
  -H 'Accept: application/json'
```

`GET /updatecheck`

### Example responses

> 200 Response

```json
{
  "current": true,
  "url": "string",
  "version": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                 |
| ------ | ------------------------------------------------------- | ----------- | ---------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.UpdateCheckResponse](schemas.md#codersdkupdatecheckresponse) |

## Get token config

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/{user}/keys/tokens/tokenconfig \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/{user}/keys/tokens/tokenconfig`

### Parameters

| Name   | In   | Type   | Required | Description          |
| ------ | ---- | ------ | -------- | -------------------- |
| `user` | path | string | true     | User ID, name, or me |

### Example responses

> 200 Response

```json
{
  "max_token_lifetime": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                 |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.TokenConfig](schemas.md#codersdktokenconfig) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
