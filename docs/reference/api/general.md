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
|--------|---------------------------------------------------------|-------------|--------------------------------------------------|
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
  "agent_api_version": "string",
  "dashboard_url": "string",
  "deployment_id": "string",
  "external_url": "string",
  "provisioner_api_version": "string",
  "telemetry": true,
  "upgrade_message": "string",
  "version": "string",
  "webpush_public_key": "string",
  "workspace_proxy": true
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                             |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------|
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
|--------|------|------------------------------------------------------|----------|------------------|
| `body` | body | [coderd.cspViolation](schemas.md#coderdcspviolation) | true     | Violation report |

### Responses

| Status | Meaning                                                 | Description | Schema |
|--------|---------------------------------------------------------|-------------|--------|
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
    "additional_csp_policy": [
      "string"
    ],
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
    "allow_workspace_renames": true,
    "autobuild_poll_interval": 0,
    "browser_only": true,
    "cache_directory": "string",
    "cli_upgrade_message": "string",
    "config": "string",
    "config_ssh": {
      "deploymentName": "string",
      "sshconfigOptions": [
        "string"
      ]
    },
    "dangerous": {
      "allow_all_cors": true,
      "allow_path_app_sharing": true,
      "allow_path_app_site_owner_access": true
    },
    "derp": {
      "config": {
        "block_direct": true,
        "force_websockets": true,
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
        "stun_addresses": [
          "string"
        ]
      }
    },
    "disable_owner_workspace_exec": true,
    "disable_password_auth": true,
    "disable_path_apps": true,
    "docs_url": {
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
    "enable_terraform_debug_mode": true,
    "ephemeral_deployment": true,
    "experiments": [
      "string"
    ],
    "external_auth": {
      "value": [
        {
          "app_install_url": "string",
          "app_installations_url": "string",
          "auth_url": "string",
          "client_id": "string",
          "device_code_url": "string",
          "device_flow": true,
          "display_icon": "string",
          "display_name": "string",
          "id": "string",
          "no_refresh": true,
          "regex": "string",
          "scopes": [
            "string"
          ],
          "token_url": "string",
          "type": "string",
          "validate_url": "string"
        }
      ]
    },
    "external_token_encryption_keys": [
      "string"
    ],
    "healthcheck": {
      "refresh": 0,
      "threshold_database": 0
    },
    "http_address": "string",
    "in_memory_database": true,
    "job_hang_detector_interval": 0,
    "logging": {
      "human": "string",
      "json": "string",
      "log_filter": [
        "string"
      ],
      "stackdriver": "string"
    },
    "metrics_cache_refresh_interval": 0,
    "notifications": {
      "dispatch_timeout": 0,
      "email": {
        "auth": {
          "identity": "string",
          "password": "string",
          "password_file": "string",
          "username": "string"
        },
        "force_tls": true,
        "from": "string",
        "hello": "string",
        "smarthost": "string",
        "tls": {
          "ca_file": "string",
          "cert_file": "string",
          "insecure_skip_verify": true,
          "key_file": "string",
          "server_name": "string",
          "start_tls": true
        }
      },
      "fetch_interval": 0,
      "inbox": {
        "enabled": true
      },
      "lease_count": 0,
      "lease_period": 0,
      "max_send_attempts": 0,
      "method": "string",
      "retry_interval": 0,
      "sync_buffer_size": 0,
      "sync_interval": 0,
      "webhook": {
        "endpoint": {
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
      }
    },
    "oauth2": {
      "github": {
        "allow_everyone": true,
        "allow_signups": true,
        "allowed_orgs": [
          "string"
        ],
        "allowed_teams": [
          "string"
        ],
        "client_id": "string",
        "client_secret": "string",
        "default_provider_enable": true,
        "device_flow": true,
        "enterprise_base_url": "string"
      }
    },
    "oidc": {
      "allow_signups": true,
      "auth_url_params": {},
      "client_cert_file": "string",
      "client_id": "string",
      "client_key_file": "string",
      "client_secret": "string",
      "email_domain": [
        "string"
      ],
      "email_field": "string",
      "group_allow_list": [
        "string"
      ],
      "group_auto_create": true,
      "group_mapping": {},
      "group_regex_filter": {},
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
      "ignore_user_info": true,
      "issuer_url": "string",
      "name_field": "string",
      "organization_assign_default": true,
      "organization_field": "string",
      "organization_mapping": {},
      "scopes": [
        "string"
      ],
      "sign_in_text": "string",
      "signups_disabled_text": "string",
      "skip_issuer_checks": true,
      "source_user_info_from_access_token": true,
      "user_role_field": "string",
      "user_role_mapping": {},
      "user_roles_default": [
        "string"
      ],
      "username_field": "string"
    },
    "pg_auth": "string",
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
      "aggregate_agent_stats_by": [
        "string"
      ],
      "collect_agent_stats": true,
      "collect_db_metrics": true,
      "enable": true
    },
    "provisioner": {
      "daemon_poll_interval": 0,
      "daemon_poll_jitter": 0,
      "daemon_psk": "string",
      "daemon_types": [
        "string"
      ],
      "daemons": 0,
      "force_cancel_interval": 0
    },
    "proxy_health_status_interval": 0,
    "proxy_trusted_headers": [
      "string"
    ],
    "proxy_trusted_origins": [
      "string"
    ],
    "rate_limit": {
      "api": 0,
      "disable_all": true
    },
    "redirect_to_access_url": true,
    "scim_api_key": "string",
    "secure_auth_cookie": true,
    "session_lifetime": {
      "default_duration": 0,
      "default_token_lifetime": 0,
      "disable_expiry_refresh": true,
      "max_token_lifetime": 0
    },
    "ssh_keygen_algorithm": "string",
    "strict_transport_security": 0,
    "strict_transport_security_options": [
      "string"
    ],
    "support": {
      "links": {
        "value": [
          {
            "icon": "bug",
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
    "terms_of_service_url": "string",
    "tls": {
      "address": {
        "host": "string",
        "port": "string"
      },
      "allow_insecure_ciphers": true,
      "cert_file": [
        "string"
      ],
      "client_auth": "string",
      "client_ca_file": "string",
      "client_cert_file": "string",
      "client_key_file": "string",
      "enable": true,
      "key_file": [
        "string"
      ],
      "min_version": "string",
      "redirect_http": true,
      "supported_ciphers": [
        "string"
      ]
    },
    "trace": {
      "capture_logs": true,
      "data_dog": true,
      "enable": true,
      "honeycomb_api_key": "string"
    },
    "update_check": true,
    "user_quiet_hours_schedule": {
      "allow_user_custom": true,
      "default_schedule": "string"
    },
    "verbose": true,
    "web_terminal_renderer": "string",
    "wgtunnel_host": "string",
    "wildcard_access_url": "string",
    "workspace_hostname_suffix": "string",
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
        "description": "string",
        "name": "string",
        "parent": {
          "description": "string",
          "name": "string",
          "parent": {},
          "yaml": "string"
        },
        "yaml": "string"
      },
      "hidden": true,
      "name": "string",
      "required": true,
      "use_instead": [
        {}
      ],
      "value": null,
      "value_source": "",
      "yaml": "string"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                           |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------|
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
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------|
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
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.DeploymentStats](schemas.md#codersdkdeploymentstats) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get enabled experiments

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
[
  "example"
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                        |
|--------|---------------------------------------------------------|-------------|---------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Experiment](schemas.md#codersdkexperiment) |

<h3 id="get-enabled-experiments-responseschema">Response Schema</h3>

Status Code **200**

| Name           | Type  | Required | Restrictions | Description |
|----------------|-------|----------|--------------|-------------|
| `[array item]` | array | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get safe experiments

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/experiments/available \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /experiments/available`

### Example responses

> 200 Response

```json
[
  "example"
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                        |
|--------|---------------------------------------------------------|-------------|---------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Experiment](schemas.md#codersdkexperiment) |

<h3 id="get-safe-experiments-responseschema">Response Schema</h3>

Status Code **200**

| Name           | Type  | Required | Restrictions | Description |
|----------------|-------|----------|--------------|-------------|
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
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------|
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
|--------|------|--------|----------|----------------------|
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
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.TokenConfig](schemas.md#codersdktokenconfig) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
