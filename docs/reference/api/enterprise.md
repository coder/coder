# Enterprise

## Get appearance

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/appearance \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /appearance`

### Example responses

> 200 Response

```json
{
  "announcement_banners": [
    {
      "background_color": "string",
      "enabled": true,
      "message": "string"
    }
  ],
  "application_name": "string",
  "docs_url": "string",
  "logo_url": "string",
  "service_banner": {
    "background_color": "string",
    "enabled": true,
    "message": "string"
  },
  "support_links": [
    {
      "icon": "bug",
      "name": "string",
      "target": "string"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                           |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AppearanceConfig](schemas.md#codersdkappearanceconfig) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update appearance

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/appearance \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /appearance`

> Body parameter

```json
{
  "announcement_banners": [
    {
      "background_color": "string",
      "enabled": true,
      "message": "string"
    }
  ],
  "application_name": "string",
  "logo_url": "string",
  "service_banner": {
    "background_color": "string",
    "enabled": true,
    "message": "string"
  }
}
```

### Parameters

| Name   | In   | Type                                                                         | Required | Description               |
|--------|------|------------------------------------------------------------------------------|----------|---------------------------|
| `body` | body | [codersdk.UpdateAppearanceConfig](schemas.md#codersdkupdateappearanceconfig) | true     | Update appearance request |

### Example responses

> 200 Response

```json
{
  "announcement_banners": [
    {
      "background_color": "string",
      "enabled": true,
      "message": "string"
    }
  ],
  "application_name": "string",
  "logo_url": "string",
  "service_banner": {
    "background_color": "string",
    "enabled": true,
    "message": "string"
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                       |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.UpdateAppearanceConfig](schemas.md#codersdkupdateappearanceconfig) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get entitlements

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/entitlements \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /entitlements`

### Example responses

> 200 Response

```json
{
  "errors": [
    "string"
  ],
  "features": {
    "property1": {
      "actual": 0,
      "enabled": true,
      "entitlement": "entitled",
      "limit": 0
    },
    "property2": {
      "actual": 0,
      "enabled": true,
      "entitlement": "entitled",
      "limit": 0
    }
  },
  "has_license": true,
  "refreshed_at": "2019-08-24T14:15:22Z",
  "require_telemetry": true,
  "trial": true,
  "warnings": [
    "string"
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                   |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Entitlements](schemas.md#codersdkentitlements) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get groups

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/groups?organization=string&has_member=string&group_ids=string \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /groups`

### Parameters

| Name           | In    | Type   | Required | Description                       |
|----------------|-------|--------|----------|-----------------------------------|
| `organization` | query | string | true     | Organization ID or name           |
| `has_member`   | query | string | true     | User ID or name                   |
| `group_ids`    | query | string | true     | Comma separated list of group IDs |

### Example responses

> 200 Response

```json
[
  {
    "avatar_url": "string",
    "display_name": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "members": [
      {
        "avatar_url": "http://example.com",
        "created_at": "2019-08-24T14:15:22Z",
        "email": "user@example.com",
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "last_seen_at": "2019-08-24T14:15:22Z",
        "login_type": "",
        "name": "string",
        "status": "active",
        "theme_preference": "string",
        "updated_at": "2019-08-24T14:15:22Z",
        "username": "string"
      }
    ],
    "name": "string",
    "organization_display_name": "string",
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "organization_name": "string",
    "quota_allowance": 0,
    "source": "user",
    "total_member_count": 0
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                              |
|--------|---------------------------------------------------------|-------------|-----------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Group](schemas.md#codersdkgroup) |

<h3 id="get-groups-responseschema">Response Schema</h3>

Status Code **200**

| Name                          | Type                                                   | Required | Restrictions | Description                                                                                                                                                           |
|-------------------------------|--------------------------------------------------------|----------|--------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `[array item]`                | array                                                  | false    |              |                                                                                                                                                                       |
| `» avatar_url`                | string                                                 | false    |              |                                                                                                                                                                       |
| `» display_name`              | string                                                 | false    |              |                                                                                                                                                                       |
| `» id`                        | string(uuid)                                           | false    |              |                                                                                                                                                                       |
| `» members`                   | array                                                  | false    |              |                                                                                                                                                                       |
| `»» avatar_url`               | string(uri)                                            | false    |              |                                                                                                                                                                       |
| `»» created_at`               | string(date-time)                                      | true     |              |                                                                                                                                                                       |
| `»» email`                    | string(email)                                          | true     |              |                                                                                                                                                                       |
| `»» id`                       | string(uuid)                                           | true     |              |                                                                                                                                                                       |
| `»» last_seen_at`             | string(date-time)                                      | false    |              |                                                                                                                                                                       |
| `»» login_type`               | [codersdk.LoginType](schemas.md#codersdklogintype)     | false    |              |                                                                                                                                                                       |
| `»» name`                     | string                                                 | false    |              |                                                                                                                                                                       |
| `»» status`                   | [codersdk.UserStatus](schemas.md#codersdkuserstatus)   | false    |              |                                                                                                                                                                       |
| `»» theme_preference`         | string                                                 | false    |              |                                                                                                                                                                       |
| `»» updated_at`               | string(date-time)                                      | false    |              |                                                                                                                                                                       |
| `»» username`                 | string                                                 | true     |              |                                                                                                                                                                       |
| `» name`                      | string                                                 | false    |              |                                                                                                                                                                       |
| `» organization_display_name` | string                                                 | false    |              |                                                                                                                                                                       |
| `» organization_id`           | string(uuid)                                           | false    |              |                                                                                                                                                                       |
| `» organization_name`         | string                                                 | false    |              |                                                                                                                                                                       |
| `» quota_allowance`           | integer                                                | false    |              |                                                                                                                                                                       |
| `» source`                    | [codersdk.GroupSource](schemas.md#codersdkgroupsource) | false    |              |                                                                                                                                                                       |
| `» total_member_count`        | integer                                                | false    |              | How many members are in this group. Shows the total count, even if the user is not authorized to read group member details. May be greater than `len(Group.Members)`. |

#### Enumerated Values

| Property     | Value       |
|--------------|-------------|
| `login_type` | ``          |
| `login_type` | `password`  |
| `login_type` | `github`    |
| `login_type` | `oidc`      |
| `login_type` | `token`     |
| `login_type` | `none`      |
| `status`     | `active`    |
| `status`     | `suspended` |
| `source`     | `user`      |
| `source`     | `oidc`      |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get group by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/groups/{group} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /groups/{group}`

### Parameters

| Name    | In   | Type   | Required | Description |
|---------|------|--------|----------|-------------|
| `group` | path | string | true     | Group id    |

### Example responses

> 200 Response

```json
{
  "avatar_url": "string",
  "display_name": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "members": [
    {
      "avatar_url": "http://example.com",
      "created_at": "2019-08-24T14:15:22Z",
      "email": "user@example.com",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "last_seen_at": "2019-08-24T14:15:22Z",
      "login_type": "",
      "name": "string",
      "status": "active",
      "theme_preference": "string",
      "updated_at": "2019-08-24T14:15:22Z",
      "username": "string"
    }
  ],
  "name": "string",
  "organization_display_name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "organization_name": "string",
  "quota_allowance": 0,
  "source": "user",
  "total_member_count": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                     |
|--------|---------------------------------------------------------|-------------|--------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Group](schemas.md#codersdkgroup) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete group by name

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/groups/{group} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /groups/{group}`

### Parameters

| Name    | In   | Type   | Required | Description |
|---------|------|--------|----------|-------------|
| `group` | path | string | true     | Group name  |

### Example responses

> 200 Response

```json
{
  "avatar_url": "string",
  "display_name": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "members": [
    {
      "avatar_url": "http://example.com",
      "created_at": "2019-08-24T14:15:22Z",
      "email": "user@example.com",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "last_seen_at": "2019-08-24T14:15:22Z",
      "login_type": "",
      "name": "string",
      "status": "active",
      "theme_preference": "string",
      "updated_at": "2019-08-24T14:15:22Z",
      "username": "string"
    }
  ],
  "name": "string",
  "organization_display_name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "organization_name": "string",
  "quota_allowance": 0,
  "source": "user",
  "total_member_count": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                     |
|--------|---------------------------------------------------------|-------------|--------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Group](schemas.md#codersdkgroup) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update group by name

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/groups/{group} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /groups/{group}`

> Body parameter

```json
{
  "add_users": [
    "string"
  ],
  "avatar_url": "string",
  "display_name": "string",
  "name": "string",
  "quota_allowance": 0,
  "remove_users": [
    "string"
  ]
}
```

### Parameters

| Name    | In   | Type                                                               | Required | Description         |
|---------|------|--------------------------------------------------------------------|----------|---------------------|
| `group` | path | string                                                             | true     | Group name          |
| `body`  | body | [codersdk.PatchGroupRequest](schemas.md#codersdkpatchgrouprequest) | true     | Patch group request |

### Example responses

> 200 Response

```json
{
  "avatar_url": "string",
  "display_name": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "members": [
    {
      "avatar_url": "http://example.com",
      "created_at": "2019-08-24T14:15:22Z",
      "email": "user@example.com",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "last_seen_at": "2019-08-24T14:15:22Z",
      "login_type": "",
      "name": "string",
      "status": "active",
      "theme_preference": "string",
      "updated_at": "2019-08-24T14:15:22Z",
      "username": "string"
    }
  ],
  "name": "string",
  "organization_display_name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "organization_name": "string",
  "quota_allowance": 0,
  "source": "user",
  "total_member_count": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                     |
|--------|---------------------------------------------------------|-------------|--------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Group](schemas.md#codersdkgroup) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get JFrog XRay scan by workspace agent ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/integrations/jfrog/xray-scan?workspace_id=string&agent_id=string \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /integrations/jfrog/xray-scan`

### Parameters

| Name           | In    | Type   | Required | Description  |
|----------------|-------|--------|----------|--------------|
| `workspace_id` | query | string | true     | Workspace ID |
| `agent_id`     | query | string | true     | Agent ID     |

### Example responses

> 200 Response

```json
{
  "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
  "critical": 0,
  "high": 0,
  "medium": 0,
  "results_url": "string",
  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                     |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.JFrogXrayScan](schemas.md#codersdkjfrogxrayscan) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Post JFrog XRay scan by workspace agent ID

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/integrations/jfrog/xray-scan \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /integrations/jfrog/xray-scan`

> Body parameter

```json
{
  "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
  "critical": 0,
  "high": 0,
  "medium": 0,
  "results_url": "string",
  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
}
```

### Parameters

| Name   | In   | Type                                                       | Required | Description                  |
|--------|------|------------------------------------------------------------|----------|------------------------------|
| `body` | body | [codersdk.JFrogXrayScan](schemas.md#codersdkjfrogxrayscan) | true     | Post JFrog XRay scan request |

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

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get licenses

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/licenses \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /licenses`

### Example responses

> 200 Response

```json
[
  {
    "claims": {},
    "id": 0,
    "uploaded_at": "2019-08-24T14:15:22Z",
    "uuid": "095be615-a8ad-4c33-8e9c-c7612fbf6c9f"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                  |
|--------|---------------------------------------------------------|-------------|---------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.License](schemas.md#codersdklicense) |

<h3 id="get-licenses-responseschema">Response Schema</h3>

Status Code **200**

| Name            | Type              | Required | Restrictions | Description                                                                                                                                                                                             |
|-----------------|-------------------|----------|--------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `[array item]`  | array             | false    |              |                                                                                                                                                                                                         |
| `» claims`      | object            | false    |              | Claims are the JWT claims asserted by the license.  Here we use a generic string map to ensure that all data from the server is parsed verbatim, not just the fields this version of Coder understands. |
| `» id`          | integer           | false    |              |                                                                                                                                                                                                         |
| `» uploaded_at` | string(date-time) | false    |              |                                                                                                                                                                                                         |
| `» uuid`        | string(uuid)      | false    |              |                                                                                                                                                                                                         |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete license

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/licenses/{id} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /licenses/{id}`

### Parameters

| Name | In   | Type           | Required | Description |
|------|------|----------------|----------|-------------|
| `id` | path | string(number) | true     | License ID  |

### Responses

| Status | Meaning                                                 | Description | Schema |
|--------|---------------------------------------------------------|-------------|--------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update notification template dispatch method

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/notifications/templates/{notification_template}/method \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /notifications/templates/{notification_template}/method`

### Parameters

| Name                    | In   | Type   | Required | Description                |
|-------------------------|------|--------|----------|----------------------------|
| `notification_template` | path | string | true     | Notification template UUID |

### Responses

| Status | Meaning                                                         | Description  | Schema |
|--------|-----------------------------------------------------------------|--------------|--------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)         | Success      |        |
| 304    | [Not Modified](https://tools.ietf.org/html/rfc7232#section-4.1) | Not modified |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get OAuth2 applications

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/oauth2-provider/apps \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /oauth2-provider/apps`

### Parameters

| Name      | In    | Type   | Required | Description                                  |
|-----------|-------|--------|----------|----------------------------------------------|
| `user_id` | query | string | false    | Filter by applications authorized for a user |

### Example responses

> 200 Response

```json
[
  {
    "callback_url": "string",
    "endpoints": {
      "authorization": "string",
      "device_authorization": "string",
      "token": "string"
    },
    "icon": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "name": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                      |
|--------|---------------------------------------------------------|-------------|-----------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.OAuth2ProviderApp](schemas.md#codersdkoauth2providerapp) |

<h3 id="get-oauth2-applications.-responseschema">Response Schema</h3>

Status Code **200**

| Name                      | Type                                                                 | Required | Restrictions | Description                                                                                                                                                                                             |
|---------------------------|----------------------------------------------------------------------|----------|--------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `[array item]`            | array                                                                | false    |              |                                                                                                                                                                                                         |
| `» callback_url`          | string                                                               | false    |              |                                                                                                                                                                                                         |
| `» endpoints`             | [codersdk.OAuth2AppEndpoints](schemas.md#codersdkoauth2appendpoints) | false    |              | Endpoints are included in the app response for easier discovery. The OAuth2 spec does not have a defined place to find these (for comparison, OIDC has a '/.well-known/openid-configuration' endpoint). |
| `»» authorization`        | string                                                               | false    |              |                                                                                                                                                                                                         |
| `»» device_authorization` | string                                                               | false    |              | Device authorization is optional.                                                                                                                                                                       |
| `»» token`                | string                                                               | false    |              |                                                                                                                                                                                                         |
| `» icon`                  | string                                                               | false    |              |                                                                                                                                                                                                         |
| `» id`                    | string(uuid)                                                         | false    |              |                                                                                                                                                                                                         |
| `» name`                  | string                                                               | false    |              |                                                                                                                                                                                                         |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create OAuth2 application

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/oauth2-provider/apps \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /oauth2-provider/apps`

> Body parameter

```json
{
  "callback_url": "string",
  "icon": "string",
  "name": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                     | Required | Description                       |
|--------|------|------------------------------------------------------------------------------------------|----------|-----------------------------------|
| `body` | body | [codersdk.PostOAuth2ProviderAppRequest](schemas.md#codersdkpostoauth2providerapprequest) | true     | The OAuth2 application to create. |

### Example responses

> 200 Response

```json
{
  "callback_url": "string",
  "endpoints": {
    "authorization": "string",
    "device_authorization": "string",
    "token": "string"
  },
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                             |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.OAuth2ProviderApp](schemas.md#codersdkoauth2providerapp) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get OAuth2 application

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/oauth2-provider/apps/{app} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /oauth2-provider/apps/{app}`

### Parameters

| Name  | In   | Type   | Required | Description |
|-------|------|--------|----------|-------------|
| `app` | path | string | true     | App ID      |

### Example responses

> 200 Response

```json
{
  "callback_url": "string",
  "endpoints": {
    "authorization": "string",
    "device_authorization": "string",
    "token": "string"
  },
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                             |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.OAuth2ProviderApp](schemas.md#codersdkoauth2providerapp) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update OAuth2 application

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/oauth2-provider/apps/{app} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /oauth2-provider/apps/{app}`

> Body parameter

```json
{
  "callback_url": "string",
  "icon": "string",
  "name": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                   | Required | Description                   |
|--------|------|----------------------------------------------------------------------------------------|----------|-------------------------------|
| `app`  | path | string                                                                                 | true     | App ID                        |
| `body` | body | [codersdk.PutOAuth2ProviderAppRequest](schemas.md#codersdkputoauth2providerapprequest) | true     | Update an OAuth2 application. |

### Example responses

> 200 Response

```json
{
  "callback_url": "string",
  "endpoints": {
    "authorization": "string",
    "device_authorization": "string",
    "token": "string"
  },
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                             |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.OAuth2ProviderApp](schemas.md#codersdkoauth2providerapp) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete OAuth2 application

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/oauth2-provider/apps/{app} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /oauth2-provider/apps/{app}`

### Parameters

| Name  | In   | Type   | Required | Description |
|-------|------|--------|----------|-------------|
| `app` | path | string | true     | App ID      |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get OAuth2 application secrets

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/oauth2-provider/apps/{app}/secrets \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /oauth2-provider/apps/{app}/secrets`

### Parameters

| Name  | In   | Type   | Required | Description |
|-------|------|--------|----------|-------------|
| `app` | path | string | true     | App ID      |

### Example responses

> 200 Response

```json
[
  {
    "client_secret_truncated": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "last_used_at": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                  |
|--------|---------------------------------------------------------|-------------|-----------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.OAuth2ProviderAppSecret](schemas.md#codersdkoauth2providerappsecret) |

<h3 id="get-oauth2-application-secrets.-responseschema">Response Schema</h3>

Status Code **200**

| Name                        | Type         | Required | Restrictions | Description |
|-----------------------------|--------------|----------|--------------|-------------|
| `[array item]`              | array        | false    |              |             |
| `» client_secret_truncated` | string       | false    |              |             |
| `» id`                      | string(uuid) | false    |              |             |
| `» last_used_at`            | string       | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create OAuth2 application secret

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/oauth2-provider/apps/{app}/secrets \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /oauth2-provider/apps/{app}/secrets`

### Parameters

| Name  | In   | Type   | Required | Description |
|-------|------|--------|----------|-------------|
| `app` | path | string | true     | App ID      |

### Example responses

> 200 Response

```json
[
  {
    "client_secret_full": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                          |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.OAuth2ProviderAppSecretFull](schemas.md#codersdkoauth2providerappsecretfull) |

<h3 id="create-oauth2-application-secret.-responseschema">Response Schema</h3>

Status Code **200**

| Name                   | Type         | Required | Restrictions | Description |
|------------------------|--------------|----------|--------------|-------------|
| `[array item]`         | array        | false    |              |             |
| `» client_secret_full` | string       | false    |              |             |
| `» id`                 | string(uuid) | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete OAuth2 application secret

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/oauth2-provider/apps/{app}/secrets/{secretID} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /oauth2-provider/apps/{app}/secrets/{secretID}`

### Parameters

| Name       | In   | Type   | Required | Description |
|------------|------|--------|----------|-------------|
| `app`      | path | string | true     | App ID      |
| `secretID` | path | string | true     | Secret ID   |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## OAuth2 authorization request

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/oauth2/authorize?client_id=string&state=string&response_type=code \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /oauth2/authorize`

### Parameters

| Name            | In    | Type   | Required | Description                       |
|-----------------|-------|--------|----------|-----------------------------------|
| `client_id`     | query | string | true     | Client ID                         |
| `state`         | query | string | true     | A random unguessable string       |
| `response_type` | query | string | true     | Response type                     |
| `redirect_uri`  | query | string | false    | Redirect here after authorization |
| `scope`         | query | string | false    | Token scopes (currently ignored)  |

#### Enumerated Values

| Parameter       | Value  |
|-----------------|--------|
| `response_type` | `code` |

### Responses

| Status | Meaning                                                    | Description | Schema |
|--------|------------------------------------------------------------|-------------|--------|
| 302    | [Found](https://tools.ietf.org/html/rfc7231#section-6.4.3) | Found       |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## OAuth2 token exchange

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/oauth2/tokens \
  -H 'Accept: application/json'
```

`POST /oauth2/tokens`

> Body parameter

```yaml
client_id: string
client_secret: string
code: string
refresh_token: string
grant_type: authorization_code

```

### Parameters

| Name              | In   | Type   | Required | Description                                                   |
|-------------------|------|--------|----------|---------------------------------------------------------------|
| `body`            | body | object | false    |                                                               |
| `» client_id`     | body | string | false    | Client ID, required if grant_type=authorization_code          |
| `» client_secret` | body | string | false    | Client secret, required if grant_type=authorization_code      |
| `» code`          | body | string | false    | Authorization code, required if grant_type=authorization_code |
| `» refresh_token` | body | string | false    | Refresh token, required if grant_type=refresh_token           |
| `» grant_type`    | body | string | true     | Grant type                                                    |

#### Enumerated Values

| Parameter      | Value                |
|----------------|----------------------|
| `» grant_type` | `authorization_code` |
| `» grant_type` | `refresh_token`      |

### Example responses

> 200 Response

```json
{
  "access_token": "string",
  "expires_in": 0,
  "expiry": "string",
  "refresh_token": "string",
  "token_type": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                 |
|--------|---------------------------------------------------------|-------------|----------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [oauth2.Token](schemas.md#oauth2token) |

## Delete OAuth2 application tokens

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/oauth2/tokens?client_id=string \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /oauth2/tokens`

### Parameters

| Name        | In    | Type   | Required | Description |
|-------------|-------|--------|----------|-------------|
| `client_id` | query | string | true     | Client ID   |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get groups by organization

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/groups \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/groups`

### Parameters

| Name           | In   | Type         | Required | Description     |
|----------------|------|--------------|----------|-----------------|
| `organization` | path | string(uuid) | true     | Organization ID |

### Example responses

> 200 Response

```json
[
  {
    "avatar_url": "string",
    "display_name": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "members": [
      {
        "avatar_url": "http://example.com",
        "created_at": "2019-08-24T14:15:22Z",
        "email": "user@example.com",
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "last_seen_at": "2019-08-24T14:15:22Z",
        "login_type": "",
        "name": "string",
        "status": "active",
        "theme_preference": "string",
        "updated_at": "2019-08-24T14:15:22Z",
        "username": "string"
      }
    ],
    "name": "string",
    "organization_display_name": "string",
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "organization_name": "string",
    "quota_allowance": 0,
    "source": "user",
    "total_member_count": 0
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                              |
|--------|---------------------------------------------------------|-------------|-----------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Group](schemas.md#codersdkgroup) |

<h3 id="get-groups-by-organization-responseschema">Response Schema</h3>

Status Code **200**

| Name                          | Type                                                   | Required | Restrictions | Description                                                                                                                                                           |
|-------------------------------|--------------------------------------------------------|----------|--------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `[array item]`                | array                                                  | false    |              |                                                                                                                                                                       |
| `» avatar_url`                | string                                                 | false    |              |                                                                                                                                                                       |
| `» display_name`              | string                                                 | false    |              |                                                                                                                                                                       |
| `» id`                        | string(uuid)                                           | false    |              |                                                                                                                                                                       |
| `» members`                   | array                                                  | false    |              |                                                                                                                                                                       |
| `»» avatar_url`               | string(uri)                                            | false    |              |                                                                                                                                                                       |
| `»» created_at`               | string(date-time)                                      | true     |              |                                                                                                                                                                       |
| `»» email`                    | string(email)                                          | true     |              |                                                                                                                                                                       |
| `»» id`                       | string(uuid)                                           | true     |              |                                                                                                                                                                       |
| `»» last_seen_at`             | string(date-time)                                      | false    |              |                                                                                                                                                                       |
| `»» login_type`               | [codersdk.LoginType](schemas.md#codersdklogintype)     | false    |              |                                                                                                                                                                       |
| `»» name`                     | string                                                 | false    |              |                                                                                                                                                                       |
| `»» status`                   | [codersdk.UserStatus](schemas.md#codersdkuserstatus)   | false    |              |                                                                                                                                                                       |
| `»» theme_preference`         | string                                                 | false    |              |                                                                                                                                                                       |
| `»» updated_at`               | string(date-time)                                      | false    |              |                                                                                                                                                                       |
| `»» username`                 | string                                                 | true     |              |                                                                                                                                                                       |
| `» name`                      | string                                                 | false    |              |                                                                                                                                                                       |
| `» organization_display_name` | string                                                 | false    |              |                                                                                                                                                                       |
| `» organization_id`           | string(uuid)                                           | false    |              |                                                                                                                                                                       |
| `» organization_name`         | string                                                 | false    |              |                                                                                                                                                                       |
| `» quota_allowance`           | integer                                                | false    |              |                                                                                                                                                                       |
| `» source`                    | [codersdk.GroupSource](schemas.md#codersdkgroupsource) | false    |              |                                                                                                                                                                       |
| `» total_member_count`        | integer                                                | false    |              | How many members are in this group. Shows the total count, even if the user is not authorized to read group member details. May be greater than `len(Group.Members)`. |

#### Enumerated Values

| Property     | Value       |
|--------------|-------------|
| `login_type` | ``          |
| `login_type` | `password`  |
| `login_type` | `github`    |
| `login_type` | `oidc`      |
| `login_type` | `token`     |
| `login_type` | `none`      |
| `status`     | `active`    |
| `status`     | `suspended` |
| `source`     | `user`      |
| `source`     | `oidc`      |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create group for organization

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/organizations/{organization}/groups \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /organizations/{organization}/groups`

> Body parameter

```json
{
  "avatar_url": "string",
  "display_name": "string",
  "name": "string",
  "quota_allowance": 0
}
```

### Parameters

| Name           | In   | Type                                                                 | Required | Description          |
|----------------|------|----------------------------------------------------------------------|----------|----------------------|
| `organization` | path | string                                                               | true     | Organization ID      |
| `body`         | body | [codersdk.CreateGroupRequest](schemas.md#codersdkcreategrouprequest) | true     | Create group request |

### Example responses

> 201 Response

```json
{
  "avatar_url": "string",
  "display_name": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "members": [
    {
      "avatar_url": "http://example.com",
      "created_at": "2019-08-24T14:15:22Z",
      "email": "user@example.com",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "last_seen_at": "2019-08-24T14:15:22Z",
      "login_type": "",
      "name": "string",
      "status": "active",
      "theme_preference": "string",
      "updated_at": "2019-08-24T14:15:22Z",
      "username": "string"
    }
  ],
  "name": "string",
  "organization_display_name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "organization_name": "string",
  "quota_allowance": 0,
  "source": "user",
  "total_member_count": 0
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                     |
|--------|--------------------------------------------------------------|-------------|--------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.Group](schemas.md#codersdkgroup) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get group by organization and group name

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/groups/{groupName} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/groups/{groupName}`

### Parameters

| Name           | In   | Type         | Required | Description     |
|----------------|------|--------------|----------|-----------------|
| `organization` | path | string(uuid) | true     | Organization ID |
| `groupName`    | path | string       | true     | Group name      |

### Example responses

> 200 Response

```json
{
  "avatar_url": "string",
  "display_name": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "members": [
    {
      "avatar_url": "http://example.com",
      "created_at": "2019-08-24T14:15:22Z",
      "email": "user@example.com",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "last_seen_at": "2019-08-24T14:15:22Z",
      "login_type": "",
      "name": "string",
      "status": "active",
      "theme_preference": "string",
      "updated_at": "2019-08-24T14:15:22Z",
      "username": "string"
    }
  ],
  "name": "string",
  "organization_display_name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "organization_name": "string",
  "quota_allowance": 0,
  "source": "user",
  "total_member_count": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                     |
|--------|---------------------------------------------------------|-------------|--------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Group](schemas.md#codersdkgroup) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get workspace quota by user

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/members/{user}/workspace-quota \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/members/{user}/workspace-quota`

### Parameters

| Name           | In   | Type         | Required | Description          |
|----------------|------|--------------|----------|----------------------|
| `user`         | path | string       | true     | User ID, name, or me |
| `organization` | path | string(uuid) | true     | Organization ID      |

### Example responses

> 200 Response

```json
{
  "budget": 0,
  "credits_consumed": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceQuota](schemas.md#codersdkworkspacequota) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Serve provisioner daemon

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/provisionerdaemons/serve \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/provisionerdaemons/serve`

### Parameters

| Name           | In   | Type         | Required | Description     |
|----------------|------|--------------|----------|-----------------|
| `organization` | path | string(uuid) | true     | Organization ID |

### Responses

| Status | Meaning                                                                  | Description         | Schema |
|--------|--------------------------------------------------------------------------|---------------------|--------|
| 101    | [Switching Protocols](https://tools.ietf.org/html/rfc7231#section-6.2.2) | Switching Protocols |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List provisioner key

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/provisionerkeys \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/provisionerkeys`

### Parameters

| Name           | In   | Type   | Required | Description     |
|----------------|------|--------|----------|-----------------|
| `organization` | path | string | true     | Organization ID |

### Example responses

> 200 Response

```json
[
  {
    "created_at": "2019-08-24T14:15:22Z",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "name": "string",
    "organization": "452c1a86-a0af-475b-b03f-724878b0f387",
    "tags": {
      "property1": "string",
      "property2": "string"
    }
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                |
|--------|---------------------------------------------------------|-------------|-----------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.ProvisionerKey](schemas.md#codersdkprovisionerkey) |

<h3 id="list-provisioner-key-responseschema">Response Schema</h3>

Status Code **200**

| Name                | Type                                                                 | Required | Restrictions | Description |
|---------------------|----------------------------------------------------------------------|----------|--------------|-------------|
| `[array item]`      | array                                                                | false    |              |             |
| `» created_at`      | string(date-time)                                                    | false    |              |             |
| `» id`              | string(uuid)                                                         | false    |              |             |
| `» name`            | string                                                               | false    |              |             |
| `» organization`    | string(uuid)                                                         | false    |              |             |
| `» tags`            | [codersdk.ProvisionerKeyTags](schemas.md#codersdkprovisionerkeytags) | false    |              |             |
| `»» [any property]` | string                                                               | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create provisioner key

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/organizations/{organization}/provisionerkeys \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /organizations/{organization}/provisionerkeys`

### Parameters

| Name           | In   | Type   | Required | Description     |
|----------------|------|--------|----------|-----------------|
| `organization` | path | string | true     | Organization ID |

### Example responses

> 201 Response

```json
{
  "key": "string"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                                                   |
|--------|--------------------------------------------------------------|-------------|------------------------------------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.CreateProvisionerKeyResponse](schemas.md#codersdkcreateprovisionerkeyresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List provisioner key daemons

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/provisionerkeys/daemons \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/provisionerkeys/daemons`

### Parameters

| Name           | In   | Type   | Required | Description     |
|----------------|------|--------|----------|-----------------|
| `organization` | path | string | true     | Organization ID |

### Example responses

> 200 Response

```json
[
  {
    "daemons": [
      {
        "api_version": "string",
        "created_at": "2019-08-24T14:15:22Z",
        "current_job": {
          "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
          "status": "pending",
          "template_display_name": "string",
          "template_icon": "string",
          "template_name": "string"
        },
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "key_id": "1e779c8a-6786-4c89-b7c3-a6666f5fd6b5",
        "key_name": "string",
        "last_seen_at": "2019-08-24T14:15:22Z",
        "name": "string",
        "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
        "previous_job": {
          "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
          "status": "pending",
          "template_display_name": "string",
          "template_icon": "string",
          "template_name": "string"
        },
        "provisioners": [
          "string"
        ],
        "status": "offline",
        "tags": {
          "property1": "string",
          "property2": "string"
        },
        "version": "string"
      }
    ],
    "key": {
      "created_at": "2019-08-24T14:15:22Z",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "name": "string",
      "organization": "452c1a86-a0af-475b-b03f-724878b0f387",
      "tags": {
        "property1": "string",
        "property2": "string"
      }
    }
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                              |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.ProvisionerKeyDaemons](schemas.md#codersdkprovisionerkeydaemons) |

<h3 id="list-provisioner-key-daemons-responseschema">Response Schema</h3>

Status Code **200**

| Name                        | Type                                                                           | Required | Restrictions | Description      |
|-----------------------------|--------------------------------------------------------------------------------|----------|--------------|------------------|
| `[array item]`              | array                                                                          | false    |              |                  |
| `» daemons`                 | array                                                                          | false    |              |                  |
| `»» api_version`            | string                                                                         | false    |              |                  |
| `»» created_at`             | string(date-time)                                                              | false    |              |                  |
| `»» current_job`            | [codersdk.ProvisionerDaemonJob](schemas.md#codersdkprovisionerdaemonjob)       | false    |              |                  |
| `»»» id`                    | string(uuid)                                                                   | false    |              |                  |
| `»»» status`                | [codersdk.ProvisionerJobStatus](schemas.md#codersdkprovisionerjobstatus)       | false    |              |                  |
| `»»» template_display_name` | string                                                                         | false    |              |                  |
| `»»» template_icon`         | string                                                                         | false    |              |                  |
| `»»» template_name`         | string                                                                         | false    |              |                  |
| `»» id`                     | string(uuid)                                                                   | false    |              |                  |
| `»» key_id`                 | string(uuid)                                                                   | false    |              |                  |
| `»» key_name`               | string                                                                         | false    |              | Optional fields. |
| `»» last_seen_at`           | string(date-time)                                                              | false    |              |                  |
| `»» name`                   | string                                                                         | false    |              |                  |
| `»» organization_id`        | string(uuid)                                                                   | false    |              |                  |
| `»» previous_job`           | [codersdk.ProvisionerDaemonJob](schemas.md#codersdkprovisionerdaemonjob)       | false    |              |                  |
| `»» provisioners`           | array                                                                          | false    |              |                  |
| `»» status`                 | [codersdk.ProvisionerDaemonStatus](schemas.md#codersdkprovisionerdaemonstatus) | false    |              |                  |
| `»» tags`                   | object                                                                         | false    |              |                  |
| `»»» [any property]`        | string                                                                         | false    |              |                  |
| `»» version`                | string                                                                         | false    |              |                  |
| `» key`                     | [codersdk.ProvisionerKey](schemas.md#codersdkprovisionerkey)                   | false    |              |                  |
| `»» created_at`             | string(date-time)                                                              | false    |              |                  |
| `»» id`                     | string(uuid)                                                                   | false    |              |                  |
| `»» name`                   | string                                                                         | false    |              |                  |
| `»» organization`           | string(uuid)                                                                   | false    |              |                  |
| `»» tags`                   | [codersdk.ProvisionerKeyTags](schemas.md#codersdkprovisionerkeytags)           | false    |              |                  |
| `»»» [any property]`        | string                                                                         | false    |              |                  |

#### Enumerated Values

| Property | Value       |
|----------|-------------|
| `status` | `pending`   |
| `status` | `running`   |
| `status` | `succeeded` |
| `status` | `canceling` |
| `status` | `canceled`  |
| `status` | `failed`    |
| `status` | `offline`   |
| `status` | `idle`      |
| `status` | `busy`      |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete provisioner key

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/organizations/{organization}/provisionerkeys/{provisionerkey} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /organizations/{organization}/provisionerkeys/{provisionerkey}`

### Parameters

| Name             | In   | Type   | Required | Description          |
|------------------|------|--------|----------|----------------------|
| `organization`   | path | string | true     | Organization ID      |
| `provisionerkey` | path | string | true     | Provisioner key name |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get the available organization idp sync claim fields

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/settings/idpsync/available-fields \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/settings/idpsync/available-fields`

### Parameters

| Name           | In   | Type         | Required | Description     |
|----------------|------|--------------|----------|-----------------|
| `organization` | path | string(uuid) | true     | Organization ID |

### Example responses

> 200 Response

```json
[
  "string"
]
```

### Responses

| Status | Meaning                                                 | Description | Schema          |
|--------|---------------------------------------------------------|-------------|-----------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of string |

<h3 id="get-the-available-organization-idp-sync-claim-fields-responseschema">Response Schema</h3>

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get the organization idp sync claim field values

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/settings/idpsync/field-values?claimField=string \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/settings/idpsync/field-values`

### Parameters

| Name           | In    | Type           | Required | Description     |
|----------------|-------|----------------|----------|-----------------|
| `organization` | path  | string(uuid)   | true     | Organization ID |
| `claimField`   | query | string(string) | true     | Claim Field     |

### Example responses

> 200 Response

```json
[
  "string"
]
```

### Responses

| Status | Meaning                                                 | Description | Schema          |
|--------|---------------------------------------------------------|-------------|-----------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of string |

<h3 id="get-the-organization-idp-sync-claim-field-values-responseschema">Response Schema</h3>

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get group IdP Sync settings by organization

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/settings/idpsync/groups \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/settings/idpsync/groups`

### Parameters

| Name           | In   | Type         | Required | Description     |
|----------------|------|--------------|----------|-----------------|
| `organization` | path | string(uuid) | true     | Organization ID |

### Example responses

> 200 Response

```json
{
  "auto_create_missing_groups": true,
  "field": "string",
  "legacy_group_name_mapping": {
    "property1": "string",
    "property2": "string"
  },
  "mapping": {
    "property1": [
      "string"
    ],
    "property2": [
      "string"
    ]
  },
  "regex_filter": {}
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                             |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.GroupSyncSettings](schemas.md#codersdkgroupsyncsettings) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update group IdP Sync settings by organization

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/organizations/{organization}/settings/idpsync/groups \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /organizations/{organization}/settings/idpsync/groups`

> Body parameter

```json
{
  "auto_create_missing_groups": true,
  "field": "string",
  "legacy_group_name_mapping": {
    "property1": "string",
    "property2": "string"
  },
  "mapping": {
    "property1": [
      "string"
    ],
    "property2": [
      "string"
    ]
  },
  "regex_filter": {}
}
```

### Parameters

| Name           | In   | Type                                                               | Required | Description     |
|----------------|------|--------------------------------------------------------------------|----------|-----------------|
| `organization` | path | string(uuid)                                                       | true     | Organization ID |
| `body`         | body | [codersdk.GroupSyncSettings](schemas.md#codersdkgroupsyncsettings) | true     | New settings    |

### Example responses

> 200 Response

```json
{
  "auto_create_missing_groups": true,
  "field": "string",
  "legacy_group_name_mapping": {
    "property1": "string",
    "property2": "string"
  },
  "mapping": {
    "property1": [
      "string"
    ],
    "property2": [
      "string"
    ]
  },
  "regex_filter": {}
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                             |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.GroupSyncSettings](schemas.md#codersdkgroupsyncsettings) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update group IdP Sync config

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/organizations/{organization}/settings/idpsync/groups/config \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /organizations/{organization}/settings/idpsync/groups/config`

> Body parameter

```json
{
  "auto_create_missing_groups": true,
  "field": "string",
  "regex_filter": {}
}
```

### Parameters

| Name           | In   | Type                                                                                         | Required | Description             |
|----------------|------|----------------------------------------------------------------------------------------------|----------|-------------------------|
| `organization` | path | string(uuid)                                                                                 | true     | Organization ID or name |
| `body`         | body | [codersdk.PatchGroupIDPSyncConfigRequest](schemas.md#codersdkpatchgroupidpsyncconfigrequest) | true     | New config values       |

### Example responses

> 200 Response

```json
{
  "auto_create_missing_groups": true,
  "field": "string",
  "legacy_group_name_mapping": {
    "property1": "string",
    "property2": "string"
  },
  "mapping": {
    "property1": [
      "string"
    ],
    "property2": [
      "string"
    ]
  },
  "regex_filter": {}
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                             |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.GroupSyncSettings](schemas.md#codersdkgroupsyncsettings) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update group IdP Sync mapping

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/organizations/{organization}/settings/idpsync/groups/mapping \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /organizations/{organization}/settings/idpsync/groups/mapping`

> Body parameter

```json
{
  "add": [
    {
      "gets": "string",
      "given": "string"
    }
  ],
  "remove": [
    {
      "gets": "string",
      "given": "string"
    }
  ]
}
```

### Parameters

| Name           | In   | Type                                                                                           | Required | Description                                   |
|----------------|------|------------------------------------------------------------------------------------------------|----------|-----------------------------------------------|
| `organization` | path | string(uuid)                                                                                   | true     | Organization ID or name                       |
| `body`         | body | [codersdk.PatchGroupIDPSyncMappingRequest](schemas.md#codersdkpatchgroupidpsyncmappingrequest) | true     | Description of the mappings to add and remove |

### Example responses

> 200 Response

```json
{
  "auto_create_missing_groups": true,
  "field": "string",
  "legacy_group_name_mapping": {
    "property1": "string",
    "property2": "string"
  },
  "mapping": {
    "property1": [
      "string"
    ],
    "property2": [
      "string"
    ]
  },
  "regex_filter": {}
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                             |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.GroupSyncSettings](schemas.md#codersdkgroupsyncsettings) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get role IdP Sync settings by organization

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/settings/idpsync/roles \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/settings/idpsync/roles`

### Parameters

| Name           | In   | Type         | Required | Description     |
|----------------|------|--------------|----------|-----------------|
| `organization` | path | string(uuid) | true     | Organization ID |

### Example responses

> 200 Response

```json
{
  "field": "string",
  "mapping": {
    "property1": [
      "string"
    ],
    "property2": [
      "string"
    ]
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                           |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.RoleSyncSettings](schemas.md#codersdkrolesyncsettings) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update role IdP Sync settings by organization

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/organizations/{organization}/settings/idpsync/roles \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /organizations/{organization}/settings/idpsync/roles`

> Body parameter

```json
{
  "field": "string",
  "mapping": {
    "property1": [
      "string"
    ],
    "property2": [
      "string"
    ]
  }
}
```

### Parameters

| Name           | In   | Type                                                             | Required | Description     |
|----------------|------|------------------------------------------------------------------|----------|-----------------|
| `organization` | path | string(uuid)                                                     | true     | Organization ID |
| `body`         | body | [codersdk.RoleSyncSettings](schemas.md#codersdkrolesyncsettings) | true     | New settings    |

### Example responses

> 200 Response

```json
{
  "field": "string",
  "mapping": {
    "property1": [
      "string"
    ],
    "property2": [
      "string"
    ]
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                           |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.RoleSyncSettings](schemas.md#codersdkrolesyncsettings) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update role IdP Sync config

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/organizations/{organization}/settings/idpsync/roles/config \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /organizations/{organization}/settings/idpsync/roles/config`

> Body parameter

```json
{
  "field": "string"
}
```

### Parameters

| Name           | In   | Type                                                                                       | Required | Description             |
|----------------|------|--------------------------------------------------------------------------------------------|----------|-------------------------|
| `organization` | path | string(uuid)                                                                               | true     | Organization ID or name |
| `body`         | body | [codersdk.PatchRoleIDPSyncConfigRequest](schemas.md#codersdkpatchroleidpsyncconfigrequest) | true     | New config values       |

### Example responses

> 200 Response

```json
{
  "field": "string",
  "mapping": {
    "property1": [
      "string"
    ],
    "property2": [
      "string"
    ]
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                           |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.RoleSyncSettings](schemas.md#codersdkrolesyncsettings) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update role IdP Sync mapping

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/organizations/{organization}/settings/idpsync/roles/mapping \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /organizations/{organization}/settings/idpsync/roles/mapping`

> Body parameter

```json
{
  "add": [
    {
      "gets": "string",
      "given": "string"
    }
  ],
  "remove": [
    {
      "gets": "string",
      "given": "string"
    }
  ]
}
```

### Parameters

| Name           | In   | Type                                                                                         | Required | Description                                   |
|----------------|------|----------------------------------------------------------------------------------------------|----------|-----------------------------------------------|
| `organization` | path | string(uuid)                                                                                 | true     | Organization ID or name                       |
| `body`         | body | [codersdk.PatchRoleIDPSyncMappingRequest](schemas.md#codersdkpatchroleidpsyncmappingrequest) | true     | Description of the mappings to add and remove |

### Example responses

> 200 Response

```json
{
  "field": "string",
  "mapping": {
    "property1": [
      "string"
    ],
    "property2": [
      "string"
    ]
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                           |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.RoleSyncSettings](schemas.md#codersdkrolesyncsettings) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Fetch provisioner key details

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/provisionerkeys/{provisionerkey} \
  -H 'Accept: application/json'
```

`GET /provisionerkeys/{provisionerkey}`

### Parameters

| Name             | In   | Type   | Required | Description     |
|------------------|------|--------|----------|-----------------|
| `provisionerkey` | path | string | true     | Provisioner Key |

### Example responses

> 200 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "organization": "452c1a86-a0af-475b-b03f-724878b0f387",
  "tags": {
    "property1": "string",
    "property2": "string"
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ProvisionerKey](schemas.md#codersdkprovisionerkey) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get active replicas

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/replicas \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /replicas`

### Example responses

> 200 Response

```json
[
  {
    "created_at": "2019-08-24T14:15:22Z",
    "database_latency": 0,
    "error": "string",
    "hostname": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "region_id": 0,
    "relay_address": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                  |
|--------|---------------------------------------------------------|-------------|---------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Replica](schemas.md#codersdkreplica) |

<h3 id="get-active-replicas-responseschema">Response Schema</h3>

Status Code **200**

| Name                 | Type              | Required | Restrictions | Description                                                        |
|----------------------|-------------------|----------|--------------|--------------------------------------------------------------------|
| `[array item]`       | array             | false    |              |                                                                    |
| `» created_at`       | string(date-time) | false    |              | Created at is the timestamp when the replica was first seen.       |
| `» database_latency` | integer           | false    |              | Database latency is the latency in microseconds to the database.   |
| `» error`            | string            | false    |              | Error is the replica error.                                        |
| `» hostname`         | string            | false    |              | Hostname is the hostname of the replica.                           |
| `» id`               | string(uuid)      | false    |              | ID is the unique identifier for the replica.                       |
| `» region_id`        | integer           | false    |              | Region ID is the region of the replica.                            |
| `» relay_address`    | string            | false    |              | Relay address is the accessible address to relay DERP connections. |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## SCIM 2.0: Service Provider Config

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/scim/v2/ServiceProviderConfig

```

`GET /scim/v2/ServiceProviderConfig`

### Responses

| Status | Meaning                                                 | Description | Schema |
|--------|---------------------------------------------------------|-------------|--------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

## SCIM 2.0: Get users

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/scim/v2/Users \
  -H 'Authorizaiton: API_KEY'
```

`GET /scim/v2/Users`

### Responses

| Status | Meaning                                                 | Description | Schema |
|--------|---------------------------------------------------------|-------------|--------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## SCIM 2.0: Create new user

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/scim/v2/Users \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Authorizaiton: API_KEY'
```

`POST /scim/v2/Users`

> Body parameter

```json
{
  "active": true,
  "emails": [
    {
      "display": "string",
      "primary": true,
      "type": "string",
      "value": "user@example.com"
    }
  ],
  "groups": [
    null
  ],
  "id": "string",
  "meta": {
    "resourceType": "string"
  },
  "name": {
    "familyName": "string",
    "givenName": "string"
  },
  "schemas": [
    "string"
  ],
  "userName": "string"
}
```

### Parameters

| Name   | In   | Type                                         | Required | Description |
|--------|------|----------------------------------------------|----------|-------------|
| `body` | body | [coderd.SCIMUser](schemas.md#coderdscimuser) | true     | New user    |

### Example responses

> 200 Response

```json
{
  "active": true,
  "emails": [
    {
      "display": "string",
      "primary": true,
      "type": "string",
      "value": "user@example.com"
    }
  ],
  "groups": [
    null
  ],
  "id": "string",
  "meta": {
    "resourceType": "string"
  },
  "name": {
    "familyName": "string",
    "givenName": "string"
  },
  "schemas": [
    "string"
  ],
  "userName": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                       |
|--------|---------------------------------------------------------|-------------|----------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [coderd.SCIMUser](schemas.md#coderdscimuser) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## SCIM 2.0: Get user by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/scim/v2/Users/{id} \
  -H 'Authorizaiton: API_KEY'
```

`GET /scim/v2/Users/{id}`

### Parameters

| Name | In   | Type         | Required | Description |
|------|------|--------------|----------|-------------|
| `id` | path | string(uuid) | true     | User ID     |

### Responses

| Status | Meaning                                                        | Description | Schema |
|--------|----------------------------------------------------------------|-------------|--------|
| 404    | [Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4) | Not Found   |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## SCIM 2.0: Replace user account

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/scim/v2/Users/{id} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/scim+json' \
  -H 'Authorizaiton: API_KEY'
```

`PUT /scim/v2/Users/{id}`

> Body parameter

```json
{
  "active": true,
  "emails": [
    {
      "display": "string",
      "primary": true,
      "type": "string",
      "value": "user@example.com"
    }
  ],
  "groups": [
    null
  ],
  "id": "string",
  "meta": {
    "resourceType": "string"
  },
  "name": {
    "familyName": "string",
    "givenName": "string"
  },
  "schemas": [
    "string"
  ],
  "userName": "string"
}
```

### Parameters

| Name   | In   | Type                                         | Required | Description          |
|--------|------|----------------------------------------------|----------|----------------------|
| `id`   | path | string(uuid)                                 | true     | User ID              |
| `body` | body | [coderd.SCIMUser](schemas.md#coderdscimuser) | true     | Replace user request |

### Example responses

> 200 Response

```json
{
  "avatar_url": "http://example.com",
  "created_at": "2019-08-24T14:15:22Z",
  "email": "user@example.com",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "last_seen_at": "2019-08-24T14:15:22Z",
  "login_type": "",
  "name": "string",
  "organization_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "roles": [
    {
      "display_name": "string",
      "name": "string",
      "organization_id": "string"
    }
  ],
  "status": "active",
  "theme_preference": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "username": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.User](schemas.md#codersdkuser) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## SCIM 2.0: Update user account

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/scim/v2/Users/{id} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/scim+json' \
  -H 'Authorizaiton: API_KEY'
```

`PATCH /scim/v2/Users/{id}`

> Body parameter

```json
{
  "active": true,
  "emails": [
    {
      "display": "string",
      "primary": true,
      "type": "string",
      "value": "user@example.com"
    }
  ],
  "groups": [
    null
  ],
  "id": "string",
  "meta": {
    "resourceType": "string"
  },
  "name": {
    "familyName": "string",
    "givenName": "string"
  },
  "schemas": [
    "string"
  ],
  "userName": "string"
}
```

### Parameters

| Name   | In   | Type                                         | Required | Description         |
|--------|------|----------------------------------------------|----------|---------------------|
| `id`   | path | string(uuid)                                 | true     | User ID             |
| `body` | body | [coderd.SCIMUser](schemas.md#coderdscimuser) | true     | Update user request |

### Example responses

> 200 Response

```json
{
  "avatar_url": "http://example.com",
  "created_at": "2019-08-24T14:15:22Z",
  "email": "user@example.com",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "last_seen_at": "2019-08-24T14:15:22Z",
  "login_type": "",
  "name": "string",
  "organization_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "roles": [
    {
      "display_name": "string",
      "name": "string",
      "organization_id": "string"
    }
  ],
  "status": "active",
  "theme_preference": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "username": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.User](schemas.md#codersdkuser) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get the available idp sync claim fields

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/settings/idpsync/available-fields \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /settings/idpsync/available-fields`

### Parameters

| Name           | In   | Type         | Required | Description     |
|----------------|------|--------------|----------|-----------------|
| `organization` | path | string(uuid) | true     | Organization ID |

### Example responses

> 200 Response

```json
[
  "string"
]
```

### Responses

| Status | Meaning                                                 | Description | Schema          |
|--------|---------------------------------------------------------|-------------|-----------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of string |

<h3 id="get-the-available-idp-sync-claim-fields-responseschema">Response Schema</h3>

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get the idp sync claim field values

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/settings/idpsync/field-values?claimField=string \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /settings/idpsync/field-values`

### Parameters

| Name           | In    | Type           | Required | Description     |
|----------------|-------|----------------|----------|-----------------|
| `organization` | path  | string(uuid)   | true     | Organization ID |
| `claimField`   | query | string(string) | true     | Claim Field     |

### Example responses

> 200 Response

```json
[
  "string"
]
```

### Responses

| Status | Meaning                                                 | Description | Schema          |
|--------|---------------------------------------------------------|-------------|-----------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of string |

<h3 id="get-the-idp-sync-claim-field-values-responseschema">Response Schema</h3>

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get organization IdP Sync settings

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/settings/idpsync/organization \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /settings/idpsync/organization`

### Example responses

> 200 Response

```json
{
  "field": "string",
  "mapping": {
    "property1": [
      "string"
    ],
    "property2": [
      "string"
    ]
  },
  "organization_assign_default": true
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                           |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.OrganizationSyncSettings](schemas.md#codersdkorganizationsyncsettings) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update organization IdP Sync settings

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/settings/idpsync/organization \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /settings/idpsync/organization`

> Body parameter

```json
{
  "field": "string",
  "mapping": {
    "property1": [
      "string"
    ],
    "property2": [
      "string"
    ]
  },
  "organization_assign_default": true
}
```

### Parameters

| Name   | In   | Type                                                                             | Required | Description  |
|--------|------|----------------------------------------------------------------------------------|----------|--------------|
| `body` | body | [codersdk.OrganizationSyncSettings](schemas.md#codersdkorganizationsyncsettings) | true     | New settings |

### Example responses

> 200 Response

```json
{
  "field": "string",
  "mapping": {
    "property1": [
      "string"
    ],
    "property2": [
      "string"
    ]
  },
  "organization_assign_default": true
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                           |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.OrganizationSyncSettings](schemas.md#codersdkorganizationsyncsettings) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update organization IdP Sync config

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/settings/idpsync/organization/config \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /settings/idpsync/organization/config`

> Body parameter

```json
{
  "assign_default": true,
  "field": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                                       | Required | Description       |
|--------|------|------------------------------------------------------------------------------------------------------------|----------|-------------------|
| `body` | body | [codersdk.PatchOrganizationIDPSyncConfigRequest](schemas.md#codersdkpatchorganizationidpsyncconfigrequest) | true     | New config values |

### Example responses

> 200 Response

```json
{
  "field": "string",
  "mapping": {
    "property1": [
      "string"
    ],
    "property2": [
      "string"
    ]
  },
  "organization_assign_default": true
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                           |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.OrganizationSyncSettings](schemas.md#codersdkorganizationsyncsettings) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update organization IdP Sync mapping

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/settings/idpsync/organization/mapping \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /settings/idpsync/organization/mapping`

> Body parameter

```json
{
  "add": [
    {
      "gets": "string",
      "given": "string"
    }
  ],
  "remove": [
    {
      "gets": "string",
      "given": "string"
    }
  ]
}
```

### Parameters

| Name   | In   | Type                                                                                                         | Required | Description                                   |
|--------|------|--------------------------------------------------------------------------------------------------------------|----------|-----------------------------------------------|
| `body` | body | [codersdk.PatchOrganizationIDPSyncMappingRequest](schemas.md#codersdkpatchorganizationidpsyncmappingrequest) | true     | Description of the mappings to add and remove |

### Example responses

> 200 Response

```json
{
  "field": "string",
  "mapping": {
    "property1": [
      "string"
    ],
    "property2": [
      "string"
    ]
  },
  "organization_assign_default": true
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                           |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.OrganizationSyncSettings](schemas.md#codersdkorganizationsyncsettings) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get template ACLs

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templates/{template}/acl \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templates/{template}/acl`

### Parameters

| Name       | In   | Type         | Required | Description |
|------------|------|--------------|----------|-------------|
| `template` | path | string(uuid) | true     | Template ID |

### Example responses

> 200 Response

```json
[
  {
    "avatar_url": "http://example.com",
    "created_at": "2019-08-24T14:15:22Z",
    "email": "user@example.com",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "last_seen_at": "2019-08-24T14:15:22Z",
    "login_type": "",
    "name": "string",
    "organization_ids": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "role": "admin",
    "roles": [
      {
        "display_name": "string",
        "name": "string",
        "organization_id": "string"
      }
    ],
    "status": "active",
    "theme_preference": "string",
    "updated_at": "2019-08-24T14:15:22Z",
    "username": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                            |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.TemplateUser](schemas.md#codersdktemplateuser) |

<h3 id="get-template-acls-responseschema">Response Schema</h3>

Status Code **200**

| Name                 | Type                                                     | Required | Restrictions | Description |
|----------------------|----------------------------------------------------------|----------|--------------|-------------|
| `[array item]`       | array                                                    | false    |              |             |
| `» avatar_url`       | string(uri)                                              | false    |              |             |
| `» created_at`       | string(date-time)                                        | true     |              |             |
| `» email`            | string(email)                                            | true     |              |             |
| `» id`               | string(uuid)                                             | true     |              |             |
| `» last_seen_at`     | string(date-time)                                        | false    |              |             |
| `» login_type`       | [codersdk.LoginType](schemas.md#codersdklogintype)       | false    |              |             |
| `» name`             | string                                                   | false    |              |             |
| `» organization_ids` | array                                                    | false    |              |             |
| `» role`             | [codersdk.TemplateRole](schemas.md#codersdktemplaterole) | false    |              |             |
| `» roles`            | array                                                    | false    |              |             |
| `»» display_name`    | string                                                   | false    |              |             |
| `»» name`            | string                                                   | false    |              |             |
| `»» organization_id` | string                                                   | false    |              |             |
| `» status`           | [codersdk.UserStatus](schemas.md#codersdkuserstatus)     | false    |              |             |
| `» theme_preference` | string                                                   | false    |              |             |
| `» updated_at`       | string(date-time)                                        | false    |              |             |
| `» username`         | string                                                   | true     |              |             |

#### Enumerated Values

| Property     | Value       |
|--------------|-------------|
| `login_type` | ``          |
| `login_type` | `password`  |
| `login_type` | `github`    |
| `login_type` | `oidc`      |
| `login_type` | `token`     |
| `login_type` | `none`      |
| `role`       | `admin`     |
| `role`       | `use`       |
| `status`     | `active`    |
| `status`     | `suspended` |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update template ACL

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/templates/{template}/acl \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /templates/{template}/acl`

> Body parameter

```json
{
  "group_perms": {
    "8bd26b20-f3e8-48be-a903-46bb920cf671": "use",
    "<user_id>>": "admin"
  },
  "user_perms": {
    "4df59e74-c027-470b-ab4d-cbba8963a5e9": "use",
    "<group_id>": "admin"
  }
}
```

### Parameters

| Name       | In   | Type                                                               | Required | Description             |
|------------|------|--------------------------------------------------------------------|----------|-------------------------|
| `template` | path | string(uuid)                                                       | true     | Template ID             |
| `body`     | body | [codersdk.UpdateTemplateACL](schemas.md#codersdkupdatetemplateacl) | true     | Update template request |

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

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get template available acl users/groups

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/templates/{template}/acl/available \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /templates/{template}/acl/available`

### Parameters

| Name       | In   | Type         | Required | Description |
|------------|------|--------------|----------|-------------|
| `template` | path | string(uuid) | true     | Template ID |

### Example responses

> 200 Response

```json
[
  {
    "groups": [
      {
        "avatar_url": "string",
        "display_name": "string",
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "members": [
          {
            "avatar_url": "http://example.com",
            "created_at": "2019-08-24T14:15:22Z",
            "email": "user@example.com",
            "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
            "last_seen_at": "2019-08-24T14:15:22Z",
            "login_type": "",
            "name": "string",
            "status": "active",
            "theme_preference": "string",
            "updated_at": "2019-08-24T14:15:22Z",
            "username": "string"
          }
        ],
        "name": "string",
        "organization_display_name": "string",
        "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
        "organization_name": "string",
        "quota_allowance": 0,
        "source": "user",
        "total_member_count": 0
      }
    ],
    "users": [
      {
        "avatar_url": "http://example.com",
        "created_at": "2019-08-24T14:15:22Z",
        "email": "user@example.com",
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "last_seen_at": "2019-08-24T14:15:22Z",
        "login_type": "",
        "name": "string",
        "status": "active",
        "theme_preference": "string",
        "updated_at": "2019-08-24T14:15:22Z",
        "username": "string"
      }
    ]
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                            |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.ACLAvailable](schemas.md#codersdkaclavailable) |

<h3 id="get-template-available-acl-users/groups-responseschema">Response Schema</h3>

Status Code **200**

| Name                           | Type                                                   | Required | Restrictions | Description                                                                                                                                                           |
|--------------------------------|--------------------------------------------------------|----------|--------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `[array item]`                 | array                                                  | false    |              |                                                                                                                                                                       |
| `» groups`                     | array                                                  | false    |              |                                                                                                                                                                       |
| `»» avatar_url`                | string                                                 | false    |              |                                                                                                                                                                       |
| `»» display_name`              | string                                                 | false    |              |                                                                                                                                                                       |
| `»» id`                        | string(uuid)                                           | false    |              |                                                                                                                                                                       |
| `»» members`                   | array                                                  | false    |              |                                                                                                                                                                       |
| `»»» avatar_url`               | string(uri)                                            | false    |              |                                                                                                                                                                       |
| `»»» created_at`               | string(date-time)                                      | true     |              |                                                                                                                                                                       |
| `»»» email`                    | string(email)                                          | true     |              |                                                                                                                                                                       |
| `»»» id`                       | string(uuid)                                           | true     |              |                                                                                                                                                                       |
| `»»» last_seen_at`             | string(date-time)                                      | false    |              |                                                                                                                                                                       |
| `»»» login_type`               | [codersdk.LoginType](schemas.md#codersdklogintype)     | false    |              |                                                                                                                                                                       |
| `»»» name`                     | string                                                 | false    |              |                                                                                                                                                                       |
| `»»» status`                   | [codersdk.UserStatus](schemas.md#codersdkuserstatus)   | false    |              |                                                                                                                                                                       |
| `»»» theme_preference`         | string                                                 | false    |              |                                                                                                                                                                       |
| `»»» updated_at`               | string(date-time)                                      | false    |              |                                                                                                                                                                       |
| `»»» username`                 | string                                                 | true     |              |                                                                                                                                                                       |
| `»» name`                      | string                                                 | false    |              |                                                                                                                                                                       |
| `»» organization_display_name` | string                                                 | false    |              |                                                                                                                                                                       |
| `»» organization_id`           | string(uuid)                                           | false    |              |                                                                                                                                                                       |
| `»» organization_name`         | string                                                 | false    |              |                                                                                                                                                                       |
| `»» quota_allowance`           | integer                                                | false    |              |                                                                                                                                                                       |
| `»» source`                    | [codersdk.GroupSource](schemas.md#codersdkgroupsource) | false    |              |                                                                                                                                                                       |
| `»» total_member_count`        | integer                                                | false    |              | How many members are in this group. Shows the total count, even if the user is not authorized to read group member details. May be greater than `len(Group.Members)`. |
| `» users`                      | array                                                  | false    |              |                                                                                                                                                                       |

#### Enumerated Values

| Property     | Value       |
|--------------|-------------|
| `login_type` | ``          |
| `login_type` | `password`  |
| `login_type` | `github`    |
| `login_type` | `oidc`      |
| `login_type` | `token`     |
| `login_type` | `none`      |
| `status`     | `active`    |
| `status`     | `suspended` |
| `source`     | `user`      |
| `source`     | `oidc`      |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get user quiet hours schedule

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/{user}/quiet-hours \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/{user}/quiet-hours`

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `user` | path | string(uuid) | true     | User ID     |

### Example responses

> 200 Response

```json
[
  {
    "next": "2019-08-24T14:15:22Z",
    "raw_schedule": "string",
    "time": "string",
    "timezone": "string",
    "user_can_set": true,
    "user_set": true
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                                |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.UserQuietHoursScheduleResponse](schemas.md#codersdkuserquiethoursscheduleresponse) |

<h3 id="get-user-quiet-hours-schedule-responseschema">Response Schema</h3>

Status Code **200**

| Name             | Type              | Required | Restrictions | Description                                                                                                                                                                      |
|------------------|-------------------|----------|--------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `[array item]`   | array             | false    |              |                                                                                                                                                                                  |
| `» next`         | string(date-time) | false    |              | Next is the next time that the quiet hours window will start.                                                                                                                    |
| `» raw_schedule` | string            | false    |              |                                                                                                                                                                                  |
| `» time`         | string            | false    |              | Time is the time of day that the quiet hours window starts in the given Timezone each day.                                                                                       |
| `» timezone`     | string            | false    |              | raw format from the cron expression, UTC if unspecified                                                                                                                          |
| `» user_can_set` | boolean           | false    |              | User can set is true if the user is allowed to set their own quiet hours schedule. If false, the user cannot set a custom schedule and the default schedule will always be used. |
| `» user_set`     | boolean           | false    |              | User set is true if the user has set their own quiet hours schedule. If false, the user is using the default schedule.                                                           |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update user quiet hours schedule

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/users/{user}/quiet-hours \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /users/{user}/quiet-hours`

> Body parameter

```json
{
  "schedule": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                                   | Required | Description             |
|--------|------|--------------------------------------------------------------------------------------------------------|----------|-------------------------|
| `user` | path | string(uuid)                                                                                           | true     | User ID                 |
| `body` | body | [codersdk.UpdateUserQuietHoursScheduleRequest](schemas.md#codersdkupdateuserquiethoursschedulerequest) | true     | Update schedule request |

### Example responses

> 200 Response

```json
[
  {
    "next": "2019-08-24T14:15:22Z",
    "raw_schedule": "string",
    "time": "string",
    "timezone": "string",
    "user_can_set": true,
    "user_set": true
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                                |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.UserQuietHoursScheduleResponse](schemas.md#codersdkuserquiethoursscheduleresponse) |

<h3 id="update-user-quiet-hours-schedule-responseschema">Response Schema</h3>

Status Code **200**

| Name             | Type              | Required | Restrictions | Description                                                                                                                                                                      |
|------------------|-------------------|----------|--------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `[array item]`   | array             | false    |              |                                                                                                                                                                                  |
| `» next`         | string(date-time) | false    |              | Next is the next time that the quiet hours window will start.                                                                                                                    |
| `» raw_schedule` | string            | false    |              |                                                                                                                                                                                  |
| `» time`         | string            | false    |              | Time is the time of day that the quiet hours window starts in the given Timezone each day.                                                                                       |
| `» timezone`     | string            | false    |              | raw format from the cron expression, UTC if unspecified                                                                                                                          |
| `» user_can_set` | boolean           | false    |              | User can set is true if the user is allowed to set their own quiet hours schedule. If false, the user cannot set a custom schedule and the default schedule will always be used. |
| `» user_set`     | boolean           | false    |              | User set is true if the user has set their own quiet hours schedule. If false, the user is using the default schedule.                                                           |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get workspace quota by user deprecated

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspace-quota/{user} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspace-quota/{user}`

### Parameters

| Name   | In   | Type   | Required | Description          |
|--------|------|--------|----------|----------------------|
| `user` | path | string | true     | User ID, name, or me |

### Example responses

> 200 Response

```json
{
  "budget": 0,
  "credits_consumed": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceQuota](schemas.md#codersdkworkspacequota) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get workspace proxies

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspaceproxies \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspaceproxies`

### Example responses

> 200 Response

```json
[
  {
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
            "errors": [
              "string"
            ],
            "warnings": [
              "string"
            ]
          },
          "status": "ok"
        },
        "updated_at": "2019-08-24T14:15:22Z",
        "version": "string",
        "wildcard_hostname": "string"
      }
    ]
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                                                  |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.RegionsResponse-codersdk_WorkspaceProxy](schemas.md#codersdkregionsresponse-codersdk_workspaceproxy) |

<h3 id="get-workspace-proxies-responseschema">Response Schema</h3>

Status Code **200**

| Name                   | Type                                                                     | Required | Restrictions | Description                                                                                                                                                                       |
|------------------------|--------------------------------------------------------------------------|----------|--------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `[array item]`         | array                                                                    | false    |              |                                                                                                                                                                                   |
| `» regions`            | array                                                                    | false    |              |                                                                                                                                                                                   |
| `»» created_at`        | string(date-time)                                                        | false    |              |                                                                                                                                                                                   |
| `»» deleted`           | boolean                                                                  | false    |              |                                                                                                                                                                                   |
| `»» derp_enabled`      | boolean                                                                  | false    |              |                                                                                                                                                                                   |
| `»» derp_only`         | boolean                                                                  | false    |              |                                                                                                                                                                                   |
| `»» display_name`      | string                                                                   | false    |              |                                                                                                                                                                                   |
| `»» healthy`           | boolean                                                                  | false    |              |                                                                                                                                                                                   |
| `»» icon_url`          | string                                                                   | false    |              |                                                                                                                                                                                   |
| `»» id`                | string(uuid)                                                             | false    |              |                                                                                                                                                                                   |
| `»» name`              | string                                                                   | false    |              |                                                                                                                                                                                   |
| `»» path_app_url`      | string                                                                   | false    |              | Path app URL is the URL to the base path for path apps. Optional unless wildcard_hostname is set. E.g. https://us.example.com                                                     |
| `»» status`            | [codersdk.WorkspaceProxyStatus](schemas.md#codersdkworkspaceproxystatus) | false    |              | Status is the latest status check of the proxy. This will be empty for deleted proxies. This value can be used to determine if a workspace proxy is healthy and ready to use.     |
| `»»» checked_at`       | string(date-time)                                                        | false    |              |                                                                                                                                                                                   |
| `»»» report`           | [codersdk.ProxyHealthReport](schemas.md#codersdkproxyhealthreport)       | false    |              | Report provides more information about the health of the workspace proxy.                                                                                                         |
| `»»»» errors`          | array                                                                    | false    |              | Errors are problems that prevent the workspace proxy from being healthy                                                                                                           |
| `»»»» warnings`        | array                                                                    | false    |              | Warnings do not prevent the workspace proxy from being healthy, but should be addressed.                                                                                          |
| `»»» status`           | [codersdk.ProxyHealthStatus](schemas.md#codersdkproxyhealthstatus)       | false    |              |                                                                                                                                                                                   |
| `»» updated_at`        | string(date-time)                                                        | false    |              |                                                                                                                                                                                   |
| `»» version`           | string                                                                   | false    |              |                                                                                                                                                                                   |
| `»» wildcard_hostname` | string                                                                   | false    |              | Wildcard hostname is the wildcard hostname for subdomain apps. E.g. *.us.example.com E.g.*--suffix.au.example.com Optional. Does not need to be on the same domain as PathAppURL. |

#### Enumerated Values

| Property | Value          |
|----------|----------------|
| `status` | `ok`           |
| `status` | `unreachable`  |
| `status` | `unhealthy`    |
| `status` | `unregistered` |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create workspace proxy

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/workspaceproxies \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /workspaceproxies`

> Body parameter

```json
{
  "display_name": "string",
  "icon": "string",
  "name": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                   | Required | Description                    |
|--------|------|----------------------------------------------------------------------------------------|----------|--------------------------------|
| `body` | body | [codersdk.CreateWorkspaceProxyRequest](schemas.md#codersdkcreateworkspaceproxyrequest) | true     | Create workspace proxy request |

### Example responses

> 201 Response

```json
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
      "errors": [
        "string"
      ],
      "warnings": [
        "string"
      ]
    },
    "status": "ok"
  },
  "updated_at": "2019-08-24T14:15:22Z",
  "version": "string",
  "wildcard_hostname": "string"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                       |
|--------|--------------------------------------------------------------|-------------|--------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.WorkspaceProxy](schemas.md#codersdkworkspaceproxy) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get workspace proxy

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/workspaceproxies/{workspaceproxy} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /workspaceproxies/{workspaceproxy}`

### Parameters

| Name             | In   | Type         | Required | Description      |
|------------------|------|--------------|----------|------------------|
| `workspaceproxy` | path | string(uuid) | true     | Proxy ID or name |

### Example responses

> 200 Response

```json
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
      "errors": [
        "string"
      ],
      "warnings": [
        "string"
      ]
    },
    "status": "ok"
  },
  "updated_at": "2019-08-24T14:15:22Z",
  "version": "string",
  "wildcard_hostname": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceProxy](schemas.md#codersdkworkspaceproxy) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete workspace proxy

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/workspaceproxies/{workspaceproxy} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /workspaceproxies/{workspaceproxy}`

### Parameters

| Name             | In   | Type         | Required | Description      |
|------------------|------|--------------|----------|------------------|
| `workspaceproxy` | path | string(uuid) | true     | Proxy ID or name |

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

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update workspace proxy

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/workspaceproxies/{workspaceproxy} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /workspaceproxies/{workspaceproxy}`

> Body parameter

```json
{
  "display_name": "string",
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "regenerate_token": true
}
```

### Parameters

| Name             | In   | Type                                                                   | Required | Description                    |
|------------------|------|------------------------------------------------------------------------|----------|--------------------------------|
| `workspaceproxy` | path | string(uuid)                                                           | true     | Proxy ID or name               |
| `body`           | body | [codersdk.PatchWorkspaceProxy](schemas.md#codersdkpatchworkspaceproxy) | true     | Update workspace proxy request |

### Example responses

> 200 Response

```json
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
      "errors": [
        "string"
      ],
      "warnings": [
        "string"
      ]
    },
    "status": "ok"
  },
  "updated_at": "2019-08-24T14:15:22Z",
  "version": "string",
  "wildcard_hostname": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceProxy](schemas.md#codersdkworkspaceproxy) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
