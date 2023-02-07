# Agents

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

## Get authorized workspace agent metadata

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspaceagents/me/metadata \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspaceagents/me/metadata`

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
  "environment_variables": {
    "property1": "string",
    "property2": "string"
  },
  "git_auth_configs": 0,
  "motd_file": "string",
  "startup_script": "string",
  "startup_script_timeout": 0,
  "vscode_port_proxy_uri": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                           |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [agentsdk.Metadata](schemas.md#agentsdkmetadata) |

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
  "conns_by_proto": {
    "property1": 0,
    "property2": 0
  },
  "num_comms": 0,
  "rx_bytes": 0,
  "rx_packets": 0,
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
  "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
  "startup_script": "string",
  "startup_script_timeout_seconds": 0,
  "status": "connecting",
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
  }
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
