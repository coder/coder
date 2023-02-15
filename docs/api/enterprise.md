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
  "logo_url": "string",
  "service_banner": {
    "background_color": "string",
    "enabled": true,
    "message": "string"
  }
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
  "logo_url": "string",
  "service_banner": {
    "background_color": "string",
    "enabled": true,
    "message": "string"
  }
}
```

### Parameters

| Name   | In   | Type                                                             | Required | Description               |
| ------ | ---- | ---------------------------------------------------------------- | -------- | ------------------------- |
| `body` | body | [codersdk.AppearanceConfig](schemas.md#codersdkappearanceconfig) | true     | Update appearance request |

### Example responses

> 200 Response

```json
{
  "logo_url": "string",
  "service_banner": {
    "background_color": "string",
    "enabled": true,
    "message": "string"
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                           |
| ------ | ------------------------------------------------------- | ----------- | ---------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AppearanceConfig](schemas.md#codersdkappearanceconfig) |

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
  "experimental": true,
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

## Get groups

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/groups \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /groups`

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
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "members": [
      {
        "avatar_url": "http://example.com",
        "created_at": "2019-08-24T14:15:22Z",
        "email": "user@example.com",
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "last_seen_at": "2019-08-24T14:15:22Z",
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
    "quota_allowance": 0
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                              |
| ------ | ------------------------------------------------------- | ----------- | --------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Group](schemas.md#codersdkgroup) |

<h3 id="get-groups-responseschema">Response Schema</h3>

Status Code **200**

| Name                  | Type                                                 | Required | Restrictions | Description |
| --------------------- | ---------------------------------------------------- | -------- | ------------ | ----------- |
| `[array item]`        | array                                                | false    |              |             |
| `» avatar_url`        | string                                               | false    |              |             |
| `» id`                | string(uuid)                                         | false    |              |             |
| `» members`           | array                                                | false    |              |             |
| `»» avatar_url`       | string(uri)                                          | false    |              |             |
| `»» created_at`       | string(date-time)                                    | true     |              |             |
| `»» email`            | string(email)                                        | true     |              |             |
| `»» id`               | string(uuid)                                         | true     |              |             |
| `»» last_seen_at`     | string(date-time)                                    | false    |              |             |
| `»» organization_ids` | array                                                | false    |              |             |
| `»» roles`            | array                                                | false    |              |             |
| `»»» display_name`    | string                                               | false    |              |             |
| `»»» name`            | string                                               | false    |              |             |
| `»» status`           | [codersdk.UserStatus](schemas.md#codersdkuserstatus) | false    |              |             |
| `»» username`         | string                                               | true     |              |             |
| `» name`              | string                                               | false    |              |             |
| `» organization_id`   | string(uuid)                                         | false    |              |             |
| `» quota_allowance`   | integer                                              | false    |              |             |

#### Enumerated Values

| Property | Value       |
| -------- | ----------- |
| `status` | `active`    |
| `status` | `suspended` |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get group by name

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
| `group` | path | string | true     | Group name  |

### Example responses

> 200 Response

```json
{
  "avatar_url": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "members": [
    {
      "avatar_url": "http://example.com",
      "created_at": "2019-08-24T14:15:22Z",
      "email": "user@example.com",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "last_seen_at": "2019-08-24T14:15:22Z",
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
  "quota_allowance": 0
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
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "members": [
    {
      "avatar_url": "http://example.com",
      "created_at": "2019-08-24T14:15:22Z",
      "email": "user@example.com",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "last_seen_at": "2019-08-24T14:15:22Z",
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
  "quota_allowance": 0
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
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /groups/{group}`

### Parameters

| Name    | In   | Type   | Required | Description |
| ------- | ---- | ------ | -------- | ----------- |
| `group` | path | string | true     | Group name  |

### Example responses

> 200 Response

```json
{
  "avatar_url": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "members": [
    {
      "avatar_url": "http://example.com",
      "created_at": "2019-08-24T14:15:22Z",
      "email": "user@example.com",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "last_seen_at": "2019-08-24T14:15:22Z",
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
  "quota_allowance": 0
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
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "members": [
      {
        "avatar_url": "http://example.com",
        "created_at": "2019-08-24T14:15:22Z",
        "email": "user@example.com",
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "last_seen_at": "2019-08-24T14:15:22Z",
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
    "quota_allowance": 0
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                              |
| ------ | ------------------------------------------------------- | ----------- | --------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Group](schemas.md#codersdkgroup) |

<h3 id="get-groups-by-organization-responseschema">Response Schema</h3>

Status Code **200**

| Name                  | Type                                                 | Required | Restrictions | Description |
| --------------------- | ---------------------------------------------------- | -------- | ------------ | ----------- |
| `[array item]`        | array                                                | false    |              |             |
| `» avatar_url`        | string                                               | false    |              |             |
| `» id`                | string(uuid)                                         | false    |              |             |
| `» members`           | array                                                | false    |              |             |
| `»» avatar_url`       | string(uri)                                          | false    |              |             |
| `»» created_at`       | string(date-time)                                    | true     |              |             |
| `»» email`            | string(email)                                        | true     |              |             |
| `»» id`               | string(uuid)                                         | true     |              |             |
| `»» last_seen_at`     | string(date-time)                                    | false    |              |             |
| `»» organization_ids` | array                                                | false    |              |             |
| `»» roles`            | array                                                | false    |              |             |
| `»»» display_name`    | string                                               | false    |              |             |
| `»»» name`            | string                                               | false    |              |             |
| `»» status`           | [codersdk.UserStatus](schemas.md#codersdkuserstatus) | false    |              |             |
| `»» username`         | string                                               | true     |              |             |
| `» name`              | string                                               | false    |              |             |
| `» organization_id`   | string(uuid)                                         | false    |              |             |
| `» quota_allowance`   | integer                                              | false    |              |             |

#### Enumerated Values

| Property | Value       |
| -------- | ----------- |
| `status` | `active`    |
| `status` | `suspended` |

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
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "members": [
    {
      "avatar_url": "http://example.com",
      "created_at": "2019-08-24T14:15:22Z",
      "email": "user@example.com",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "last_seen_at": "2019-08-24T14:15:22Z",
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
  "quota_allowance": 0
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
| `» organization_ids` | array                                                    | false    |              |             |
| `» role`             | [codersdk.TemplateRole](schemas.md#codersdktemplaterole) | false    |              |             |
| `» roles`            | array                                                    | false    |              |             |
| `»» display_name`    | string                                                   | false    |              |             |
| `»» name`            | string                                                   | false    |              |             |
| `» status`           | [codersdk.UserStatus](schemas.md#codersdkuserstatus)     | false    |              |             |
| `» username`         | string                                                   | true     |              |             |

#### Enumerated Values

| Property | Value       |
| -------- | ----------- |
| `role`   | `admin`     |
| `role`   | `use`       |
| `status` | `active`    |
| `status` | `suspended` |

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
    "property1": "admin",
    "property2": "admin"
  },
  "user_perms": {
    "property1": "admin",
    "property2": "admin"
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
