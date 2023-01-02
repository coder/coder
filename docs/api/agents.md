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

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

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

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

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

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

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

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.
