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

## Get deployment config

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/config/deployment \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /config/deployment`

### Example responses

> 200 Response

```json
{
  "access_url": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "address": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "agent_fallback_troubleshooting_url": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "agent_stat_refresh_interval": {
    "default": 0,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": 0
  },
  "audit_logging": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "autobuild_poll_interval": {
    "default": 0,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": 0
  },
  "browser_only": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "cache_directory": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "dangerous": {
    "allow_path_app_sharing": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    },
    "allow_path_app_site_owner_access": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    }
  },
  "derp": {
    "config": {
      "path": {
        "default": "string",
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": "string"
      },
      "url": {
        "default": "string",
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": "string"
      }
    },
    "server": {
      "enable": {
        "default": true,
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": true
      },
      "region_code": {
        "default": "string",
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": "string"
      },
      "region_id": {
        "default": 0,
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": 0
      },
      "region_name": {
        "default": "string",
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": "string"
      },
      "relay_url": {
        "default": "string",
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": "string"
      },
      "stun_addresses": {
        "default": ["string"],
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": ["string"]
      }
    }
  },
  "disable_password_auth": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "disable_path_apps": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "disable_session_expiry_refresh": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "experimental": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "experiments": {
    "default": ["string"],
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": ["string"]
  },
  "gitauth": {
    "default": [
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
    ],
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
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
  "http_address": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "in_memory_database": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "logging": {
    "human": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "json": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "stackdriver": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    }
  },
  "max_session_expiry": {
    "default": 0,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": 0
  },
  "max_token_lifetime": {
    "default": 0,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": 0
  },
  "metrics_cache_refresh_interval": {
    "default": 0,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": 0
  },
  "oauth2": {
    "github": {
      "allow_everyone": {
        "default": true,
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": true
      },
      "allow_signups": {
        "default": true,
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": true
      },
      "allowed_orgs": {
        "default": ["string"],
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": ["string"]
      },
      "allowed_teams": {
        "default": ["string"],
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": ["string"]
      },
      "client_id": {
        "default": "string",
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": "string"
      },
      "client_secret": {
        "default": "string",
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": "string"
      },
      "enterprise_base_url": {
        "default": "string",
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": "string"
      }
    }
  },
  "oidc": {
    "allow_signups": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    },
    "client_id": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "client_secret": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "email_domain": {
      "default": ["string"],
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": ["string"]
    },
    "icon_url": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "ignore_email_verified": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    },
    "issuer_url": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "scopes": {
      "default": ["string"],
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": ["string"]
    },
    "sign_in_text": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "username_field": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    }
  },
  "pg_connection_url": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "pprof": {
    "address": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "enable": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    }
  },
  "prometheus": {
    "address": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "enable": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    }
  },
  "provisioner": {
    "daemon_poll_interval": {
      "default": 0,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": 0
    },
    "daemon_poll_jitter": {
      "default": 0,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": 0
    },
    "daemons": {
      "default": 0,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": 0
    },
    "force_cancel_interval": {
      "default": 0,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": 0
    }
  },
  "proxy_trusted_headers": {
    "default": ["string"],
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": ["string"]
  },
  "proxy_trusted_origins": {
    "default": ["string"],
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": ["string"]
  },
  "rate_limit": {
    "api": {
      "default": 0,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": 0
    },
    "disable_all": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    }
  },
  "redirect_to_access_url": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "scim_api_key": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "secure_auth_cookie": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "ssh_keygen_algorithm": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "strict_transport_security": {
    "default": 0,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": 0
  },
  "strict_transport_security_options": {
    "default": ["string"],
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": ["string"]
  },
  "support": {
    "links": {
      "default": [
        {
          "icon": "string",
          "name": "string",
          "target": "string"
        }
      ],
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
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
    "enable": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    }
  },
  "telemetry": {
    "enable": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    },
    "trace": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    },
    "url": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    }
  },
  "tls": {
    "address": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "cert_file": {
      "default": ["string"],
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": ["string"]
    },
    "client_auth": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "client_ca_file": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "client_cert_file": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "client_key_file": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "enable": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    },
    "key_file": {
      "default": ["string"],
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": ["string"]
    },
    "min_version": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "redirect_http": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    }
  },
  "trace": {
    "capture_logs": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    },
    "enable": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    },
    "honeycomb_api_key": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    }
  },
  "update_check": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "wildcard_access_url": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                           |
| ------ | ------------------------------------------------------- | ----------- | ---------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.DeploymentConfig](schemas.md#codersdkdeploymentconfig) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

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
["authz_querier"]
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
