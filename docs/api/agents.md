# Agents

## Get DERP map updates

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/derp-map \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /derp-map`

### Responses

| Status | Meaning                                                                  | Description         | Schema |
| ------ | ------------------------------------------------------------------------ | ------------------- | ------ |
| 101    | [Switching Protocols](https://tools.ietf.org/html/rfc7231#section-6.2.2) | Switching Protocols |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Authenticate agent on AWS instance

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/workspaceagents/aws-instance-identity \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /workspaceagents/aws-instance-identity`

> Body parameter

```json
{
  "document": "string",
  "signature": "string"
}
```

### Parameters

| Name   | In   | Type                                                                             | Required | Description             |
| ------ | ---- | -------------------------------------------------------------------------------- | -------- | ----------------------- |
| `body` | body | [agentsdk.AWSInstanceIdentityToken](schemas.md#agentsdkawsinstanceidentitytoken) | true     | Instance identity token |

### Example responses

> 200 Response

```json
{
  "session_token": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                   |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [agentsdk.AuthenticateResponse](schemas.md#agentsdkauthenticateresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Authenticate agent on Azure instance

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/workspaceagents/azure-instance-identity \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /workspaceagents/azure-instance-identity`

> Body parameter

```json
{
  "encoding": "string",
  "signature": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                 | Required | Description             |
| ------ | ---- | ------------------------------------------------------------------------------------ | -------- | ----------------------- |
| `body` | body | [agentsdk.AzureInstanceIdentityToken](schemas.md#agentsdkazureinstanceidentitytoken) | true     | Instance identity token |

### Example responses

> 200 Response

```json
{
  "session_token": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                   |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [agentsdk.AuthenticateResponse](schemas.md#agentsdkauthenticateresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Authenticate agent on Google Cloud instance

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/workspaceagents/google-instance-identity \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /workspaceagents/google-instance-identity`

> Body parameter

```json
{
  "json_web_token": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                   | Required | Description             |
| ------ | ---- | -------------------------------------------------------------------------------------- | -------- | ----------------------- |
| `body` | body | [agentsdk.GoogleInstanceIdentityToken](schemas.md#agentsdkgoogleinstanceidentitytoken) | true     | Instance identity token |

### Example responses

> 200 Response

```json
{
  "session_token": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                   |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [agentsdk.AuthenticateResponse](schemas.md#agentsdkauthenticateresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Submit workspace agent application health

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/workspaceagents/me/app-health \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /workspaceagents/me/app-health`

> Body parameter

```json
{
  "healths": {
    "property1": "disabled",
    "property2": "disabled"
  }
}
```

### Parameters

| Name   | In   | Type                                                                       | Required | Description                |
| ------ | ---- | -------------------------------------------------------------------------- | -------- | -------------------------- |
| `body` | body | [agentsdk.PostAppHealthsRequest](schemas.md#agentsdkpostapphealthsrequest) | true     | Application health request |

### Responses

| Status | Meaning                                                 | Description | Schema |
| ------ | ------------------------------------------------------- | ----------- | ------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Coordinate workspace agent via Tailnet

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspaceagents/me/coordinate \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspaceagents/me/coordinate`

It accepts a WebSocket connection to an agent that listens to
incoming connections and publishes node updates.

### Responses

| Status | Meaning                                                                  | Description         | Schema |
| ------ | ------------------------------------------------------------------------ | ------------------- | ------ |
| 101    | [Switching Protocols](https://tools.ietf.org/html/rfc7231#section-6.2.2) | Switching Protocols |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get workspace agent Git auth

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspaceagents/me/gitauth?url=http%3A%2F%2Fexample.com \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspaceagents/me/gitauth`

### Parameters

| Name     | In    | Type        | Required | Description                       |
| -------- | ----- | ----------- | -------- | --------------------------------- |
| `url`    | query | string(uri) | true     | Git URL                           |
| `listen` | query | boolean     | false    | Wait for a new token to be issued |

### Example responses

> 200 Response

```json
{
  "password": "string",
  "url": "string",
  "username": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                         |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [agentsdk.GitAuthResponse](schemas.md#agentsdkgitauthresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get workspace agent Git SSH key

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspaceagents/me/gitsshkey \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspaceagents/me/gitsshkey`

### Example responses

> 200 Response

```json
{
  "private_key": "string",
  "public_key": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                             |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [agentsdk.GitSSHKey](schemas.md#agentsdkgitsshkey) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get authorized workspace agent manifest

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspaceagents/me/manifest \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspaceagents/me/manifest`

### Example responses

> 200 Response

```json
{
  "agent_id": "string",
  "apps": [
    {
      "command": "string",
      "display_name": "string",
      "external": true,
      "health": "disabled",
      "healthcheck": {
        "interval": 0,
        "threshold": 0,
        "url": "string"
      },
      "icon": "string",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "sharing_level": "owner",
      "slug": "string",
      "subdomain": true,
      "url": "string"
    }
  ],
  "derpmap": {
    "omitDefaultRegions": true,
    "regions": {
      "property1": {
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
      },
      "property2": {
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
  },
  "directory": "string",
  "disable_direct_connections": true,
  "environment_variables": {
    "property1": "string",
    "property2": "string"
  },
  "git_auth_configs": 0,
  "metadata": [
    {
      "display_name": "string",
      "interval": 0,
      "key": "string",
      "script": "string",
      "timeout": 0
    }
  ],
  "motd_file": "string",
  "shutdown_script": "string",
  "shutdown_script_timeout": 0,
  "startup_script": "string",
  "startup_script_timeout": 0,
  "vscode_port_proxy_uri": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                           |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [agentsdk.Manifest](schemas.md#agentsdkmanifest) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Submit workspace agent stats

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/workspaceagents/me/report-stats \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /workspaceagents/me/report-stats`

> Body parameter

```json
{
  "connection_count": 0,
  "connection_median_latency_ms": 0,
  "connections_by_proto": {
    "property1": 0,
    "property2": 0
  },
  "metrics": [
    {
      "labels": [
        {
          "name": "string",
          "value": "string"
        }
      ],
      "name": "string",
      "type": "counter",
      "value": 0
    }
  ],
  "rx_bytes": 0,
  "rx_packets": 0,
  "session_count_jetbrains": 0,
  "session_count_reconnecting_pty": 0,
  "session_count_ssh": 0,
  "session_count_vscode": 0,
  "tx_bytes": 0,
  "tx_packets": 0
}
```

### Parameters

| Name   | In   | Type                                       | Required | Description   |
| ------ | ---- | ------------------------------------------ | -------- | ------------- |
| `body` | body | [agentsdk.Stats](schemas.md#agentsdkstats) | true     | Stats request |

### Example responses

> 200 Response

```json
{
  "report_interval": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                     |
| ------ | ------------------------------------------------------- | ----------- | ---------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [agentsdk.StatsResponse](schemas.md#agentsdkstatsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Patch workspace agent startup logs

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/workspaceagents/me/startup-logs \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /workspaceagents/me/startup-logs`

> Body parameter

```json
{
  "logs": [
    {
      "created_at": "string",
      "level": "trace",
      "output": "string"
    }
  ]
}
```

### Parameters

| Name   | In   | Type                                                             | Required | Description  |
| ------ | ---- | ---------------------------------------------------------------- | -------- | ------------ |
| `body` | body | [agentsdk.PatchStartupLogs](schemas.md#agentsdkpatchstartuplogs) | true     | Startup logs |

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

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get workspace agent by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspaceagents/{workspaceagent} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspaceagents/{workspaceagent}`

### Parameters

| Name             | In   | Type         | Required | Description        |
| ---------------- | ---- | ------------ | -------- | ------------------ |
| `workspaceagent` | path | string(uuid) | true     | Workspace agent ID |

### Example responses

> 200 Response

```json
{
  "apps": [
    {
      "command": "string",
      "display_name": "string",
      "external": true,
      "health": "disabled",
      "healthcheck": {
        "interval": 0,
        "threshold": 0,
        "url": "string"
      },
      "icon": "string",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "sharing_level": "owner",
      "slug": "string",
      "subdomain": true,
      "url": "string"
    }
  ],
  "architecture": "string",
  "connection_timeout_seconds": 0,
  "created_at": "2019-08-24T14:15:22Z",
  "directory": "string",
  "disconnected_at": "2019-08-24T14:15:22Z",
  "environment_variables": {
    "property1": "string",
    "property2": "string"
  },
  "expanded_directory": "string",
  "first_connected_at": "2019-08-24T14:15:22Z",
  "health": {
    "healthy": false,
    "reason": "agent has lost connection"
  },
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "instance_id": "string",
  "last_connected_at": "2019-08-24T14:15:22Z",
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
  "lifecycle_state": "created",
  "login_before_ready": true,
  "name": "string",
  "operating_system": "string",
  "ready_at": "2019-08-24T14:15:22Z",
  "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
  "shutdown_script": "string",
  "shutdown_script_timeout_seconds": 0,
  "started_at": "2019-08-24T14:15:22Z",
  "startup_logs_length": 0,
  "startup_logs_overflowed": true,
  "startup_script": "string",
  "startup_script_behavior": "blocking",
  "startup_script_timeout_seconds": 0,
  "status": "connecting",
  "subsystem": "envbox",
  "troubleshooting_url": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "version": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceAgent](schemas.md#codersdkworkspaceagent) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get connection info for workspace agent

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspaceagents/{workspaceagent}/connection \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspaceagents/{workspaceagent}/connection`

### Parameters

| Name             | In   | Type         | Required | Description        |
| ---------------- | ---- | ------------ | -------- | ------------------ |
| `workspaceagent` | path | string(uuid) | true     | Workspace agent ID |

### Example responses

> 200 Response

```json
{
  "derp_map": {
    "omitDefaultRegions": true,
    "regions": {
      "property1": {
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
      },
      "property2": {
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
  },
  "disable_direct_connections": true
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                   |
| ------ | ------------------------------------------------------- | ----------- | ---------------------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceAgentConnectionInfo](schemas.md#codersdkworkspaceagentconnectioninfo) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Coordinate workspace agent

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspaceagents/{workspaceagent}/coordinate \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspaceagents/{workspaceagent}/coordinate`

### Parameters

| Name             | In   | Type         | Required | Description        |
| ---------------- | ---- | ------------ | -------- | ------------------ |
| `workspaceagent` | path | string(uuid) | true     | Workspace agent ID |

### Responses

| Status | Meaning                                                                  | Description         | Schema |
| ------ | ------------------------------------------------------------------------ | ------------------- | ------ |
| 101    | [Switching Protocols](https://tools.ietf.org/html/rfc7231#section-6.2.2) | Switching Protocols |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get listening ports for workspace agent

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspaceagents/{workspaceagent}/listening-ports \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspaceagents/{workspaceagent}/listening-ports`

### Parameters

| Name             | In   | Type         | Required | Description        |
| ---------------- | ---- | ------------ | -------- | ------------------ |
| `workspaceagent` | path | string(uuid) | true     | Workspace agent ID |

### Example responses

> 200 Response

```json
{
  "ports": [
    {
      "network": "string",
      "port": 0,
      "process_name": "string"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                                   |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceAgentListeningPortsResponse](schemas.md#codersdkworkspaceagentlisteningportsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Open PTY to workspace agent

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspaceagents/{workspaceagent}/pty \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspaceagents/{workspaceagent}/pty`

### Parameters

| Name             | In   | Type         | Required | Description        |
| ---------------- | ---- | ------------ | -------- | ------------------ |
| `workspaceagent` | path | string(uuid) | true     | Workspace agent ID |

### Responses

| Status | Meaning                                                                  | Description         | Schema |
| ------ | ------------------------------------------------------------------------ | ------------------- | ------ |
| 101    | [Switching Protocols](https://tools.ietf.org/html/rfc7231#section-6.2.2) | Switching Protocols |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get startup logs by workspace agent

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspaceagents/{workspaceagent}/startup-logs \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspaceagents/{workspaceagent}/startup-logs`

### Parameters

| Name             | In    | Type         | Required | Description                                  |
| ---------------- | ----- | ------------ | -------- | -------------------------------------------- |
| `workspaceagent` | path  | string(uuid) | true     | Workspace agent ID                           |
| `before`         | query | integer      | false    | Before log id                                |
| `after`          | query | integer      | false    | After log id                                 |
| `follow`         | query | boolean      | false    | Follow log stream                            |
| `no_compression` | query | boolean      | false    | Disable compression for WebSocket connection |

### Example responses

> 200 Response

```json
[
  {
    "created_at": "2019-08-24T14:15:22Z",
    "id": 0,
    "level": "trace",
    "output": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                    |
| ------ | ------------------------------------------------------- | ----------- | ----------------------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.WorkspaceAgentStartupLog](schemas.md#codersdkworkspaceagentstartuplog) |

<h3 id="get-startup-logs-by-workspace-agent-responseschema">Response Schema</h3>

Status Code **200**

| Name           | Type                                             | Required | Restrictions | Description |
| -------------- | ------------------------------------------------ | -------- | ------------ | ----------- |
| `[array item]` | array                                            | false    |              |             |
| `» created_at` | string(date-time)                                | false    |              |             |
| `» id`         | integer                                          | false    |              |             |
| `» level`      | [codersdk.LogLevel](schemas.md#codersdkloglevel) | false    |              |             |
| `» output`     | string                                           | false    |              |             |

#### Enumerated Values

| Property | Value   |
| -------- | ------- |
| `level`  | `trace` |
| `level`  | `debug` |
| `level`  | `info`  |
| `level`  | `warn`  |
| `level`  | `error` |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
