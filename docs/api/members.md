# Members

## Get member roles by organization

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/members/roles \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/members/roles`

### Parameters

| Name           | In   | Type         | Required | Description     |
| -------------- | ---- | ------------ | -------- | --------------- |
| `organization` | path | string(uuid) | true     | Organization ID |

### Example responses

> 200 Response

```json
[
  {
    "assignable": true,
    "built_in": true,
    "display_name": "string",
    "name": "string",
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "organization_permissions": {
      "property1": [
        {
          "action": "application_connect",
          "negate": true,
          "resource_type": "*"
        }
      ],
      "property2": [
        {
          "action": "application_connect",
          "negate": true,
          "resource_type": "*"
        }
      ]
    },
    "site_permissions": [
      {
        "action": "application_connect",
        "negate": true,
        "resource_type": "*"
      }
    ],
    "user_permissions": [
      {
        "action": "application_connect",
        "negate": true,
        "resource_type": "*"
      }
    ]
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                  |
| ------ | ------------------------------------------------------- | ----------- | ----------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.AssignableRoles](schemas.md#codersdkassignableroles) |

<h3 id="get-member-roles-by-organization-responseschema">Response Schema</h3>

Status Code **200**

| Name                         | Type                                                     | Required | Restrictions | Description                             |
| ---------------------------- | -------------------------------------------------------- | -------- | ------------ | --------------------------------------- |
| `[array item]`               | array                                                    | false    |              |                                         |
| `» assignable`               | boolean                                                  | false    |              |                                         |
| `» built_in`                 | boolean                                                  | false    |              | Built in roles are immutable            |
| `» display_name`             | string                                                   | false    |              |                                         |
| `» name`                     | string                                                   | false    |              |                                         |
| `» organization_id`          | string(uuid)                                             | false    |              |                                         |
| `» organization_permissions` | object                                                   | false    |              | map[<org_id>] -> Permissions            |
| `»» [any property]`          | array                                                    | false    |              |                                         |
| `»»» action`                 | [codersdk.RBACAction](schemas.md#codersdkrbacaction)     | false    |              |                                         |
| `»»» negate`                 | boolean                                                  | false    |              | Negate makes this a negative permission |
| `»»» resource_type`          | [codersdk.RBACResource](schemas.md#codersdkrbacresource) | false    |              |                                         |
| `» site_permissions`         | array                                                    | false    |              |                                         |
| `» user_permissions`         | array                                                    | false    |              |                                         |

#### Enumerated Values

| Property        | Value                   |
| --------------- | ----------------------- |
| `action`        | `application_connect`   |
| `action`        | `assign`                |
| `action`        | `create`                |
| `action`        | `delete`                |
| `action`        | `read`                  |
| `action`        | `read_personal`         |
| `action`        | `ssh`                   |
| `action`        | `update`                |
| `action`        | `update_personal`       |
| `action`        | `use`                   |
| `action`        | `view_insights`         |
| `action`        | `start`                 |
| `action`        | `stop`                  |
| `resource_type` | `*`                     |
| `resource_type` | `api_key`               |
| `resource_type` | `assign_org_role`       |
| `resource_type` | `assign_role`           |
| `resource_type` | `audit_log`             |
| `resource_type` | `debug_info`            |
| `resource_type` | `deployment_config`     |
| `resource_type` | `deployment_stats`      |
| `resource_type` | `file`                  |
| `resource_type` | `group`                 |
| `resource_type` | `license`               |
| `resource_type` | `oauth2_app`            |
| `resource_type` | `oauth2_app_code_token` |
| `resource_type` | `oauth2_app_secret`     |
| `resource_type` | `organization`          |
| `resource_type` | `organization_member`   |
| `resource_type` | `provisioner_daemon`    |
| `resource_type` | `replicas`              |
| `resource_type` | `system`                |
| `resource_type` | `tailnet_coordinator`   |
| `resource_type` | `template`              |
| `resource_type` | `user`                  |
| `resource_type` | `workspace`             |
| `resource_type` | `workspace_dormant`     |
| `resource_type` | `workspace_proxy`       |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Assign role to organization member

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/organizations/{organization}/members/{user}/roles \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /organizations/{organization}/members/{user}/roles`

> Body parameter

```json
{
  "roles": ["string"]
}
```

### Parameters

| Name           | In   | Type                                                   | Required | Description          |
| -------------- | ---- | ------------------------------------------------------ | -------- | -------------------- |
| `organization` | path | string                                                 | true     | Organization ID      |
| `user`         | path | string                                                 | true     | User ID, name, or me |
| `body`         | body | [codersdk.UpdateRoles](schemas.md#codersdkupdateroles) | true     | Update roles request |

### Example responses

> 200 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "roles": [
    {
      "display_name": "string",
      "name": "string"
    }
  ],
  "updated_at": "2019-08-24T14:15:22Z",
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                               |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.OrganizationMember](schemas.md#codersdkorganizationmember) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get site member roles

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/roles \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/roles`

### Example responses

> 200 Response

```json
[
  {
    "assignable": true,
    "built_in": true,
    "display_name": "string",
    "name": "string",
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "organization_permissions": {
      "property1": [
        {
          "action": "application_connect",
          "negate": true,
          "resource_type": "*"
        }
      ],
      "property2": [
        {
          "action": "application_connect",
          "negate": true,
          "resource_type": "*"
        }
      ]
    },
    "site_permissions": [
      {
        "action": "application_connect",
        "negate": true,
        "resource_type": "*"
      }
    ],
    "user_permissions": [
      {
        "action": "application_connect",
        "negate": true,
        "resource_type": "*"
      }
    ]
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                  |
| ------ | ------------------------------------------------------- | ----------- | ----------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.AssignableRoles](schemas.md#codersdkassignableroles) |

<h3 id="get-site-member-roles-responseschema">Response Schema</h3>

Status Code **200**

| Name                         | Type                                                     | Required | Restrictions | Description                             |
| ---------------------------- | -------------------------------------------------------- | -------- | ------------ | --------------------------------------- |
| `[array item]`               | array                                                    | false    |              |                                         |
| `» assignable`               | boolean                                                  | false    |              |                                         |
| `» built_in`                 | boolean                                                  | false    |              | Built in roles are immutable            |
| `» display_name`             | string                                                   | false    |              |                                         |
| `» name`                     | string                                                   | false    |              |                                         |
| `» organization_id`          | string(uuid)                                             | false    |              |                                         |
| `» organization_permissions` | object                                                   | false    |              | map[<org_id>] -> Permissions            |
| `»» [any property]`          | array                                                    | false    |              |                                         |
| `»»» action`                 | [codersdk.RBACAction](schemas.md#codersdkrbacaction)     | false    |              |                                         |
| `»»» negate`                 | boolean                                                  | false    |              | Negate makes this a negative permission |
| `»»» resource_type`          | [codersdk.RBACResource](schemas.md#codersdkrbacresource) | false    |              |                                         |
| `» site_permissions`         | array                                                    | false    |              |                                         |
| `» user_permissions`         | array                                                    | false    |              |                                         |

#### Enumerated Values

| Property        | Value                   |
| --------------- | ----------------------- |
| `action`        | `application_connect`   |
| `action`        | `assign`                |
| `action`        | `create`                |
| `action`        | `delete`                |
| `action`        | `read`                  |
| `action`        | `read_personal`         |
| `action`        | `ssh`                   |
| `action`        | `update`                |
| `action`        | `update_personal`       |
| `action`        | `use`                   |
| `action`        | `view_insights`         |
| `action`        | `start`                 |
| `action`        | `stop`                  |
| `resource_type` | `*`                     |
| `resource_type` | `api_key`               |
| `resource_type` | `assign_org_role`       |
| `resource_type` | `assign_role`           |
| `resource_type` | `audit_log`             |
| `resource_type` | `debug_info`            |
| `resource_type` | `deployment_config`     |
| `resource_type` | `deployment_stats`      |
| `resource_type` | `file`                  |
| `resource_type` | `group`                 |
| `resource_type` | `license`               |
| `resource_type` | `oauth2_app`            |
| `resource_type` | `oauth2_app_code_token` |
| `resource_type` | `oauth2_app_secret`     |
| `resource_type` | `organization`          |
| `resource_type` | `organization_member`   |
| `resource_type` | `provisioner_daemon`    |
| `resource_type` | `replicas`              |
| `resource_type` | `system`                |
| `resource_type` | `tailnet_coordinator`   |
| `resource_type` | `template`              |
| `resource_type` | `user`                  |
| `resource_type` | `workspace`             |
| `resource_type` | `workspace_dormant`     |
| `resource_type` | `workspace_proxy`       |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Upsert a custom site-wide role

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/users/roles \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /users/roles`

### Example responses

> 200 Response

```json
[
  {
    "display_name": "string",
    "name": "string",
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "organization_permissions": {
      "property1": [
        {
          "action": "application_connect",
          "negate": true,
          "resource_type": "*"
        }
      ],
      "property2": [
        {
          "action": "application_connect",
          "negate": true,
          "resource_type": "*"
        }
      ]
    },
    "site_permissions": [
      {
        "action": "application_connect",
        "negate": true,
        "resource_type": "*"
      }
    ],
    "user_permissions": [
      {
        "action": "application_connect",
        "negate": true,
        "resource_type": "*"
      }
    ]
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                            |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Role](schemas.md#codersdkrole) |

<h3 id="upsert-a-custom-site-wide-role-responseschema">Response Schema</h3>

Status Code **200**

| Name                         | Type                                                     | Required | Restrictions | Description                             |
| ---------------------------- | -------------------------------------------------------- | -------- | ------------ | --------------------------------------- |
| `[array item]`               | array                                                    | false    |              |                                         |
| `» display_name`             | string                                                   | false    |              |                                         |
| `» name`                     | string                                                   | false    |              |                                         |
| `» organization_id`          | string(uuid)                                             | false    |              |                                         |
| `» organization_permissions` | object                                                   | false    |              | map[<org_id>] -> Permissions            |
| `»» [any property]`          | array                                                    | false    |              |                                         |
| `»»» action`                 | [codersdk.RBACAction](schemas.md#codersdkrbacaction)     | false    |              |                                         |
| `»»» negate`                 | boolean                                                  | false    |              | Negate makes this a negative permission |
| `»»» resource_type`          | [codersdk.RBACResource](schemas.md#codersdkrbacresource) | false    |              |                                         |
| `» site_permissions`         | array                                                    | false    |              |                                         |
| `» user_permissions`         | array                                                    | false    |              |                                         |

#### Enumerated Values

| Property        | Value                   |
| --------------- | ----------------------- |
| `action`        | `application_connect`   |
| `action`        | `assign`                |
| `action`        | `create`                |
| `action`        | `delete`                |
| `action`        | `read`                  |
| `action`        | `read_personal`         |
| `action`        | `ssh`                   |
| `action`        | `update`                |
| `action`        | `update_personal`       |
| `action`        | `use`                   |
| `action`        | `view_insights`         |
| `action`        | `start`                 |
| `action`        | `stop`                  |
| `resource_type` | `*`                     |
| `resource_type` | `api_key`               |
| `resource_type` | `assign_org_role`       |
| `resource_type` | `assign_role`           |
| `resource_type` | `audit_log`             |
| `resource_type` | `debug_info`            |
| `resource_type` | `deployment_config`     |
| `resource_type` | `deployment_stats`      |
| `resource_type` | `file`                  |
| `resource_type` | `group`                 |
| `resource_type` | `license`               |
| `resource_type` | `oauth2_app`            |
| `resource_type` | `oauth2_app_code_token` |
| `resource_type` | `oauth2_app_secret`     |
| `resource_type` | `organization`          |
| `resource_type` | `organization_member`   |
| `resource_type` | `provisioner_daemon`    |
| `resource_type` | `replicas`              |
| `resource_type` | `system`                |
| `resource_type` | `tailnet_coordinator`   |
| `resource_type` | `template`              |
| `resource_type` | `user`                  |
| `resource_type` | `workspace`             |
| `resource_type` | `workspace_dormant`     |
| `resource_type` | `workspace_proxy`       |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
