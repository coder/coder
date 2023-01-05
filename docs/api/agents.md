# Agents

> This page is incomplete, stay tuned.

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
| `body` | body | [codersdk.AWSInstanceIdentityToken](schemas.md#codersdkawsinstanceidentitytoken) | true     | Instance identity token |

### Example responses

> 200 Response

```json
{
  "session_token": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                               |
| ------ | ------------------------------------------------------- | ----------- | ---------------------------------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceAgentAuthenticateResponse](schemas.md#codersdkworkspaceagentauthenticateresponse) |

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
| `body` | body | [codersdk.AzureInstanceIdentityToken](schemas.md#codersdkazureinstanceidentitytoken) | true     | Instance identity token |

### Example responses

> 200 Response

```json
{
  "session_token": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                               |
| ------ | ------------------------------------------------------- | ----------- | ---------------------------------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceAgentAuthenticateResponse](schemas.md#codersdkworkspaceagentauthenticateresponse) |

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
| `body` | body | [codersdk.GoogleInstanceIdentityToken](schemas.md#codersdkgoogleinstanceidentitytoken) | true     | Instance identity token |

### Example responses

> 200 Response

```json
{
  "session_token": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                               |
| ------ | ------------------------------------------------------- | ----------- | ---------------------------------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceAgentAuthenticateResponse](schemas.md#codersdkworkspaceagentauthenticateresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Submit workspace application health

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
    "property1": "string",
    "property2": "string"
  }
}
```

### Parameters

| Name   | In   | Type                                                                                         | Required | Description                |
| ------ | ---- | -------------------------------------------------------------------------------------------- | -------- | -------------------------- |
| `body` | body | [codersdk.PostWorkspaceAppHealthsRequest](schemas.md#codersdkpostworkspaceapphealthsrequest) | true     | Application health request |

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

| Status | Meaning                                                 | Description | Schema                                                                                     |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceAgentGitAuthResponse](schemas.md#codersdkworkspaceagentgitauthresponse) |

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

| Status | Meaning                                                 | Description | Schema                                                       |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AgentGitSSHKey](schemas.md#codersdkagentgitsshkey) |

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
      "health": "string",
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
  "vscode_port_proxy_uri": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                       |
| ------ | ------------------------------------------------------- | ----------- | ---------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceAgentMetadata](schemas.md#codersdkworkspaceagentmetadata) |

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

| Name   | In   | Type                                                 | Required | Description   |
| ------ | ---- | ---------------------------------------------------- | -------- | ------------- |
| `body` | body | [codersdk.AgentStats](schemas.md#codersdkagentstats) | true     | Stats request |

### Example responses

> 200 Response

```json
{
  "report_interval": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                               |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AgentStatsResponse](schemas.md#codersdkagentstatsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
