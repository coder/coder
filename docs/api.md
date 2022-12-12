<h1 id="coderd-api">Coderd API v2.0</h1>

> XScroll down for code samples, example requests and responses. Select a language for code samples from the tabs above or the mobile navigation menu.

Coderd is the service created by running coder server. It is a thin API that connects workspaces, provisioners and users. coderd stores its state in Postgres and is the only service that communicates with Postgres.

Base URLs:

- <a href="/api/v2">/api/v2</a>

<a href="https://coder.com/legal/terms-of-service">Terms of service</a>
Email: <a href="mailto:support@coder.com">API Support</a> Web: <a href="http://coder.com">API Support</a>
License: <a href="https://github.com/coder/coder/blob/main/LICENSE">AGPL-3.0</a>

# Authentication

- API Key (CoderSessionToken)
  - Parameter Name: **Coder-Session-Token**, in: header.

<h1 id="coderd-api-workspaces">Workspaces</h1>

## List workspaces

<a id="opIdget-workspaces"></a>

> Code samples

```shell
# You can also use wget
curl -X GET /api/v2/workspaces \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'

```

`GET /workspaces`

<h3 id="list-workspaces-parameters">Parameters</h3>

| Name      | In    | Type    | Required | Description        |
| --------- | ----- | ------- | -------- | ------------------ |
| owner     | query | string  | false    | Owner username     |
| template  | query | string  | false    | Template name      |
| name      | query | string  | false    | Workspace name     |
| status    | query | string  | false    | Workspace status   |
| deleted   | query | boolean | false    | Deleted workspaces |
| has_agent | query | boolean | false    | Has agent          |

> Example responses

> 200 Response

```json
{
  "count": 0,
  "workspaces": [
    {
      "autostart_schedule": "string",
      "created_at": "string",
      "id": "string",
      "last_used_at": "string",
      "latest_build": {
        "build_number": 0,
        "created_at": "string",
        "daily_cost": 0,
        "deadline": {
          "time": "string",
          "valid": true
        },
        "id": "string",
        "initiator_id": "string",
        "initiator_name": "string",
        "job": {
          "canceled_at": "string",
          "completed_at": "string",
          "created_at": "string",
          "error": "string",
          "file_id": "string",
          "id": "string",
          "started_at": "string",
          "status": "string",
          "tags": {
            "property1": "string",
            "property2": "string"
          },
          "worker_id": "string"
        },
        "reason": "string",
        "resources": [
          {
            "agents": [
              {
                "apps": [
                  {
                    "command": "string",
                    "display_name": "string",
                    "health": "string",
                    "healthcheck": {},
                    "icon": "string",
                    "id": "string",
                    "sharing_level": "string",
                    "slug": "string",
                    "subdomain": true
                  }
                ],
                "architecture": "string",
                "connection_timeout_seconds": 0,
                "created_at": "string",
                "directory": "string",
                "disconnected_at": "string",
                "environment_variables": {
                  "property1": "string",
                  "property2": "string"
                },
                "first_connected_at": "string",
                "id": "string",
                "instance_id": "string",
                "last_connected_at": "string",
                "latency": {
                  "property1": {
                    "latency_ms": 0,
                    "preferred": true
                  },
                  "property2": {
                    "latency_ms": 0,
                    "preferred": true
                  }
                },
                "name": "string",
                "operating_system": "string",
                "resource_id": "string",
                "startup_script": "string",
                "status": "string",
                "troubleshooting_url": "string",
                "updated_at": "string",
                "version": "string"
              }
            ],
            "created_at": "string",
            "daily_cost": 0,
            "hide": true,
            "icon": "string",
            "id": "string",
            "job_id": "string",
            "metadata": [
              {
                "key": "string",
                "sensitive": true,
                "value": "string"
              }
            ],
            "name": "string",
            "type": "string",
            "workspace_transition": "string"
          }
        ],
        "status": "string",
        "template_version_id": "string",
        "template_version_name": "string",
        "transition": "string",
        "updated_at": "string",
        "workspace_id": "string",
        "workspace_name": "string",
        "workspace_owner_id": "string",
        "workspace_owner_name": "string"
      },
      "name": "string",
      "outdated": true,
      "owner_id": "string",
      "owner_name": "string",
      "template_allow_user_cancel_workspace_jobs": true,
      "template_display_name": "string",
      "template_icon": "string",
      "template_id": "string",
      "template_name": "string",
      "ttl_ms": 0,
      "updated_at": "string"
    }
  ]
}
```

<h3 id="list-workspaces-responses">Responses</h3>

| Status | Meaning                                                                    | Description           | Schema                                                            |
| ------ | -------------------------------------------------------------------------- | --------------------- | ----------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)                    | OK                    | [codersdk.WorkspacesResponse](#schemacodersdk.workspacesresponse) |
| 400    | [Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)           | Bad Request           | [codersdk.Response](#schemacodersdk.response)                     |
| 500    | [Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1) | Internal Server Error | [codersdk.Response](#schemacodersdk.response)                     |

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
CoderSessionToken
</aside>

## Get workspace metadata

<a id="opIdget-workspace"></a>

> Code samples

```shell
# You can also use wget
curl -X GET /api/v2/workspaces/{id} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'

```

`GET /workspaces/{id}`

<h3 id="get-workspace-metadata-parameters">Parameters</h3>

| Name            | In    | Type   | Required | Description     |
| --------------- | ----- | ------ | -------- | --------------- |
| id              | path  | string | true     | Workspace ID    |
| include_deleted | query | string | false    | Include deleted |

> Example responses

> 200 Response

```json
{
  "autostart_schedule": "string",
  "created_at": "string",
  "id": "string",
  "last_used_at": "string",
  "latest_build": {
    "build_number": 0,
    "created_at": "string",
    "daily_cost": 0,
    "deadline": {
      "time": "string",
      "valid": true
    },
    "id": "string",
    "initiator_id": "string",
    "initiator_name": "string",
    "job": {
      "canceled_at": "string",
      "completed_at": "string",
      "created_at": "string",
      "error": "string",
      "file_id": "string",
      "id": "string",
      "started_at": "string",
      "status": "string",
      "tags": {
        "property1": "string",
        "property2": "string"
      },
      "worker_id": "string"
    },
    "reason": "string",
    "resources": [
      {
        "agents": [
          {
            "apps": [
              {
                "command": "string",
                "display_name": "string",
                "health": "string",
                "healthcheck": {
                  "interval": 0,
                  "threshold": 0,
                  "url": "string"
                },
                "icon": "string",
                "id": "string",
                "sharing_level": "string",
                "slug": "string",
                "subdomain": true
              }
            ],
            "architecture": "string",
            "connection_timeout_seconds": 0,
            "created_at": "string",
            "directory": "string",
            "disconnected_at": "string",
            "environment_variables": {
              "property1": "string",
              "property2": "string"
            },
            "first_connected_at": "string",
            "id": "string",
            "instance_id": "string",
            "last_connected_at": "string",
            "latency": {
              "property1": {
                "latency_ms": 0,
                "preferred": true
              },
              "property2": {
                "latency_ms": 0,
                "preferred": true
              }
            },
            "name": "string",
            "operating_system": "string",
            "resource_id": "string",
            "startup_script": "string",
            "status": "string",
            "troubleshooting_url": "string",
            "updated_at": "string",
            "version": "string"
          }
        ],
        "created_at": "string",
        "daily_cost": 0,
        "hide": true,
        "icon": "string",
        "id": "string",
        "job_id": "string",
        "metadata": [
          {
            "key": "string",
            "sensitive": true,
            "value": "string"
          }
        ],
        "name": "string",
        "type": "string",
        "workspace_transition": "string"
      }
    ],
    "status": "string",
    "template_version_id": "string",
    "template_version_name": "string",
    "transition": "string",
    "updated_at": "string",
    "workspace_id": "string",
    "workspace_name": "string",
    "workspace_owner_id": "string",
    "workspace_owner_name": "string"
  },
  "name": "string",
  "outdated": true,
  "owner_id": "string",
  "owner_name": "string",
  "template_allow_user_cancel_workspace_jobs": true,
  "template_display_name": "string",
  "template_icon": "string",
  "template_id": "string",
  "template_name": "string",
  "ttl_ms": 0,
  "updated_at": "string"
}
```

<h3 id="get-workspace-metadata-responses">Responses</h3>

| Status | Meaning                                                                    | Description           | Schema                                          |
| ------ | -------------------------------------------------------------------------- | --------------------- | ----------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)                    | OK                    | [codersdk.Workspace](#schemacodersdk.workspace) |
| 400    | [Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)           | Bad Request           | [codersdk.Response](#schemacodersdk.response)   |
| 404    | [Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)             | Not Found             | [codersdk.Response](#schemacodersdk.response)   |
| 410    | [Gone](https://tools.ietf.org/html/rfc7231#section-6.5.9)                  | Gone                  | [codersdk.Response](#schemacodersdk.response)   |
| 500    | [Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1) | Internal Server Error | [codersdk.Response](#schemacodersdk.response)   |

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
CoderSessionToken
</aside>

# Schemas

<h2 id="tocS_codersdk.DERPRegion">codersdk.DERPRegion</h2>

<a id="schemacodersdk.derpregion"></a>
<a id="schema_codersdk.DERPRegion"></a>
<a id="tocScodersdk.derpregion"></a>
<a id="tocscodersdk.derpregion"></a>

```json
{
  "latency_ms": 0,
  "preferred": true
}
```

### Properties

| Name       | Type    | Required | Restrictions | Description |
| ---------- | ------- | -------- | ------------ | ----------- |
| latency_ms | number  | false    | none         | none        |
| preferred  | boolean | false    | none         | none        |

<h2 id="tocS_codersdk.Healthcheck">codersdk.Healthcheck</h2>

<a id="schemacodersdk.healthcheck"></a>
<a id="schema_codersdk.Healthcheck"></a>
<a id="tocScodersdk.healthcheck"></a>
<a id="tocscodersdk.healthcheck"></a>

```json
{
  "interval": 0,
  "threshold": 0,
  "url": "string"
}
```

### Properties

| Name      | Type    | Required | Restrictions | Description                                                                                      |
| --------- | ------- | -------- | ------------ | ------------------------------------------------------------------------------------------------ |
| interval  | integer | false    | none         | Interval specifies the seconds between each health check.                                        |
| threshold | integer | false    | none         | Threshold specifies the number of consecutive failed health checks before returning "unhealthy". |
| url       | string  | false    | none         | URL specifies the url to check for the app health.                                               |

<h2 id="tocS_codersdk.NullTime">codersdk.NullTime</h2>

<a id="schemacodersdk.nulltime"></a>
<a id="schema_codersdk.NullTime"></a>
<a id="tocScodersdk.nulltime"></a>
<a id="tocscodersdk.nulltime"></a>

```json
{
  "time": "string",
  "valid": true
}
```

### Properties

| Name  | Type    | Required | Restrictions | Description                       |
| ----- | ------- | -------- | ------------ | --------------------------------- |
| time  | string  | false    | none         | none                              |
| valid | boolean | false    | none         | Valid is true if Time is not NULL |

<h2 id="tocS_codersdk.ProvisionerJob">codersdk.ProvisionerJob</h2>

<a id="schemacodersdk.provisionerjob"></a>
<a id="schema_codersdk.ProvisionerJob"></a>
<a id="tocScodersdk.provisionerjob"></a>
<a id="tocscodersdk.provisionerjob"></a>

```json
{
  "canceled_at": "string",
  "completed_at": "string",
  "created_at": "string",
  "error": "string",
  "file_id": "string",
  "id": "string",
  "started_at": "string",
  "status": "string",
  "tags": {
    "property1": "string",
    "property2": "string"
  },
  "worker_id": "string"
}
```

### Properties

| Name                       | Type   | Required | Restrictions | Description |
| -------------------------- | ------ | -------- | ------------ | ----------- |
| canceled_at                | string | false    | none         | none        |
| completed_at               | string | false    | none         | none        |
| created_at                 | string | false    | none         | none        |
| error                      | string | false    | none         | none        |
| file_id                    | string | false    | none         | none        |
| id                         | string | false    | none         | none        |
| started_at                 | string | false    | none         | none        |
| status                     | string | false    | none         | none        |
| tags                       | object | false    | none         | none        |
| » **additionalProperties** | string | false    | none         | none        |
| worker_id                  | string | false    | none         | none        |

<h2 id="tocS_codersdk.Response">codersdk.Response</h2>

<a id="schemacodersdk.response"></a>
<a id="schema_codersdk.Response"></a>
<a id="tocScodersdk.response"></a>
<a id="tocscodersdk.response"></a>

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

### Properties

| Name        | Type                                                          | Required | Restrictions | Description                                                                                                                                                                                                                                    |
| ----------- | ------------------------------------------------------------- | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| detail      | string                                                        | false    | none         | Detail is a debug message that provides further insight into why the<br>action failed. This information can be technical and a regular golang<br>err.Error() text.<br>- "database: too many open connections"<br>- "stat: too many open files" |
| message     | string                                                        | false    | none         | Message is an actionable message that depicts actions the request took.<br>These messages should be fully formed sentences with proper punctuation.<br>Examples:<br>- "A user has been created."<br>- "Failed to create a user."               |
| validations | [[codersdk.ValidationError](#schemacodersdk.validationerror)] | false    | none         | Validations are form field-specific friendly error messages. They will be<br>shown on a form field in the UI. These can also be used to add additional<br>context if there is a set of errors in the primary 'Message'.                        |

<h2 id="tocS_codersdk.ValidationError">codersdk.ValidationError</h2>

<a id="schemacodersdk.validationerror"></a>
<a id="schema_codersdk.ValidationError"></a>
<a id="tocScodersdk.validationerror"></a>
<a id="tocscodersdk.validationerror"></a>

```json
{
  "detail": "string",
  "field": "string"
}
```

### Properties

| Name   | Type   | Required | Restrictions | Description |
| ------ | ------ | -------- | ------------ | ----------- |
| detail | string | true     | none         | none        |
| field  | string | true     | none         | none        |

<h2 id="tocS_codersdk.Workspace">codersdk.Workspace</h2>

<a id="schemacodersdk.workspace"></a>
<a id="schema_codersdk.Workspace"></a>
<a id="tocScodersdk.workspace"></a>
<a id="tocscodersdk.workspace"></a>

```json
{
  "autostart_schedule": "string",
  "created_at": "string",
  "id": "string",
  "last_used_at": "string",
  "latest_build": {
    "build_number": 0,
    "created_at": "string",
    "daily_cost": 0,
    "deadline": {
      "time": "string",
      "valid": true
    },
    "id": "string",
    "initiator_id": "string",
    "initiator_name": "string",
    "job": {
      "canceled_at": "string",
      "completed_at": "string",
      "created_at": "string",
      "error": "string",
      "file_id": "string",
      "id": "string",
      "started_at": "string",
      "status": "string",
      "tags": {
        "property1": "string",
        "property2": "string"
      },
      "worker_id": "string"
    },
    "reason": "string",
    "resources": [
      {
        "agents": [
          {
            "apps": [
              {
                "command": "string",
                "display_name": "string",
                "health": "string",
                "healthcheck": {
                  "interval": 0,
                  "threshold": 0,
                  "url": "string"
                },
                "icon": "string",
                "id": "string",
                "sharing_level": "string",
                "slug": "string",
                "subdomain": true
              }
            ],
            "architecture": "string",
            "connection_timeout_seconds": 0,
            "created_at": "string",
            "directory": "string",
            "disconnected_at": "string",
            "environment_variables": {
              "property1": "string",
              "property2": "string"
            },
            "first_connected_at": "string",
            "id": "string",
            "instance_id": "string",
            "last_connected_at": "string",
            "latency": {
              "property1": {
                "latency_ms": 0,
                "preferred": true
              },
              "property2": {
                "latency_ms": 0,
                "preferred": true
              }
            },
            "name": "string",
            "operating_system": "string",
            "resource_id": "string",
            "startup_script": "string",
            "status": "string",
            "troubleshooting_url": "string",
            "updated_at": "string",
            "version": "string"
          }
        ],
        "created_at": "string",
        "daily_cost": 0,
        "hide": true,
        "icon": "string",
        "id": "string",
        "job_id": "string",
        "metadata": [
          {
            "key": "string",
            "sensitive": true,
            "value": "string"
          }
        ],
        "name": "string",
        "type": "string",
        "workspace_transition": "string"
      }
    ],
    "status": "string",
    "template_version_id": "string",
    "template_version_name": "string",
    "transition": "string",
    "updated_at": "string",
    "workspace_id": "string",
    "workspace_name": "string",
    "workspace_owner_id": "string",
    "workspace_owner_name": "string"
  },
  "name": "string",
  "outdated": true,
  "owner_id": "string",
  "owner_name": "string",
  "template_allow_user_cancel_workspace_jobs": true,
  "template_display_name": "string",
  "template_icon": "string",
  "template_id": "string",
  "template_name": "string",
  "ttl_ms": 0,
  "updated_at": "string"
}
```

### Properties

| Name                                      | Type                                                      | Required | Restrictions | Description |
| ----------------------------------------- | --------------------------------------------------------- | -------- | ------------ | ----------- |
| autostart_schedule                        | string                                                    | false    | none         | none        |
| created_at                                | string                                                    | false    | none         | none        |
| id                                        | string                                                    | false    | none         | none        |
| last_used_at                              | string                                                    | false    | none         | none        |
| latest_build                              | [codersdk.WorkspaceBuild](#schemacodersdk.workspacebuild) | false    | none         | none        |
| name                                      | string                                                    | false    | none         | none        |
| outdated                                  | boolean                                                   | false    | none         | none        |
| owner_id                                  | string                                                    | false    | none         | none        |
| owner_name                                | string                                                    | false    | none         | none        |
| template_allow_user_cancel_workspace_jobs | boolean                                                   | false    | none         | none        |
| template_display_name                     | string                                                    | false    | none         | none        |
| template_icon                             | string                                                    | false    | none         | none        |
| template_id                               | string                                                    | false    | none         | none        |
| template_name                             | string                                                    | false    | none         | none        |
| ttl_ms                                    | integer                                                   | false    | none         | none        |
| updated_at                                | string                                                    | false    | none         | none        |

<h2 id="tocS_codersdk.WorkspaceAgent">codersdk.WorkspaceAgent</h2>

<a id="schemacodersdk.workspaceagent"></a>
<a id="schema_codersdk.WorkspaceAgent"></a>
<a id="tocScodersdk.workspaceagent"></a>
<a id="tocscodersdk.workspaceagent"></a>

```json
{
  "apps": [
    {
      "command": "string",
      "display_name": "string",
      "health": "string",
      "healthcheck": {
        "interval": 0,
        "threshold": 0,
        "url": "string"
      },
      "icon": "string",
      "id": "string",
      "sharing_level": "string",
      "slug": "string",
      "subdomain": true
    }
  ],
  "architecture": "string",
  "connection_timeout_seconds": 0,
  "created_at": "string",
  "directory": "string",
  "disconnected_at": "string",
  "environment_variables": {
    "property1": "string",
    "property2": "string"
  },
  "first_connected_at": "string",
  "id": "string",
  "instance_id": "string",
  "last_connected_at": "string",
  "latency": {
    "property1": {
      "latency_ms": 0,
      "preferred": true
    },
    "property2": {
      "latency_ms": 0,
      "preferred": true
    }
  },
  "name": "string",
  "operating_system": "string",
  "resource_id": "string",
  "startup_script": "string",
  "status": "string",
  "troubleshooting_url": "string",
  "updated_at": "string",
  "version": "string"
}
```

### Properties

| Name                       | Type                                                    | Required | Restrictions | Description                                                             |
| -------------------------- | ------------------------------------------------------- | -------- | ------------ | ----------------------------------------------------------------------- |
| apps                       | [[codersdk.WorkspaceApp](#schemacodersdk.workspaceapp)] | false    | none         | none                                                                    |
| architecture               | string                                                  | false    | none         | none                                                                    |
| connection_timeout_seconds | integer                                                 | false    | none         | none                                                                    |
| created_at                 | string                                                  | false    | none         | none                                                                    |
| directory                  | string                                                  | false    | none         | none                                                                    |
| disconnected_at            | string                                                  | false    | none         | none                                                                    |
| environment_variables      | object                                                  | false    | none         | none                                                                    |
| » **additionalProperties** | string                                                  | false    | none         | none                                                                    |
| first_connected_at         | string                                                  | false    | none         | none                                                                    |
| id                         | string                                                  | false    | none         | none                                                                    |
| instance_id                | string                                                  | false    | none         | none                                                                    |
| last_connected_at          | string                                                  | false    | none         | none                                                                    |
| latency                    | object                                                  | false    | none         | DERPLatency is mapped by region name (e.g. "New York City", "Seattle"). |
| » **additionalProperties** | [codersdk.DERPRegion](#schemacodersdk.derpregion)       | false    | none         | none                                                                    |
| name                       | string                                                  | false    | none         | none                                                                    |
| operating_system           | string                                                  | false    | none         | none                                                                    |
| resource_id                | string                                                  | false    | none         | none                                                                    |
| startup_script             | string                                                  | false    | none         | none                                                                    |
| status                     | string                                                  | false    | none         | none                                                                    |
| troubleshooting_url        | string                                                  | false    | none         | none                                                                    |
| updated_at                 | string                                                  | false    | none         | none                                                                    |
| version                    | string                                                  | false    | none         | none                                                                    |

<h2 id="tocS_codersdk.WorkspaceApp">codersdk.WorkspaceApp</h2>

<a id="schemacodersdk.workspaceapp"></a>
<a id="schema_codersdk.WorkspaceApp"></a>
<a id="tocScodersdk.workspaceapp"></a>
<a id="tocscodersdk.workspaceapp"></a>

```json
{
  "command": "string",
  "display_name": "string",
  "health": "string",
  "healthcheck": {
    "interval": 0,
    "threshold": 0,
    "url": "string"
  },
  "icon": "string",
  "id": "string",
  "sharing_level": "string",
  "slug": "string",
  "subdomain": true
}
```

### Properties

| Name          | Type                                                | Required | Restrictions | Description                                                                                                                                                                                                                                             |
| ------------- | --------------------------------------------------- | -------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| command       | string                                              | false    | none         | none                                                                                                                                                                                                                                                    |
| display_name  | string                                              | false    | none         | DisplayName is a friendly name for the app.                                                                                                                                                                                                             |
| health        | string                                              | false    | none         | none                                                                                                                                                                                                                                                    |
| healthcheck   | [codersdk.Healthcheck](#schemacodersdk.healthcheck) | false    | none         | none                                                                                                                                                                                                                                                    |
| icon          | string                                              | false    | none         | Icon is a relative path or external URL that specifies<br>an icon to be displayed in the dashboard.                                                                                                                                                     |
| id            | string                                              | false    | none         | none                                                                                                                                                                                                                                                    |
| sharing_level | string                                              | false    | none         | none                                                                                                                                                                                                                                                    |
| slug          | string                                              | false    | none         | Slug is a unique identifier within the agent.                                                                                                                                                                                                           |
| subdomain     | boolean                                             | false    | none         | Subdomain denotes whether the app should be accessed via a path on the<br>`coder server` or via a hostname-based dev URL. If this is set to true<br>and there is no app wildcard configured on the server, the app will not<br>be accessible in the UI. |

<h2 id="tocS_codersdk.WorkspaceBuild">codersdk.WorkspaceBuild</h2>

<a id="schemacodersdk.workspacebuild"></a>
<a id="schema_codersdk.WorkspaceBuild"></a>
<a id="tocScodersdk.workspacebuild"></a>
<a id="tocscodersdk.workspacebuild"></a>

```json
{
  "build_number": 0,
  "created_at": "string",
  "daily_cost": 0,
  "deadline": {
    "time": "string",
    "valid": true
  },
  "id": "string",
  "initiator_id": "string",
  "initiator_name": "string",
  "job": {
    "canceled_at": "string",
    "completed_at": "string",
    "created_at": "string",
    "error": "string",
    "file_id": "string",
    "id": "string",
    "started_at": "string",
    "status": "string",
    "tags": {
      "property1": "string",
      "property2": "string"
    },
    "worker_id": "string"
  },
  "reason": "string",
  "resources": [
    {
      "agents": [
        {
          "apps": [
            {
              "command": "string",
              "display_name": "string",
              "health": "string",
              "healthcheck": {
                "interval": 0,
                "threshold": 0,
                "url": "string"
              },
              "icon": "string",
              "id": "string",
              "sharing_level": "string",
              "slug": "string",
              "subdomain": true
            }
          ],
          "architecture": "string",
          "connection_timeout_seconds": 0,
          "created_at": "string",
          "directory": "string",
          "disconnected_at": "string",
          "environment_variables": {
            "property1": "string",
            "property2": "string"
          },
          "first_connected_at": "string",
          "id": "string",
          "instance_id": "string",
          "last_connected_at": "string",
          "latency": {
            "property1": {
              "latency_ms": 0,
              "preferred": true
            },
            "property2": {
              "latency_ms": 0,
              "preferred": true
            }
          },
          "name": "string",
          "operating_system": "string",
          "resource_id": "string",
          "startup_script": "string",
          "status": "string",
          "troubleshooting_url": "string",
          "updated_at": "string",
          "version": "string"
        }
      ],
      "created_at": "string",
      "daily_cost": 0,
      "hide": true,
      "icon": "string",
      "id": "string",
      "job_id": "string",
      "metadata": [
        {
          "key": "string",
          "sensitive": true,
          "value": "string"
        }
      ],
      "name": "string",
      "type": "string",
      "workspace_transition": "string"
    }
  ],
  "status": "string",
  "template_version_id": "string",
  "template_version_name": "string",
  "transition": "string",
  "updated_at": "string",
  "workspace_id": "string",
  "workspace_name": "string",
  "workspace_owner_id": "string",
  "workspace_owner_name": "string"
}
```

### Properties

| Name                  | Type                                                              | Required | Restrictions | Description |
| --------------------- | ----------------------------------------------------------------- | -------- | ------------ | ----------- |
| build_number          | integer                                                           | false    | none         | none        |
| created_at            | string                                                            | false    | none         | none        |
| daily_cost            | integer                                                           | false    | none         | none        |
| deadline              | [codersdk.NullTime](#schemacodersdk.nulltime)                     | false    | none         | none        |
| id                    | string                                                            | false    | none         | none        |
| initiator_id          | string                                                            | false    | none         | none        |
| initiator_name        | string                                                            | false    | none         | none        |
| job                   | [codersdk.ProvisionerJob](#schemacodersdk.provisionerjob)         | false    | none         | none        |
| reason                | string                                                            | false    | none         | none        |
| resources             | [[codersdk.WorkspaceResource](#schemacodersdk.workspaceresource)] | false    | none         | none        |
| status                | string                                                            | false    | none         | none        |
| template_version_id   | string                                                            | false    | none         | none        |
| template_version_name | string                                                            | false    | none         | none        |
| transition            | string                                                            | false    | none         | none        |
| updated_at            | string                                                            | false    | none         | none        |
| workspace_id          | string                                                            | false    | none         | none        |
| workspace_name        | string                                                            | false    | none         | none        |
| workspace_owner_id    | string                                                            | false    | none         | none        |
| workspace_owner_name  | string                                                            | false    | none         | none        |

<h2 id="tocS_codersdk.WorkspaceResource">codersdk.WorkspaceResource</h2>

<a id="schemacodersdk.workspaceresource"></a>
<a id="schema_codersdk.WorkspaceResource"></a>
<a id="tocScodersdk.workspaceresource"></a>
<a id="tocscodersdk.workspaceresource"></a>

```json
{
  "agents": [
    {
      "apps": [
        {
          "command": "string",
          "display_name": "string",
          "health": "string",
          "healthcheck": {
            "interval": 0,
            "threshold": 0,
            "url": "string"
          },
          "icon": "string",
          "id": "string",
          "sharing_level": "string",
          "slug": "string",
          "subdomain": true
        }
      ],
      "architecture": "string",
      "connection_timeout_seconds": 0,
      "created_at": "string",
      "directory": "string",
      "disconnected_at": "string",
      "environment_variables": {
        "property1": "string",
        "property2": "string"
      },
      "first_connected_at": "string",
      "id": "string",
      "instance_id": "string",
      "last_connected_at": "string",
      "latency": {
        "property1": {
          "latency_ms": 0,
          "preferred": true
        },
        "property2": {
          "latency_ms": 0,
          "preferred": true
        }
      },
      "name": "string",
      "operating_system": "string",
      "resource_id": "string",
      "startup_script": "string",
      "status": "string",
      "troubleshooting_url": "string",
      "updated_at": "string",
      "version": "string"
    }
  ],
  "created_at": "string",
  "daily_cost": 0,
  "hide": true,
  "icon": "string",
  "id": "string",
  "job_id": "string",
  "metadata": [
    {
      "key": "string",
      "sensitive": true,
      "value": "string"
    }
  ],
  "name": "string",
  "type": "string",
  "workspace_transition": "string"
}
```

### Properties

| Name                 | Type                                                                              | Required | Restrictions | Description |
| -------------------- | --------------------------------------------------------------------------------- | -------- | ------------ | ----------- |
| agents               | [[codersdk.WorkspaceAgent](#schemacodersdk.workspaceagent)]                       | false    | none         | none        |
| created_at           | string                                                                            | false    | none         | none        |
| daily_cost           | integer                                                                           | false    | none         | none        |
| hide                 | boolean                                                                           | false    | none         | none        |
| icon                 | string                                                                            | false    | none         | none        |
| id                   | string                                                                            | false    | none         | none        |
| job_id               | string                                                                            | false    | none         | none        |
| metadata             | [[codersdk.WorkspaceResourceMetadata](#schemacodersdk.workspaceresourcemetadata)] | false    | none         | none        |
| name                 | string                                                                            | false    | none         | none        |
| type                 | string                                                                            | false    | none         | none        |
| workspace_transition | string                                                                            | false    | none         | none        |

<h2 id="tocS_codersdk.WorkspaceResourceMetadata">codersdk.WorkspaceResourceMetadata</h2>

<a id="schemacodersdk.workspaceresourcemetadata"></a>
<a id="schema_codersdk.WorkspaceResourceMetadata"></a>
<a id="tocScodersdk.workspaceresourcemetadata"></a>
<a id="tocscodersdk.workspaceresourcemetadata"></a>

```json
{
  "key": "string",
  "sensitive": true,
  "value": "string"
}
```

### Properties

| Name      | Type    | Required | Restrictions | Description |
| --------- | ------- | -------- | ------------ | ----------- |
| key       | string  | false    | none         | none        |
| sensitive | boolean | false    | none         | none        |
| value     | string  | false    | none         | none        |

<h2 id="tocS_codersdk.WorkspacesResponse">codersdk.WorkspacesResponse</h2>

<a id="schemacodersdk.workspacesresponse"></a>
<a id="schema_codersdk.WorkspacesResponse"></a>
<a id="tocScodersdk.workspacesresponse"></a>
<a id="tocscodersdk.workspacesresponse"></a>

```json
{
  "count": 0,
  "workspaces": [
    {
      "autostart_schedule": "string",
      "created_at": "string",
      "id": "string",
      "last_used_at": "string",
      "latest_build": {
        "build_number": 0,
        "created_at": "string",
        "daily_cost": 0,
        "deadline": {
          "time": "string",
          "valid": true
        },
        "id": "string",
        "initiator_id": "string",
        "initiator_name": "string",
        "job": {
          "canceled_at": "string",
          "completed_at": "string",
          "created_at": "string",
          "error": "string",
          "file_id": "string",
          "id": "string",
          "started_at": "string",
          "status": "string",
          "tags": {
            "property1": "string",
            "property2": "string"
          },
          "worker_id": "string"
        },
        "reason": "string",
        "resources": [
          {
            "agents": [
              {
                "apps": [
                  {
                    "command": "string",
                    "display_name": "string",
                    "health": "string",
                    "healthcheck": {},
                    "icon": "string",
                    "id": "string",
                    "sharing_level": "string",
                    "slug": "string",
                    "subdomain": true
                  }
                ],
                "architecture": "string",
                "connection_timeout_seconds": 0,
                "created_at": "string",
                "directory": "string",
                "disconnected_at": "string",
                "environment_variables": {
                  "property1": "string",
                  "property2": "string"
                },
                "first_connected_at": "string",
                "id": "string",
                "instance_id": "string",
                "last_connected_at": "string",
                "latency": {
                  "property1": {
                    "latency_ms": 0,
                    "preferred": true
                  },
                  "property2": {
                    "latency_ms": 0,
                    "preferred": true
                  }
                },
                "name": "string",
                "operating_system": "string",
                "resource_id": "string",
                "startup_script": "string",
                "status": "string",
                "troubleshooting_url": "string",
                "updated_at": "string",
                "version": "string"
              }
            ],
            "created_at": "string",
            "daily_cost": 0,
            "hide": true,
            "icon": "string",
            "id": "string",
            "job_id": "string",
            "metadata": [
              {
                "key": "string",
                "sensitive": true,
                "value": "string"
              }
            ],
            "name": "string",
            "type": "string",
            "workspace_transition": "string"
          }
        ],
        "status": "string",
        "template_version_id": "string",
        "template_version_name": "string",
        "transition": "string",
        "updated_at": "string",
        "workspace_id": "string",
        "workspace_name": "string",
        "workspace_owner_id": "string",
        "workspace_owner_name": "string"
      },
      "name": "string",
      "outdated": true,
      "owner_id": "string",
      "owner_name": "string",
      "template_allow_user_cancel_workspace_jobs": true,
      "template_display_name": "string",
      "template_icon": "string",
      "template_id": "string",
      "template_name": "string",
      "ttl_ms": 0,
      "updated_at": "string"
    }
  ]
}
```

### Properties

| Name       | Type                                              | Required | Restrictions | Description |
| ---------- | ------------------------------------------------- | -------- | ------------ | ----------- |
| count      | integer                                           | false    | none         | none        |
| workspaces | [[codersdk.Workspace](#schemacodersdk.workspace)] | false    | none         | none        |
