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
  "application_name": "string",
  "logo_url": "string",
  "service_banner": {
    "background_color": "string",
    "enabled": true,
    "message": "string"
  },
  "support_links": [
    {
      "icon": "string",
      "name": "string",
      "target": "string"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                           |
| ------ | ------------------------------------------------------- | ----------- | ---------------------------------------------------------------- |
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
| ------ | ---- | ---------------------------------------------------------------------------- | -------- | ------------------------- |
| `body` | body | [codersdk.UpdateAppearanceConfig](schemas.md#codersdkupdateappearanceconfig) | true     | Update appearance request |

### Example responses

> 200 Response

```json
{
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
| ------ | ------------------------------------------------------- | ----------- | ---------------------------------------------------------------------------- |
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
  "errors": ["string"],
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
  "warnings": ["string"]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                   |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Entitlements](schemas.md#codersdkentitlements) |

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
| ------- | ---- | ------ | -------- | ----------- |
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
      "organization_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
      "roles": [
        {
          "display_name": "string",
          "name": "string"
        }
      ],
      "status": "active",
      "username": "string"
    }
  ],
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "quota_allowance": 0,
  "source": "user"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                     |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------ |
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
| ------- | ---- | ------ | -------- | ----------- |
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
      "organization_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
      "roles": [
        {
          "display_name": "string",
          "name": "string"
        }
      ],
      "status": "active",
      "username": "string"
    }
  ],
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "quota_allowance": 0,
  "source": "user"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                     |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------ |
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
  "add_users": ["string"],
  "avatar_url": "string",
  "display_name": "string",
  "name": "string",
  "quota_allowance": 0,
  "remove_users": ["string"]
}
```

### Parameters

| Name    | In   | Type                                                               | Required | Description         |
| ------- | ---- | ------------------------------------------------------------------ | -------- | ------------------- |
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
      "organization_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
      "roles": [
        {
          "display_name": "string",
          "name": "string"
        }
      ],
      "status": "active",
      "username": "string"
    }
  ],
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "quota_allowance": 0,
  "source": "user"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                     |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Group](schemas.md#codersdkgroup) |

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
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.License](schemas.md#codersdklicense) |

<h3 id="get-licenses-responseschema">Response Schema</h3>

Status Code **200**

| Name            | Type              | Required | Restrictions | Description                                                                                                                                                                                            |
| --------------- | ----------------- | -------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `[array item]`  | array             | false    |              |                                                                                                                                                                                                        |
| `» claims`      | object            | false    |              | Claims are the JWT claims asserted by the license. Here we use a generic string map to ensure that all data from the server is parsed verbatim, not just the fields this version of Coder understands. |
| `» id`          | integer           | false    |              |                                                                                                                                                                                                        |
| `» uploaded_at` | string(date-time) | false    |              |                                                                                                                                                                                                        |
| `» uuid`        | string(uuid)      | false    |              |                                                                                                                                                                                                        |

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
| ---- | ---- | -------------- | -------- | ----------- |
| `id` | path | string(number) | true     | License ID  |

### Responses

| Status | Meaning                                                 | Description | Schema |
| ------ | ------------------------------------------------------- | ----------- | ------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

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
| -------------- | ---- | ------------ | -------- | --------------- |
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
        "organization_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
        "roles": [
          {
            "display_name": "string",
            "name": "string"
          }
        ],
        "status": "active",
        "username": "string"
      }
    ],
    "name": "string",
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "quota_allowance": 0,
    "source": "user"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                              |
| ------ | ------------------------------------------------------- | ----------- | --------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Group](schemas.md#codersdkgroup) |

<h3 id="get-groups-by-organization-responseschema">Response Schema</h3>

Status Code **200**

| Name                  | Type                                                   | Required | Restrictions | Description |
| --------------------- | ------------------------------------------------------ | -------- | ------------ | ----------- |
| `[array item]`        | array                                                  | false    |              |             |
| `» avatar_url`        | string                                                 | false    |              |             |
| `» display_name`      | string                                                 | false    |              |             |
| `» id`                | string(uuid)                                           | false    |              |             |
| `» members`           | array                                                  | false    |              |             |
| `»» avatar_url`       | string(uri)                                            | false    |              |             |
| `»» created_at`       | string(date-time)                                      | true     |              |             |
| `»» email`            | string(email)                                          | true     |              |             |
| `»» id`               | string(uuid)                                           | true     |              |             |
| `»» last_seen_at`     | string(date-time)                                      | false    |              |             |
| `»» login_type`       | [codersdk.LoginType](schemas.md#codersdklogintype)     | false    |              |             |
| `»» organization_ids` | array                                                  | false    |              |             |
| `»» roles`            | array                                                  | false    |              |             |
| `»»» display_name`    | string                                                 | false    |              |             |
| `»»» name`            | string                                                 | false    |              |             |
| `»» status`           | [codersdk.UserStatus](schemas.md#codersdkuserstatus)   | false    |              |             |
| `»» username`         | string                                                 | true     |              |             |
| `» name`              | string                                                 | false    |              |             |
| `» organization_id`   | string(uuid)                                           | false    |              |             |
| `» quota_allowance`   | integer                                                | false    |              |             |
| `» source`            | [codersdk.GroupSource](schemas.md#codersdkgroupsource) | false    |              |             |

#### Enumerated Values

| Property     | Value       |
| ------------ | ----------- |
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
| -------------- | ---- | -------------------------------------------------------------------- | -------- | -------------------- |
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
      "organization_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
      "roles": [
        {
          "display_name": "string",
          "name": "string"
        }
      ],
      "status": "active",
      "username": "string"
    }
  ],
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "quota_allowance": 0,
  "source": "user"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                     |
| ------ | ------------------------------------------------------------ | ----------- | ------------------------------------------ |
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
| -------------- | ---- | ------------ | -------- | --------------- |
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
      "organization_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
      "roles": [
        {
          "display_name": "string",
          "name": "string"
        }
      ],
      "status": "active",
      "username": "string"
    }
  ],
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "quota_allowance": 0,
  "source": "user"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                     |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Group](schemas.md#codersdkgroup) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get provisioner daemons

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/provisionerdaemons \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/provisionerdaemons`

### Parameters

| Name           | In   | Type         | Required | Description     |
| -------------- | ---- | ------------ | -------- | --------------- |
| `organization` | path | string(uuid) | true     | Organization ID |

### Example responses

> 200 Response

```json
[
  {
    "created_at": "2019-08-24T14:15:22Z",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "name": "string",
    "provisioners": ["string"],
    "tags": {
      "property1": "string",
      "property2": "string"
    },
    "updated_at": {
      "time": "string",
      "valid": true
    }
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                      |
| ------ | ------------------------------------------------------- | ----------- | --------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.ProvisionerDaemon](schemas.md#codersdkprovisionerdaemon) |

<h3 id="get-provisioner-daemons-responseschema">Response Schema</h3>

Status Code **200**

| Name                | Type                                   | Required | Restrictions | Description                       |
| ------------------- | -------------------------------------- | -------- | ------------ | --------------------------------- |
| `[array item]`      | array                                  | false    |              |                                   |
| `» created_at`      | string(date-time)                      | false    |              |                                   |
| `» id`              | string(uuid)                           | false    |              |                                   |
| `» name`            | string                                 | false    |              |                                   |
| `» provisioners`    | array                                  | false    |              |                                   |
| `» tags`            | object                                 | false    |              |                                   |
| `»» [any property]` | string                                 | false    |              |                                   |
| `» updated_at`      | [sql.NullTime](schemas.md#sqlnulltime) | false    |              |                                   |
| `»» time`           | string                                 | false    |              |                                   |
| `»» valid`          | boolean                                | false    |              | Valid is true if Time is not NULL |

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
| -------------- | ---- | ------------ | -------- | --------------- |
| `organization` | path | string(uuid) | true     | Organization ID |

### Responses

| Status | Meaning                                                                  | Description         | Schema |
| ------ | ------------------------------------------------------------------------ | ------------------- | ------ |
| 101    | [Switching Protocols](https://tools.ietf.org/html/rfc7231#section-6.2.2) | Switching Protocols |        |

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
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Replica](schemas.md#codersdkreplica) |

<h3 id="get-active-replicas-responseschema">Response Schema</h3>

Status Code **200**

| Name                 | Type              | Required | Restrictions | Description                                                        |
| -------------------- | ----------------- | -------- | ------------ | ------------------------------------------------------------------ |
| `[array item]`       | array             | false    |              |                                                                    |
| `» created_at`       | string(date-time) | false    |              | Created at is the timestamp when the replica was first seen.       |
| `» database_latency` | integer           | false    |              | Database latency is the latency in microseconds to the database.   |
| `» error`            | string            | false    |              | Error is the replica error.                                        |
| `» hostname`         | string            | false    |              | Hostname is the hostname of the replica.                           |
| `» id`               | string(uuid)      | false    |              | ID is the unique identifier for the replica.                       |
| `» region_id`        | integer           | false    |              | Region ID is the region of the replica.                            |
| `» relay_address`    | string            | false    |              | Relay address is the accessible address to relay DERP connections. |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## SCIM 2.0: Get users

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/scim/v2/Users \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /scim/v2/Users`

### Responses

| Status | Meaning                                                 | Description | Schema |
| ------ | ------------------------------------------------------- | ----------- | ------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## SCIM 2.0: Create new user

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/scim/v2/Users \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
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
  "groups": [null],
  "id": "string",
  "meta": {
    "resourceType": "string"
  },
  "name": {
    "familyName": "string",
    "givenName": "string"
  },
  "schemas": ["string"],
  "userName": "string"
}
```

### Parameters

| Name   | In   | Type                                         | Required | Description |
| ------ | ---- | -------------------------------------------- | -------- | ----------- |
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
  "groups": [null],
  "id": "string",
  "meta": {
    "resourceType": "string"
  },
  "name": {
    "familyName": "string",
    "givenName": "string"
  },
  "schemas": ["string"],
  "userName": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                       |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [coderd.SCIMUser](schemas.md#coderdscimuser) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## SCIM 2.0: Get user by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/scim/v2/Users/{id} \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /scim/v2/Users/{id}`

### Parameters

| Name | In   | Type         | Required | Description |
| ---- | ---- | ------------ | -------- | ----------- |
| `id` | path | string(uuid) | true     | User ID     |

### Responses

| Status | Meaning                                                        | Description | Schema |
| ------ | -------------------------------------------------------------- | ----------- | ------ |
| 404    | [Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4) | Not Found   |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## SCIM 2.0: Update user account

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/scim/v2/Users/{id} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/scim+json' \
  -H 'Coder-Session-Token: API_KEY'
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
  "groups": [null],
  "id": "string",
  "meta": {
    "resourceType": "string"
  },
  "name": {
    "familyName": "string",
    "givenName": "string"
  },
  "schemas": ["string"],
  "userName": "string"
}
```

### Parameters

| Name   | In   | Type                                         | Required | Description         |
| ------ | ---- | -------------------------------------------- | -------- | ------------------- |
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
  "organization_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
  "roles": [
    {
      "display_name": "string",
      "name": "string"
    }
  ],
  "status": "active",
  "username": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                   |
| ------ | ------------------------------------------------------- | ----------- | ---------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.User](schemas.md#codersdkuser) |

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
| ---------- | ---- | ------------ | -------- | ----------- |
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
    "organization_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
    "role": "admin",
    "roles": [
      {
        "display_name": "string",
        "name": "string"
      }
    ],
    "status": "active",
    "username": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                            |
| ------ | ------------------------------------------------------- | ----------- | ----------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.TemplateUser](schemas.md#codersdktemplateuser) |

<h3 id="get-template-acls-responseschema">Response Schema</h3>

Status Code **200**

| Name                 | Type                                                     | Required | Restrictions | Description |
| -------------------- | -------------------------------------------------------- | -------- | ------------ | ----------- |
| `[array item]`       | array                                                    | false    |              |             |
| `» avatar_url`       | string(uri)                                              | false    |              |             |
| `» created_at`       | string(date-time)                                        | true     |              |             |
| `» email`            | string(email)                                            | true     |              |             |
| `» id`               | string(uuid)                                             | true     |              |             |
| `» last_seen_at`     | string(date-time)                                        | false    |              |             |
| `» login_type`       | [codersdk.LoginType](schemas.md#codersdklogintype)       | false    |              |             |
| `» organization_ids` | array                                                    | false    |              |             |
| `» role`             | [codersdk.TemplateRole](schemas.md#codersdktemplaterole) | false    |              |             |
| `» roles`            | array                                                    | false    |              |             |
| `»» display_name`    | string                                                   | false    |              |             |
| `»» name`            | string                                                   | false    |              |             |
| `» status`           | [codersdk.UserStatus](schemas.md#codersdkuserstatus)     | false    |              |             |
| `» username`         | string                                                   | true     |              |             |

#### Enumerated Values

| Property     | Value       |
| ------------ | ----------- |
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
| ---------- | ---- | ------------------------------------------------------------------ | -------- | ----------------------- |
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
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------ |
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
| ---------- | ---- | ------------ | -------- | ----------- |
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
            "organization_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
            "roles": [
              {
                "display_name": "string",
                "name": "string"
              }
            ],
            "status": "active",
            "username": "string"
          }
        ],
        "name": "string",
        "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
        "quota_allowance": 0,
        "source": "user"
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
        "organization_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
        "roles": [
          {
            "display_name": "string",
            "name": "string"
          }
        ],
        "status": "active",
        "username": "string"
      }
    ]
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                            |
| ------ | ------------------------------------------------------- | ----------- | ----------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.ACLAvailable](schemas.md#codersdkaclavailable) |

<h3 id="get-template-available-acl-users/groups-responseschema">Response Schema</h3>

Status Code **200**

| Name                   | Type                                                   | Required | Restrictions | Description |
| ---------------------- | ------------------------------------------------------ | -------- | ------------ | ----------- |
| `[array item]`         | array                                                  | false    |              |             |
| `» groups`             | array                                                  | false    |              |             |
| `»» avatar_url`        | string                                                 | false    |              |             |
| `»» display_name`      | string                                                 | false    |              |             |
| `»» id`                | string(uuid)                                           | false    |              |             |
| `»» members`           | array                                                  | false    |              |             |
| `»»» avatar_url`       | string(uri)                                            | false    |              |             |
| `»»» created_at`       | string(date-time)                                      | true     |              |             |
| `»»» email`            | string(email)                                          | true     |              |             |
| `»»» id`               | string(uuid)                                           | true     |              |             |
| `»»» last_seen_at`     | string(date-time)                                      | false    |              |             |
| `»»» login_type`       | [codersdk.LoginType](schemas.md#codersdklogintype)     | false    |              |             |
| `»»» organization_ids` | array                                                  | false    |              |             |
| `»»» roles`            | array                                                  | false    |              |             |
| `»»»» display_name`    | string                                                 | false    |              |             |
| `»»»» name`            | string                                                 | false    |              |             |
| `»»» status`           | [codersdk.UserStatus](schemas.md#codersdkuserstatus)   | false    |              |             |
| `»»» username`         | string                                                 | true     |              |             |
| `»» name`              | string                                                 | false    |              |             |
| `»» organization_id`   | string(uuid)                                           | false    |              |             |
| `»» quota_allowance`   | integer                                                | false    |              |             |
| `»» source`            | [codersdk.GroupSource](schemas.md#codersdkgroupsource) | false    |              |             |
| `» users`              | array                                                  | false    |              |             |

#### Enumerated Values

| Property     | Value       |
| ------------ | ----------- |
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
| ------ | ---- | ------------ | -------- | ----------- |
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
    "user_set": true
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                                |
| ------ | ------------------------------------------------------- | ----------- | ----------------------------------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.UserQuietHoursScheduleResponse](schemas.md#codersdkuserquiethoursscheduleresponse) |

<h3 id="get-user-quiet-hours-schedule-responseschema">Response Schema</h3>

Status Code **200**

| Name             | Type              | Required | Restrictions | Description                                                                                                            |
| ---------------- | ----------------- | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------- |
| `[array item]`   | array             | false    |              |                                                                                                                        |
| `» next`         | string(date-time) | false    |              | Next is the next time that the quiet hours window will start.                                                          |
| `» raw_schedule` | string            | false    |              |                                                                                                                        |
| `» time`         | string            | false    |              | Time is the time of day that the quiet hours window starts in the given Timezone each day.                             |
| `» timezone`     | string            | false    |              | raw format from the cron expression, UTC if unspecified                                                                |
| `» user_set`     | boolean           | false    |              | User set is true if the user has set their own quiet hours schedule. If false, the user is using the default schedule. |

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
| ------ | ---- | ------------------------------------------------------------------------------------------------------ | -------- | ----------------------- |
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
    "user_set": true
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                                |
| ------ | ------------------------------------------------------- | ----------- | ----------------------------------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.UserQuietHoursScheduleResponse](schemas.md#codersdkuserquiethoursscheduleresponse) |

<h3 id="update-user-quiet-hours-schedule-responseschema">Response Schema</h3>

Status Code **200**

| Name             | Type              | Required | Restrictions | Description                                                                                                            |
| ---------------- | ----------------- | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------- |
| `[array item]`   | array             | false    |              |                                                                                                                        |
| `» next`         | string(date-time) | false    |              | Next is the next time that the quiet hours window will start.                                                          |
| `» raw_schedule` | string            | false    |              |                                                                                                                        |
| `» time`         | string            | false    |              | Time is the time of day that the quiet hours window starts in the given Timezone each day.                             |
| `» timezone`     | string            | false    |              | raw format from the cron expression, UTC if unspecified                                                                |
| `» user_set`     | boolean           | false    |              | User set is true if the user has set their own quiet hours schedule. If false, the user is using the default schedule. |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get workspace quota by user

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
| ------ | ---- | ------ | -------- | -------------------- |
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
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------ |
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
            "errors": ["string"],
            "warnings": ["string"]
          },
          "status": "ok"
        },
        "updated_at": "2019-08-24T14:15:22Z",
        "wildcard_hostname": "string"
      }
    ]
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                                                  |
| ------ | ------------------------------------------------------- | ----------- | ----------------------------------------------------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.RegionsResponse-codersdk_WorkspaceProxy](schemas.md#codersdkregionsresponse-codersdk_workspaceproxy) |

<h3 id="get-workspace-proxies-responseschema">Response Schema</h3>

Status Code **200**

| Name                   | Type                                                                     | Required | Restrictions | Description                                                                                                                                                                        |
| ---------------------- | ------------------------------------------------------------------------ | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `[array item]`         | array                                                                    | false    |              |                                                                                                                                                                                    |
| `» regions`            | array                                                                    | false    |              |                                                                                                                                                                                    |
| `»» created_at`        | string(date-time)                                                        | false    |              |                                                                                                                                                                                    |
| `»» deleted`           | boolean                                                                  | false    |              |                                                                                                                                                                                    |
| `»» derp_enabled`      | boolean                                                                  | false    |              |                                                                                                                                                                                    |
| `»» derp_only`         | boolean                                                                  | false    |              |                                                                                                                                                                                    |
| `»» display_name`      | string                                                                   | false    |              |                                                                                                                                                                                    |
| `»» healthy`           | boolean                                                                  | false    |              |                                                                                                                                                                                    |
| `»» icon_url`          | string                                                                   | false    |              |                                                                                                                                                                                    |
| `»» id`                | string(uuid)                                                             | false    |              |                                                                                                                                                                                    |
| `»» name`              | string                                                                   | false    |              |                                                                                                                                                                                    |
| `»» path_app_url`      | string                                                                   | false    |              | Path app URL is the URL to the base path for path apps. Optional unless wildcard_hostname is set. E.g. https://us.example.com                                                      |
| `»» status`            | [codersdk.WorkspaceProxyStatus](schemas.md#codersdkworkspaceproxystatus) | false    |              | Status is the latest status check of the proxy. This will be empty for deleted proxies. This value can be used to determine if a workspace proxy is healthy and ready to use.      |
| `»»» checked_at`       | string(date-time)                                                        | false    |              |                                                                                                                                                                                    |
| `»»» report`           | [codersdk.ProxyHealthReport](schemas.md#codersdkproxyhealthreport)       | false    |              | Report provides more information about the health of the workspace proxy.                                                                                                          |
| `»»»» errors`          | array                                                                    | false    |              | Errors are problems that prevent the workspace proxy from being healthy                                                                                                            |
| `»»»» warnings`        | array                                                                    | false    |              | Warnings do not prevent the workspace proxy from being healthy, but should be addressed.                                                                                           |
| `»»» status`           | [codersdk.ProxyHealthStatus](schemas.md#codersdkproxyhealthstatus)       | false    |              |                                                                                                                                                                                    |
| `»» updated_at`        | string(date-time)                                                        | false    |              |                                                                                                                                                                                    |
| `»» wildcard_hostname` | string                                                                   | false    |              | Wildcard hostname is the wildcard hostname for subdomain apps. E.g. _.us.example.com E.g. _--suffix.au.example.com Optional. Does not need to be on the same domain as PathAppURL. |

#### Enumerated Values

| Property | Value          |
| -------- | -------------- |
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
| ------ | ---- | -------------------------------------------------------------------------------------- | -------- | ------------------------------ |
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
      "errors": ["string"],
      "warnings": ["string"]
    },
    "status": "ok"
  },
  "updated_at": "2019-08-24T14:15:22Z",
  "wildcard_hostname": "string"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                       |
| ------ | ------------------------------------------------------------ | ----------- | ------------------------------------------------------------ |
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
| ---------------- | ---- | ------------ | -------- | ---------------- |
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
      "errors": ["string"],
      "warnings": ["string"]
    },
    "status": "ok"
  },
  "updated_at": "2019-08-24T14:15:22Z",
  "wildcard_hostname": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------ |
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
| ---------------- | ---- | ------------ | -------- | ---------------- |
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
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------ |
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
| ---------------- | ---- | ---------------------------------------------------------------------- | -------- | ------------------------------ |
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
      "errors": ["string"],
      "warnings": ["string"]
    },
    "status": "ok"
  },
  "updated_at": "2019-08-24T14:15:22Z",
  "wildcard_hostname": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceProxy](schemas.md#codersdkworkspaceproxy) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
