# Members

## List organization members

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/members \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/members`

### Parameters

| Name           | In   | Type   | Required | Description     |
|----------------|------|--------|----------|-----------------|
| `organization` | path | string | true     | Organization ID |

### Example responses

> 200 Response

```json
[
  {
    "avatar_url": "string",
    "created_at": "2019-08-24T14:15:22Z",
    "email": "string",
    "global_roles": [
      {
        "display_name": "string",
        "name": "string",
        "organization_id": "string"
      }
    ],
    "name": "string",
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "roles": [
      {
        "display_name": "string",
        "name": "string",
        "organization_id": "string"
      }
    ],
    "updated_at": "2019-08-24T14:15:22Z",
    "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5",
    "username": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                                |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.OrganizationMemberWithUserData](schemas.md#codersdkorganizationmemberwithuserdata) |

<h3 id="list-organization-members-responseschema">Response Schema</h3>

Status Code **200**

| Name                 | Type              | Required | Restrictions | Description |
|----------------------|-------------------|----------|--------------|-------------|
| `[array item]`       | array             | false    |              |             |
| `» avatar_url`       | string            | false    |              |             |
| `» created_at`       | string(date-time) | false    |              |             |
| `» email`            | string            | false    |              |             |
| `» global_roles`     | array             | false    |              |             |
| `»» display_name`    | string            | false    |              |             |
| `»» name`            | string            | false    |              |             |
| `»» organization_id` | string            | false    |              |             |
| `» name`             | string            | false    |              |             |
| `» organization_id`  | string(uuid)      | false    |              |             |
| `» roles`            | array             | false    |              |             |
| `» updated_at`       | string(date-time) | false    |              |             |
| `» user_id`          | string(uuid)      | false    |              |             |
| `» username`         | string            | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

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
|----------------|------|--------------|----------|-----------------|
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
    "organization_permissions": [
      {
        "action": "application_connect",
        "negate": true,
        "resource_type": "*"
      }
    ],
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
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.AssignableRoles](schemas.md#codersdkassignableroles) |

<h3 id="get-member-roles-by-organization-responseschema">Response Schema</h3>

Status Code **200**

| Name                         | Type                                                     | Required | Restrictions | Description                                                                                     |
|------------------------------|----------------------------------------------------------|----------|--------------|-------------------------------------------------------------------------------------------------|
| `[array item]`               | array                                                    | false    |              |                                                                                                 |
| `» assignable`               | boolean                                                  | false    |              |                                                                                                 |
| `» built_in`                 | boolean                                                  | false    |              | Built in roles are immutable                                                                    |
| `» display_name`             | string                                                   | false    |              |                                                                                                 |
| `» name`                     | string                                                   | false    |              |                                                                                                 |
| `» organization_id`          | string(uuid)                                             | false    |              |                                                                                                 |
| `» organization_permissions` | array                                                    | false    |              | Organization permissions are specific for the organization in the field 'OrganizationID' above. |
| `»» action`                  | [codersdk.RBACAction](schemas.md#codersdkrbacaction)     | false    |              |                                                                                                 |
| `»» negate`                  | boolean                                                  | false    |              | Negate makes this a negative permission                                                         |
| `»» resource_type`           | [codersdk.RBACResource](schemas.md#codersdkrbacresource) | false    |              |                                                                                                 |
| `» site_permissions`         | array                                                    | false    |              |                                                                                                 |
| `» user_permissions`         | array                                                    | false    |              |                                                                                                 |

#### Enumerated Values

| Property        | Value                              |
|-----------------|------------------------------------|
| `action`        | `application_connect`              |
| `action`        | `assign`                           |
| `action`        | `create`                           |
| `action`        | `delete`                           |
| `action`        | `read`                             |
| `action`        | `read_personal`                    |
| `action`        | `ssh`                              |
| `action`        | `update`                           |
| `action`        | `update_personal`                  |
| `action`        | `use`                              |
| `action`        | `view_insights`                    |
| `action`        | `start`                            |
| `action`        | `stop`                             |
| `resource_type` | `*`                                |
| `resource_type` | `api_key`                          |
| `resource_type` | `assign_org_role`                  |
| `resource_type` | `assign_role`                      |
| `resource_type` | `audit_log`                        |
| `resource_type` | `crypto_key`                       |
| `resource_type` | `debug_info`                       |
| `resource_type` | `deployment_config`                |
| `resource_type` | `deployment_stats`                 |
| `resource_type` | `file`                             |
| `resource_type` | `group`                            |
| `resource_type` | `group_member`                     |
| `resource_type` | `idpsync_settings`                 |
| `resource_type` | `license`                          |
| `resource_type` | `notification_message`             |
| `resource_type` | `notification_preference`          |
| `resource_type` | `notification_template`            |
| `resource_type` | `oauth2_app`                       |
| `resource_type` | `oauth2_app_code_token`            |
| `resource_type` | `oauth2_app_secret`                |
| `resource_type` | `organization`                     |
| `resource_type` | `organization_member`              |
| `resource_type` | `provisioner_daemon`               |
| `resource_type` | `provisioner_jobs`                 |
| `resource_type` | `provisioner_keys`                 |
| `resource_type` | `replicas`                         |
| `resource_type` | `system`                           |
| `resource_type` | `tailnet_coordinator`              |
| `resource_type` | `template`                         |
| `resource_type` | `user`                             |
| `resource_type` | `workspace`                        |
| `resource_type` | `workspace_agent_resource_monitor` |
| `resource_type` | `workspace_dormant`                |
| `resource_type` | `workspace_proxy`                  |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Upsert a custom organization role

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/organizations/{organization}/members/roles \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /organizations/{organization}/members/roles`

> Body parameter

```json
{
  "display_name": "string",
  "name": "string",
  "organization_permissions": [
    {
      "action": "application_connect",
      "negate": true,
      "resource_type": "*"
    }
  ],
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
```

### Parameters

| Name           | In   | Type                                                               | Required | Description         |
|----------------|------|--------------------------------------------------------------------|----------|---------------------|
| `organization` | path | string(uuid)                                                       | true     | Organization ID     |
| `body`         | body | [codersdk.CustomRoleRequest](schemas.md#codersdkcustomrolerequest) | true     | Upsert role request |

### Example responses

> 200 Response

```json
[
  {
    "display_name": "string",
    "name": "string",
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "organization_permissions": [
      {
        "action": "application_connect",
        "negate": true,
        "resource_type": "*"
      }
    ],
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
|--------|---------------------------------------------------------|-------------|---------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Role](schemas.md#codersdkrole) |

<h3 id="upsert-a-custom-organization-role-responseschema">Response Schema</h3>

Status Code **200**

| Name                         | Type                                                     | Required | Restrictions | Description                                                                                     |
|------------------------------|----------------------------------------------------------|----------|--------------|-------------------------------------------------------------------------------------------------|
| `[array item]`               | array                                                    | false    |              |                                                                                                 |
| `» display_name`             | string                                                   | false    |              |                                                                                                 |
| `» name`                     | string                                                   | false    |              |                                                                                                 |
| `» organization_id`          | string(uuid)                                             | false    |              |                                                                                                 |
| `» organization_permissions` | array                                                    | false    |              | Organization permissions are specific for the organization in the field 'OrganizationID' above. |
| `»» action`                  | [codersdk.RBACAction](schemas.md#codersdkrbacaction)     | false    |              |                                                                                                 |
| `»» negate`                  | boolean                                                  | false    |              | Negate makes this a negative permission                                                         |
| `»» resource_type`           | [codersdk.RBACResource](schemas.md#codersdkrbacresource) | false    |              |                                                                                                 |
| `» site_permissions`         | array                                                    | false    |              |                                                                                                 |
| `» user_permissions`         | array                                                    | false    |              |                                                                                                 |

#### Enumerated Values

| Property        | Value                              |
|-----------------|------------------------------------|
| `action`        | `application_connect`              |
| `action`        | `assign`                           |
| `action`        | `create`                           |
| `action`        | `delete`                           |
| `action`        | `read`                             |
| `action`        | `read_personal`                    |
| `action`        | `ssh`                              |
| `action`        | `update`                           |
| `action`        | `update_personal`                  |
| `action`        | `use`                              |
| `action`        | `view_insights`                    |
| `action`        | `start`                            |
| `action`        | `stop`                             |
| `resource_type` | `*`                                |
| `resource_type` | `api_key`                          |
| `resource_type` | `assign_org_role`                  |
| `resource_type` | `assign_role`                      |
| `resource_type` | `audit_log`                        |
| `resource_type` | `crypto_key`                       |
| `resource_type` | `debug_info`                       |
| `resource_type` | `deployment_config`                |
| `resource_type` | `deployment_stats`                 |
| `resource_type` | `file`                             |
| `resource_type` | `group`                            |
| `resource_type` | `group_member`                     |
| `resource_type` | `idpsync_settings`                 |
| `resource_type` | `license`                          |
| `resource_type` | `notification_message`             |
| `resource_type` | `notification_preference`          |
| `resource_type` | `notification_template`            |
| `resource_type` | `oauth2_app`                       |
| `resource_type` | `oauth2_app_code_token`            |
| `resource_type` | `oauth2_app_secret`                |
| `resource_type` | `organization`                     |
| `resource_type` | `organization_member`              |
| `resource_type` | `provisioner_daemon`               |
| `resource_type` | `provisioner_jobs`                 |
| `resource_type` | `provisioner_keys`                 |
| `resource_type` | `replicas`                         |
| `resource_type` | `system`                           |
| `resource_type` | `tailnet_coordinator`              |
| `resource_type` | `template`                         |
| `resource_type` | `user`                             |
| `resource_type` | `workspace`                        |
| `resource_type` | `workspace_agent_resource_monitor` |
| `resource_type` | `workspace_dormant`                |
| `resource_type` | `workspace_proxy`                  |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Insert a custom organization role

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/organizations/{organization}/members/roles \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /organizations/{organization}/members/roles`

> Body parameter

```json
{
  "display_name": "string",
  "name": "string",
  "organization_permissions": [
    {
      "action": "application_connect",
      "negate": true,
      "resource_type": "*"
    }
  ],
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
```

### Parameters

| Name           | In   | Type                                                               | Required | Description         |
|----------------|------|--------------------------------------------------------------------|----------|---------------------|
| `organization` | path | string(uuid)                                                       | true     | Organization ID     |
| `body`         | body | [codersdk.CustomRoleRequest](schemas.md#codersdkcustomrolerequest) | true     | Insert role request |

### Example responses

> 200 Response

```json
[
  {
    "display_name": "string",
    "name": "string",
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "organization_permissions": [
      {
        "action": "application_connect",
        "negate": true,
        "resource_type": "*"
      }
    ],
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
|--------|---------------------------------------------------------|-------------|---------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Role](schemas.md#codersdkrole) |

<h3 id="insert-a-custom-organization-role-responseschema">Response Schema</h3>

Status Code **200**

| Name                         | Type                                                     | Required | Restrictions | Description                                                                                     |
|------------------------------|----------------------------------------------------------|----------|--------------|-------------------------------------------------------------------------------------------------|
| `[array item]`               | array                                                    | false    |              |                                                                                                 |
| `» display_name`             | string                                                   | false    |              |                                                                                                 |
| `» name`                     | string                                                   | false    |              |                                                                                                 |
| `» organization_id`          | string(uuid)                                             | false    |              |                                                                                                 |
| `» organization_permissions` | array                                                    | false    |              | Organization permissions are specific for the organization in the field 'OrganizationID' above. |
| `»» action`                  | [codersdk.RBACAction](schemas.md#codersdkrbacaction)     | false    |              |                                                                                                 |
| `»» negate`                  | boolean                                                  | false    |              | Negate makes this a negative permission                                                         |
| `»» resource_type`           | [codersdk.RBACResource](schemas.md#codersdkrbacresource) | false    |              |                                                                                                 |
| `» site_permissions`         | array                                                    | false    |              |                                                                                                 |
| `» user_permissions`         | array                                                    | false    |              |                                                                                                 |

#### Enumerated Values

| Property        | Value                              |
|-----------------|------------------------------------|
| `action`        | `application_connect`              |
| `action`        | `assign`                           |
| `action`        | `create`                           |
| `action`        | `delete`                           |
| `action`        | `read`                             |
| `action`        | `read_personal`                    |
| `action`        | `ssh`                              |
| `action`        | `update`                           |
| `action`        | `update_personal`                  |
| `action`        | `use`                              |
| `action`        | `view_insights`                    |
| `action`        | `start`                            |
| `action`        | `stop`                             |
| `resource_type` | `*`                                |
| `resource_type` | `api_key`                          |
| `resource_type` | `assign_org_role`                  |
| `resource_type` | `assign_role`                      |
| `resource_type` | `audit_log`                        |
| `resource_type` | `crypto_key`                       |
| `resource_type` | `debug_info`                       |
| `resource_type` | `deployment_config`                |
| `resource_type` | `deployment_stats`                 |
| `resource_type` | `file`                             |
| `resource_type` | `group`                            |
| `resource_type` | `group_member`                     |
| `resource_type` | `idpsync_settings`                 |
| `resource_type` | `license`                          |
| `resource_type` | `notification_message`             |
| `resource_type` | `notification_preference`          |
| `resource_type` | `notification_template`            |
| `resource_type` | `oauth2_app`                       |
| `resource_type` | `oauth2_app_code_token`            |
| `resource_type` | `oauth2_app_secret`                |
| `resource_type` | `organization`                     |
| `resource_type` | `organization_member`              |
| `resource_type` | `provisioner_daemon`               |
| `resource_type` | `provisioner_jobs`                 |
| `resource_type` | `provisioner_keys`                 |
| `resource_type` | `replicas`                         |
| `resource_type` | `system`                           |
| `resource_type` | `tailnet_coordinator`              |
| `resource_type` | `template`                         |
| `resource_type` | `user`                             |
| `resource_type` | `workspace`                        |
| `resource_type` | `workspace_agent_resource_monitor` |
| `resource_type` | `workspace_dormant`                |
| `resource_type` | `workspace_proxy`                  |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete a custom organization role

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/organizations/{organization}/members/roles/{roleName} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /organizations/{organization}/members/roles/{roleName}`

### Parameters

| Name           | In   | Type         | Required | Description     |
|----------------|------|--------------|----------|-----------------|
| `organization` | path | string(uuid) | true     | Organization ID |
| `roleName`     | path | string       | true     | Role name       |

### Example responses

> 200 Response

```json
[
  {
    "display_name": "string",
    "name": "string",
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "organization_permissions": [
      {
        "action": "application_connect",
        "negate": true,
        "resource_type": "*"
      }
    ],
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
|--------|---------------------------------------------------------|-------------|---------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Role](schemas.md#codersdkrole) |

<h3 id="delete-a-custom-organization-role-responseschema">Response Schema</h3>

Status Code **200**

| Name                         | Type                                                     | Required | Restrictions | Description                                                                                     |
|------------------------------|----------------------------------------------------------|----------|--------------|-------------------------------------------------------------------------------------------------|
| `[array item]`               | array                                                    | false    |              |                                                                                                 |
| `» display_name`             | string                                                   | false    |              |                                                                                                 |
| `» name`                     | string                                                   | false    |              |                                                                                                 |
| `» organization_id`          | string(uuid)                                             | false    |              |                                                                                                 |
| `» organization_permissions` | array                                                    | false    |              | Organization permissions are specific for the organization in the field 'OrganizationID' above. |
| `»» action`                  | [codersdk.RBACAction](schemas.md#codersdkrbacaction)     | false    |              |                                                                                                 |
| `»» negate`                  | boolean                                                  | false    |              | Negate makes this a negative permission                                                         |
| `»» resource_type`           | [codersdk.RBACResource](schemas.md#codersdkrbacresource) | false    |              |                                                                                                 |
| `» site_permissions`         | array                                                    | false    |              |                                                                                                 |
| `» user_permissions`         | array                                                    | false    |              |                                                                                                 |

#### Enumerated Values

| Property        | Value                              |
|-----------------|------------------------------------|
| `action`        | `application_connect`              |
| `action`        | `assign`                           |
| `action`        | `create`                           |
| `action`        | `delete`                           |
| `action`        | `read`                             |
| `action`        | `read_personal`                    |
| `action`        | `ssh`                              |
| `action`        | `update`                           |
| `action`        | `update_personal`                  |
| `action`        | `use`                              |
| `action`        | `view_insights`                    |
| `action`        | `start`                            |
| `action`        | `stop`                             |
| `resource_type` | `*`                                |
| `resource_type` | `api_key`                          |
| `resource_type` | `assign_org_role`                  |
| `resource_type` | `assign_role`                      |
| `resource_type` | `audit_log`                        |
| `resource_type` | `crypto_key`                       |
| `resource_type` | `debug_info`                       |
| `resource_type` | `deployment_config`                |
| `resource_type` | `deployment_stats`                 |
| `resource_type` | `file`                             |
| `resource_type` | `group`                            |
| `resource_type` | `group_member`                     |
| `resource_type` | `idpsync_settings`                 |
| `resource_type` | `license`                          |
| `resource_type` | `notification_message`             |
| `resource_type` | `notification_preference`          |
| `resource_type` | `notification_template`            |
| `resource_type` | `oauth2_app`                       |
| `resource_type` | `oauth2_app_code_token`            |
| `resource_type` | `oauth2_app_secret`                |
| `resource_type` | `organization`                     |
| `resource_type` | `organization_member`              |
| `resource_type` | `provisioner_daemon`               |
| `resource_type` | `provisioner_jobs`                 |
| `resource_type` | `provisioner_keys`                 |
| `resource_type` | `replicas`                         |
| `resource_type` | `system`                           |
| `resource_type` | `tailnet_coordinator`              |
| `resource_type` | `template`                         |
| `resource_type` | `user`                             |
| `resource_type` | `workspace`                        |
| `resource_type` | `workspace_agent_resource_monitor` |
| `resource_type` | `workspace_dormant`                |
| `resource_type` | `workspace_proxy`                  |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Add organization member

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/organizations/{organization}/members/{user} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /organizations/{organization}/members/{user}`

### Parameters

| Name           | In   | Type   | Required | Description          |
|----------------|------|--------|----------|----------------------|
| `organization` | path | string | true     | Organization ID      |
| `user`         | path | string | true     | User ID, name, or me |

### Example responses

> 200 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "roles": [
    {
      "display_name": "string",
      "name": "string",
      "organization_id": "string"
    }
  ],
  "updated_at": "2019-08-24T14:15:22Z",
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                               |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.OrganizationMember](schemas.md#codersdkorganizationmember) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Remove organization member

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/organizations/{organization}/members/{user} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /organizations/{organization}/members/{user}`

### Parameters

| Name           | In   | Type   | Required | Description          |
|----------------|------|--------|----------|----------------------|
| `organization` | path | string | true     | Organization ID      |
| `user`         | path | string | true     | User ID, name, or me |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

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
  "roles": [
    "string"
  ]
}
```

### Parameters

| Name           | In   | Type                                                   | Required | Description          |
|----------------|------|--------------------------------------------------------|----------|----------------------|
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
      "name": "string",
      "organization_id": "string"
    }
  ],
  "updated_at": "2019-08-24T14:15:22Z",
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                               |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------|
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
    "organization_permissions": [
      {
        "action": "application_connect",
        "negate": true,
        "resource_type": "*"
      }
    ],
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
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.AssignableRoles](schemas.md#codersdkassignableroles) |

<h3 id="get-site-member-roles-responseschema">Response Schema</h3>

Status Code **200**

| Name                         | Type                                                     | Required | Restrictions | Description                                                                                     |
|------------------------------|----------------------------------------------------------|----------|--------------|-------------------------------------------------------------------------------------------------|
| `[array item]`               | array                                                    | false    |              |                                                                                                 |
| `» assignable`               | boolean                                                  | false    |              |                                                                                                 |
| `» built_in`                 | boolean                                                  | false    |              | Built in roles are immutable                                                                    |
| `» display_name`             | string                                                   | false    |              |                                                                                                 |
| `» name`                     | string                                                   | false    |              |                                                                                                 |
| `» organization_id`          | string(uuid)                                             | false    |              |                                                                                                 |
| `» organization_permissions` | array                                                    | false    |              | Organization permissions are specific for the organization in the field 'OrganizationID' above. |
| `»» action`                  | [codersdk.RBACAction](schemas.md#codersdkrbacaction)     | false    |              |                                                                                                 |
| `»» negate`                  | boolean                                                  | false    |              | Negate makes this a negative permission                                                         |
| `»» resource_type`           | [codersdk.RBACResource](schemas.md#codersdkrbacresource) | false    |              |                                                                                                 |
| `» site_permissions`         | array                                                    | false    |              |                                                                                                 |
| `» user_permissions`         | array                                                    | false    |              |                                                                                                 |

#### Enumerated Values

| Property        | Value                              |
|-----------------|------------------------------------|
| `action`        | `application_connect`              |
| `action`        | `assign`                           |
| `action`        | `create`                           |
| `action`        | `delete`                           |
| `action`        | `read`                             |
| `action`        | `read_personal`                    |
| `action`        | `ssh`                              |
| `action`        | `update`                           |
| `action`        | `update_personal`                  |
| `action`        | `use`                              |
| `action`        | `view_insights`                    |
| `action`        | `start`                            |
| `action`        | `stop`                             |
| `resource_type` | `*`                                |
| `resource_type` | `api_key`                          |
| `resource_type` | `assign_org_role`                  |
| `resource_type` | `assign_role`                      |
| `resource_type` | `audit_log`                        |
| `resource_type` | `crypto_key`                       |
| `resource_type` | `debug_info`                       |
| `resource_type` | `deployment_config`                |
| `resource_type` | `deployment_stats`                 |
| `resource_type` | `file`                             |
| `resource_type` | `group`                            |
| `resource_type` | `group_member`                     |
| `resource_type` | `idpsync_settings`                 |
| `resource_type` | `license`                          |
| `resource_type` | `notification_message`             |
| `resource_type` | `notification_preference`          |
| `resource_type` | `notification_template`            |
| `resource_type` | `oauth2_app`                       |
| `resource_type` | `oauth2_app_code_token`            |
| `resource_type` | `oauth2_app_secret`                |
| `resource_type` | `organization`                     |
| `resource_type` | `organization_member`              |
| `resource_type` | `provisioner_daemon`               |
| `resource_type` | `provisioner_jobs`                 |
| `resource_type` | `provisioner_keys`                 |
| `resource_type` | `replicas`                         |
| `resource_type` | `system`                           |
| `resource_type` | `tailnet_coordinator`              |
| `resource_type` | `template`                         |
| `resource_type` | `user`                             |
| `resource_type` | `workspace`                        |
| `resource_type` | `workspace_agent_resource_monitor` |
| `resource_type` | `workspace_dormant`                |
| `resource_type` | `workspace_proxy`                  |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
