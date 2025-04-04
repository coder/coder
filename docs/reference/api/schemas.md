# Schemas

## agentsdk.AWSInstanceIdentityToken

```json
{
  "document": "string",
  "signature": "string"
}
```

### Properties

| Name        | Type   | Required | Restrictions | Description |
|-------------|--------|----------|--------------|-------------|
| `document`  | string | true     |              |             |
| `signature` | string | true     |              |             |

## agentsdk.AuthenticateResponse

```json
{
  "session_token": "string"
}
```

### Properties

| Name            | Type   | Required | Restrictions | Description |
|-----------------|--------|----------|--------------|-------------|
| `session_token` | string | false    |              |             |

## agentsdk.AzureInstanceIdentityToken

```json
{
  "encoding": "string",
  "signature": "string"
}
```

### Properties

| Name        | Type   | Required | Restrictions | Description |
|-------------|--------|----------|--------------|-------------|
| `encoding`  | string | true     |              |             |
| `signature` | string | true     |              |             |

## agentsdk.ExternalAuthResponse

```json
{
  "access_token": "string",
  "password": "string",
  "token_extra": {},
  "type": "string",
  "url": "string",
  "username": "string"
}
```

### Properties

| Name           | Type   | Required | Restrictions | Description                                                                              |
|----------------|--------|----------|--------------|------------------------------------------------------------------------------------------|
| `access_token` | string | false    |              |                                                                                          |
| `password`     | string | false    |              |                                                                                          |
| `token_extra`  | object | false    |              |                                                                                          |
| `type`         | string | false    |              |                                                                                          |
| `url`          | string | false    |              |                                                                                          |
| `username`     | string | false    |              | Deprecated: Only supported on `/workspaceagents/me/gitauth` for backwards compatibility. |

## agentsdk.GitSSHKey

```json
{
  "private_key": "string",
  "public_key": "string"
}
```

### Properties

| Name          | Type   | Required | Restrictions | Description |
|---------------|--------|----------|--------------|-------------|
| `private_key` | string | false    |              |             |
| `public_key`  | string | false    |              |             |

## agentsdk.GoogleInstanceIdentityToken

```json
{
  "json_web_token": "string"
}
```

### Properties

| Name             | Type   | Required | Restrictions | Description |
|------------------|--------|----------|--------------|-------------|
| `json_web_token` | string | true     |              |             |

## agentsdk.Log

```json
{
  "created_at": "string",
  "level": "trace",
  "output": "string"
}
```

### Properties

| Name         | Type                                   | Required | Restrictions | Description |
|--------------|----------------------------------------|----------|--------------|-------------|
| `created_at` | string                                 | false    |              |             |
| `level`      | [codersdk.LogLevel](#codersdkloglevel) | false    |              |             |
| `output`     | string                                 | false    |              |             |

## agentsdk.PatchAppStatus

```json
{
  "app_slug": "string",
  "icon": "string",
  "message": "string",
  "needs_user_attention": true,
  "state": "working",
  "uri": "string"
}
```

### Properties

| Name                   | Type                                                                 | Required | Restrictions | Description |
|------------------------|----------------------------------------------------------------------|----------|--------------|-------------|
| `app_slug`             | string                                                               | false    |              |             |
| `icon`                 | string                                                               | false    |              |             |
| `message`              | string                                                               | false    |              |             |
| `needs_user_attention` | boolean                                                              | false    |              |             |
| `state`                | [codersdk.WorkspaceAppStatusState](#codersdkworkspaceappstatusstate) | false    |              |             |
| `uri`                  | string                                                               | false    |              |             |

## agentsdk.PatchLogs

```json
{
  "log_source_id": "string",
  "logs": [
    {
      "created_at": "string",
      "level": "trace",
      "output": "string"
    }
  ]
}
```

### Properties

| Name            | Type                                  | Required | Restrictions | Description |
|-----------------|---------------------------------------|----------|--------------|-------------|
| `log_source_id` | string                                | false    |              |             |
| `logs`          | array of [agentsdk.Log](#agentsdklog) | false    |              |             |

## agentsdk.PostLogSourceRequest

```json
{
  "display_name": "string",
  "icon": "string",
  "id": "string"
}
```

### Properties

| Name           | Type   | Required | Restrictions | Description                                                                                                                                                                                    |
|----------------|--------|----------|--------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `display_name` | string | false    |              |                                                                                                                                                                                                |
| `icon`         | string | false    |              |                                                                                                                                                                                                |
| `id`           | string | false    |              | ID is a unique identifier for the log source. It is scoped to a workspace agent, and can be statically defined inside code to prevent duplicate sources from being created for the same agent. |

## coderd.SCIMUser

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

### Properties

| Name             | Type               | Required | Restrictions | Description                                                                 |
|------------------|--------------------|----------|--------------|-----------------------------------------------------------------------------|
| `active`         | boolean            | false    |              | Active is a ptr to prevent the empty value from being interpreted as false. |
| `emails`         | array of object    | false    |              |                                                                             |
| `» display`      | string             | false    |              |                                                                             |
| `» primary`      | boolean            | false    |              |                                                                             |
| `» type`         | string             | false    |              |                                                                             |
| `» value`        | string             | false    |              |                                                                             |
| `groups`         | array of undefined | false    |              |                                                                             |
| `id`             | string             | false    |              |                                                                             |
| `meta`           | object             | false    |              |                                                                             |
| `» resourceType` | string             | false    |              |                                                                             |
| `name`           | object             | false    |              |                                                                             |
| `» familyName`   | string             | false    |              |                                                                             |
| `» givenName`    | string             | false    |              |                                                                             |
| `schemas`        | array of string    | false    |              |                                                                             |
| `userName`       | string             | false    |              |                                                                             |

## coderd.cspViolation

```json
{
  "csp-report": {}
}
```

### Properties

| Name         | Type   | Required | Restrictions | Description |
|--------------|--------|----------|--------------|-------------|
| `csp-report` | object | false    |              |             |

## codersdk.ACLAvailable

```json
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
```

### Properties

| Name     | Type                                                  | Required | Restrictions | Description |
|----------|-------------------------------------------------------|----------|--------------|-------------|
| `groups` | array of [codersdk.Group](#codersdkgroup)             | false    |              |             |
| `users`  | array of [codersdk.ReducedUser](#codersdkreduceduser) | false    |              |             |

## codersdk.APIKey

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "expires_at": "2019-08-24T14:15:22Z",
  "id": "string",
  "last_used": "2019-08-24T14:15:22Z",
  "lifetime_seconds": 0,
  "login_type": "password",
  "scope": "all",
  "token_name": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
}
```

### Properties

| Name               | Type                                         | Required | Restrictions | Description |
|--------------------|----------------------------------------------|----------|--------------|-------------|
| `created_at`       | string                                       | true     |              |             |
| `expires_at`       | string                                       | true     |              |             |
| `id`               | string                                       | true     |              |             |
| `last_used`        | string                                       | true     |              |             |
| `lifetime_seconds` | integer                                      | true     |              |             |
| `login_type`       | [codersdk.LoginType](#codersdklogintype)     | true     |              |             |
| `scope`            | [codersdk.APIKeyScope](#codersdkapikeyscope) | true     |              |             |
| `token_name`       | string                                       | true     |              |             |
| `updated_at`       | string                                       | true     |              |             |
| `user_id`          | string                                       | true     |              |             |

#### Enumerated Values

| Property     | Value                 |
|--------------|-----------------------|
| `login_type` | `password`            |
| `login_type` | `github`              |
| `login_type` | `oidc`                |
| `login_type` | `token`               |
| `scope`      | `all`                 |
| `scope`      | `application_connect` |

## codersdk.APIKeyScope

```json
"all"
```

### Properties

#### Enumerated Values

| Value                 |
|-----------------------|
| `all`                 |
| `application_connect` |

## codersdk.AddLicenseRequest

```json
{
  "license": "string"
}
```

### Properties

| Name      | Type   | Required | Restrictions | Description |
|-----------|--------|----------|--------------|-------------|
| `license` | string | true     |              |             |

## codersdk.AgentConnectionTiming

```json
{
  "ended_at": "2019-08-24T14:15:22Z",
  "stage": "init",
  "started_at": "2019-08-24T14:15:22Z",
  "workspace_agent_id": "string",
  "workspace_agent_name": "string"
}
```

### Properties

| Name                   | Type                                         | Required | Restrictions | Description |
|------------------------|----------------------------------------------|----------|--------------|-------------|
| `ended_at`             | string                                       | false    |              |             |
| `stage`                | [codersdk.TimingStage](#codersdktimingstage) | false    |              |             |
| `started_at`           | string                                       | false    |              |             |
| `workspace_agent_id`   | string                                       | false    |              |             |
| `workspace_agent_name` | string                                       | false    |              |             |

## codersdk.AgentScriptTiming

```json
{
  "display_name": "string",
  "ended_at": "2019-08-24T14:15:22Z",
  "exit_code": 0,
  "stage": "init",
  "started_at": "2019-08-24T14:15:22Z",
  "status": "string",
  "workspace_agent_id": "string",
  "workspace_agent_name": "string"
}
```

### Properties

| Name                   | Type                                         | Required | Restrictions | Description |
|------------------------|----------------------------------------------|----------|--------------|-------------|
| `display_name`         | string                                       | false    |              |             |
| `ended_at`             | string                                       | false    |              |             |
| `exit_code`            | integer                                      | false    |              |             |
| `stage`                | [codersdk.TimingStage](#codersdktimingstage) | false    |              |             |
| `started_at`           | string                                       | false    |              |             |
| `status`               | string                                       | false    |              |             |
| `workspace_agent_id`   | string                                       | false    |              |             |
| `workspace_agent_name` | string                                       | false    |              |             |

## codersdk.AgentSubsystem

```json
"envbox"
```

### Properties

#### Enumerated Values

| Value        |
|--------------|
| `envbox`     |
| `envbuilder` |
| `exectrace`  |

## codersdk.AppHostResponse

```json
{
  "host": "string"
}
```

### Properties

| Name   | Type   | Required | Restrictions | Description                                                   |
|--------|--------|----------|--------------|---------------------------------------------------------------|
| `host` | string | false    |              | Host is the externally accessible URL for the Coder instance. |

## codersdk.AppearanceConfig

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

### Properties

| Name                   | Type                                                    | Required | Restrictions | Description                                                         |
|------------------------|---------------------------------------------------------|----------|--------------|---------------------------------------------------------------------|
| `announcement_banners` | array of [codersdk.BannerConfig](#codersdkbannerconfig) | false    |              |                                                                     |
| `application_name`     | string                                                  | false    |              |                                                                     |
| `docs_url`             | string                                                  | false    |              |                                                                     |
| `logo_url`             | string                                                  | false    |              |                                                                     |
| `service_banner`       | [codersdk.BannerConfig](#codersdkbannerconfig)          | false    |              | Deprecated: ServiceBanner has been replaced by AnnouncementBanners. |
| `support_links`        | array of [codersdk.LinkConfig](#codersdklinkconfig)     | false    |              |                                                                     |

## codersdk.ArchiveTemplateVersionsRequest

```json
{
  "all": true
}
```

### Properties

| Name  | Type    | Required | Restrictions | Description                                                                                                              |
|-------|---------|----------|--------------|--------------------------------------------------------------------------------------------------------------------------|
| `all` | boolean | false    |              | By default, only failed versions are archived. Set this to true to archive all unused versions regardless of job status. |

## codersdk.AssignableRoles

```json
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
```

### Properties

| Name                       | Type                                                | Required | Restrictions | Description                                                                                     |
|----------------------------|-----------------------------------------------------|----------|--------------|-------------------------------------------------------------------------------------------------|
| `assignable`               | boolean                                             | false    |              |                                                                                                 |
| `built_in`                 | boolean                                             | false    |              | Built in roles are immutable                                                                    |
| `display_name`             | string                                              | false    |              |                                                                                                 |
| `name`                     | string                                              | false    |              |                                                                                                 |
| `organization_id`          | string                                              | false    |              |                                                                                                 |
| `organization_permissions` | array of [codersdk.Permission](#codersdkpermission) | false    |              | Organization permissions are specific for the organization in the field 'OrganizationID' above. |
| `site_permissions`         | array of [codersdk.Permission](#codersdkpermission) | false    |              |                                                                                                 |
| `user_permissions`         | array of [codersdk.Permission](#codersdkpermission) | false    |              |                                                                                                 |

## codersdk.AuditAction

```json
"create"
```

### Properties

#### Enumerated Values

| Value                    |
|--------------------------|
| `create`                 |
| `write`                  |
| `delete`                 |
| `start`                  |
| `stop`                   |
| `login`                  |
| `logout`                 |
| `register`               |
| `request_password_reset` |
| `connect`                |
| `disconnect`             |
| `open`                   |
| `close`                  |

## codersdk.AuditDiff

```json
{
  "property1": {
    "new": null,
    "old": null,
    "secret": true
  },
  "property2": {
    "new": null,
    "old": null,
    "secret": true
  }
}
```

### Properties

| Name             | Type                                               | Required | Restrictions | Description |
|------------------|----------------------------------------------------|----------|--------------|-------------|
| `[any property]` | [codersdk.AuditDiffField](#codersdkauditdifffield) | false    |              |             |

## codersdk.AuditDiffField

```json
{
  "new": null,
  "old": null,
  "secret": true
}
```

### Properties

| Name     | Type    | Required | Restrictions | Description |
|----------|---------|----------|--------------|-------------|
| `new`    | any     | false    |              |             |
| `old`    | any     | false    |              |             |
| `secret` | boolean | false    |              |             |

## codersdk.AuditLog

```json
{
  "action": "create",
  "additional_fields": [
    0
  ],
  "description": "string",
  "diff": {
    "property1": {
      "new": null,
      "old": null,
      "secret": true
    },
    "property2": {
      "new": null,
      "old": null,
      "secret": true
    }
  },
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "ip": "string",
  "is_deleted": true,
  "organization": {
    "display_name": "string",
    "icon": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "name": "string"
  },
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "request_id": "266ea41d-adf5-480b-af50-15b940c2b846",
  "resource_icon": "string",
  "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
  "resource_link": "string",
  "resource_target": "string",
  "resource_type": "template",
  "status_code": 0,
  "time": "2019-08-24T14:15:22Z",
  "user": {
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
  },
  "user_agent": "string"
}
```

### Properties

| Name                | Type                                                         | Required | Restrictions | Description                                  |
|---------------------|--------------------------------------------------------------|----------|--------------|----------------------------------------------|
| `action`            | [codersdk.AuditAction](#codersdkauditaction)                 | false    |              |                                              |
| `additional_fields` | array of integer                                             | false    |              |                                              |
| `description`       | string                                                       | false    |              |                                              |
| `diff`              | [codersdk.AuditDiff](#codersdkauditdiff)                     | false    |              |                                              |
| `id`                | string                                                       | false    |              |                                              |
| `ip`                | string                                                       | false    |              |                                              |
| `is_deleted`        | boolean                                                      | false    |              |                                              |
| `organization`      | [codersdk.MinimalOrganization](#codersdkminimalorganization) | false    |              |                                              |
| `organization_id`   | string                                                       | false    |              | Deprecated: Use 'organization.id' instead.   |
| `request_id`        | string                                                       | false    |              |                                              |
| `resource_icon`     | string                                                       | false    |              |                                              |
| `resource_id`       | string                                                       | false    |              |                                              |
| `resource_link`     | string                                                       | false    |              |                                              |
| `resource_target`   | string                                                       | false    |              | Resource target is the name of the resource. |
| `resource_type`     | [codersdk.ResourceType](#codersdkresourcetype)               | false    |              |                                              |
| `status_code`       | integer                                                      | false    |              |                                              |
| `time`              | string                                                       | false    |              |                                              |
| `user`              | [codersdk.User](#codersdkuser)                               | false    |              |                                              |
| `user_agent`        | string                                                       | false    |              |                                              |

## codersdk.AuditLogResponse

```json
{
  "audit_logs": [
    {
      "action": "create",
      "additional_fields": [
        0
      ],
      "description": "string",
      "diff": {
        "property1": {
          "new": null,
          "old": null,
          "secret": true
        },
        "property2": {
          "new": null,
          "old": null,
          "secret": true
        }
      },
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "ip": "string",
      "is_deleted": true,
      "organization": {
        "display_name": "string",
        "icon": "string",
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "name": "string"
      },
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "request_id": "266ea41d-adf5-480b-af50-15b940c2b846",
      "resource_icon": "string",
      "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
      "resource_link": "string",
      "resource_target": "string",
      "resource_type": "template",
      "status_code": 0,
      "time": "2019-08-24T14:15:22Z",
      "user": {
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
      },
      "user_agent": "string"
    }
  ],
  "count": 0
}
```

### Properties

| Name         | Type                                            | Required | Restrictions | Description |
|--------------|-------------------------------------------------|----------|--------------|-------------|
| `audit_logs` | array of [codersdk.AuditLog](#codersdkauditlog) | false    |              |             |
| `count`      | integer                                         | false    |              |             |

## codersdk.AuthMethod

```json
{
  "enabled": true
}
```

### Properties

| Name      | Type    | Required | Restrictions | Description |
|-----------|---------|----------|--------------|-------------|
| `enabled` | boolean | false    |              |             |

## codersdk.AuthMethods

```json
{
  "github": {
    "default_provider_configured": true,
    "enabled": true
  },
  "oidc": {
    "enabled": true,
    "iconUrl": "string",
    "signInText": "string"
  },
  "password": {
    "enabled": true
  },
  "terms_of_service_url": "string"
}
```

### Properties

| Name                   | Type                                                   | Required | Restrictions | Description |
|------------------------|--------------------------------------------------------|----------|--------------|-------------|
| `github`               | [codersdk.GithubAuthMethod](#codersdkgithubauthmethod) | false    |              |             |
| `oidc`                 | [codersdk.OIDCAuthMethod](#codersdkoidcauthmethod)     | false    |              |             |
| `password`             | [codersdk.AuthMethod](#codersdkauthmethod)             | false    |              |             |
| `terms_of_service_url` | string                                                 | false    |              |             |

## codersdk.AuthorizationCheck

```json
{
  "action": "create",
  "object": {
    "any_org": true,
    "organization_id": "string",
    "owner_id": "string",
    "resource_id": "string",
    "resource_type": "*"
  }
}
```

AuthorizationCheck is used to check if the currently authenticated user (or the specified user) can do a given action to a given set of objects.

### Properties

| Name     | Type                                                         | Required | Restrictions | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
|----------|--------------------------------------------------------------|----------|--------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `action` | [codersdk.RBACAction](#codersdkrbacaction)                   | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `object` | [codersdk.AuthorizationObject](#codersdkauthorizationobject) | false    |              | Object can represent a "set" of objects, such as: all workspaces in an organization, all workspaces owned by me, and all workspaces across the entire product. When defining an object, use the most specific language when possible to produce the smallest set. Meaning to set as many fields on 'Object' as you can. Example, if you want to check if you can update all workspaces owned by 'me', try to also add an 'OrganizationID' to the settings. Omitting the 'OrganizationID' could produce the incorrect value, as workspaces have both `user` and `organization` owners. |

#### Enumerated Values

| Property | Value    |
|----------|----------|
| `action` | `create` |
| `action` | `read`   |
| `action` | `update` |
| `action` | `delete` |

## codersdk.AuthorizationObject

```json
{
  "any_org": true,
  "organization_id": "string",
  "owner_id": "string",
  "resource_id": "string",
  "resource_type": "*"
}
```

AuthorizationObject can represent a "set" of objects, such as: all workspaces in an organization, all workspaces owned by me, all workspaces across the entire product.

### Properties

| Name              | Type                                           | Required | Restrictions | Description                                                                                                                                                                                                                                                                                                                                                          |
|-------------------|------------------------------------------------|----------|--------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `any_org`         | boolean                                        | false    |              | Any org (optional) will disregard the org_owner when checking for permissions. This cannot be set to true if the OrganizationID is set.                                                                                                                                                                                                                              |
| `organization_id` | string                                         | false    |              | Organization ID (optional) adds the set constraint to all resources owned by a given organization.                                                                                                                                                                                                                                                                   |
| `owner_id`        | string                                         | false    |              | Owner ID (optional) adds the set constraint to all resources owned by a given user.                                                                                                                                                                                                                                                                                  |
| `resource_id`     | string                                         | false    |              | Resource ID (optional) reduces the set to a singular resource. This assigns a resource ID to the resource type, eg: a single workspace. The rbac library will not fetch the resource from the database, so if you are using this option, you should also set the owner ID and organization ID if possible. Be as specific as possible using all the fields relevant. |
| `resource_type`   | [codersdk.RBACResource](#codersdkrbacresource) | false    |              | Resource type is the name of the resource. `./coderd/rbac/object.go` has the list of valid resource types.                                                                                                                                                                                                                                                           |

## codersdk.AuthorizationRequest

```json
{
  "checks": {
    "property1": {
      "action": "create",
      "object": {
        "any_org": true,
        "organization_id": "string",
        "owner_id": "string",
        "resource_id": "string",
        "resource_type": "*"
      }
    },
    "property2": {
      "action": "create",
      "object": {
        "any_org": true,
        "organization_id": "string",
        "owner_id": "string",
        "resource_id": "string",
        "resource_type": "*"
      }
    }
  }
}
```

### Properties

| Name               | Type                                                       | Required | Restrictions | Description                                                                                                                                                                                                                                                                      |
|--------------------|------------------------------------------------------------|----------|--------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `checks`           | object                                                     | false    |              | Checks is a map keyed with an arbitrary string to a permission check. The key can be any string that is helpful to the caller, and allows multiple permission checks to be run in a single request. The key ensures that each permission check has the same key in the response. |
| » `[any property]` | [codersdk.AuthorizationCheck](#codersdkauthorizationcheck) | false    |              | It is used to check if the currently authenticated user (or the specified user) can do a given action to a given set of objects.                                                                                                                                                 |

## codersdk.AuthorizationResponse

```json
{
  "property1": true,
  "property2": true
}
```

### Properties

| Name             | Type    | Required | Restrictions | Description |
|------------------|---------|----------|--------------|-------------|
| `[any property]` | boolean | false    |              |             |

## codersdk.AutomaticUpdates

```json
"always"
```

### Properties

#### Enumerated Values

| Value    |
|----------|
| `always` |
| `never`  |

## codersdk.BannerConfig

```json
{
  "background_color": "string",
  "enabled": true,
  "message": "string"
}
```

### Properties

| Name               | Type    | Required | Restrictions | Description |
|--------------------|---------|----------|--------------|-------------|
| `background_color` | string  | false    |              |             |
| `enabled`          | boolean | false    |              |             |
| `message`          | string  | false    |              |             |

## codersdk.BuildInfoResponse

```json
{
  "agent_api_version": "string",
  "dashboard_url": "string",
  "deployment_id": "string",
  "external_url": "string",
  "provisioner_api_version": "string",
  "telemetry": true,
  "upgrade_message": "string",
  "version": "string",
  "webpush_public_key": "string",
  "workspace_proxy": true
}
```

### Properties

| Name                      | Type    | Required | Restrictions | Description                                                                                                                                                         |
|---------------------------|---------|----------|--------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `agent_api_version`       | string  | false    |              | Agent api version is the current version of the Agent API (back versions MAY still be supported).                                                                   |
| `dashboard_url`           | string  | false    |              | Dashboard URL is the URL to hit the deployment's dashboard. For external workspace proxies, this is the coderd they are connected to.                               |
| `deployment_id`           | string  | false    |              | Deployment ID is the unique identifier for this deployment.                                                                                                         |
| `external_url`            | string  | false    |              | External URL references the current Coder version. For production builds, this will link directly to a release. For development builds, this will link to a commit. |
| `provisioner_api_version` | string  | false    |              | Provisioner api version is the current version of the Provisioner API                                                                                               |
| `telemetry`               | boolean | false    |              | Telemetry is a boolean that indicates whether telemetry is enabled.                                                                                                 |
| `upgrade_message`         | string  | false    |              | Upgrade message is the message displayed to users when an outdated client is detected.                                                                              |
| `version`                 | string  | false    |              | Version returns the semantic version of the build.                                                                                                                  |
| `webpush_public_key`      | string  | false    |              | Webpush public key is the public key for push notifications via Web Push.                                                                                           |
| `workspace_proxy`         | boolean | false    |              |                                                                                                                                                                     |

## codersdk.BuildReason

```json
"initiator"
```

### Properties

#### Enumerated Values

| Value       |
|-------------|
| `initiator` |
| `autostart` |
| `autostop`  |

## codersdk.ChangePasswordWithOneTimePasscodeRequest

```json
{
  "email": "user@example.com",
  "one_time_passcode": "string",
  "password": "string"
}
```

### Properties

| Name                | Type   | Required | Restrictions | Description |
|---------------------|--------|----------|--------------|-------------|
| `email`             | string | true     |              |             |
| `one_time_passcode` | string | true     |              |             |
| `password`          | string | true     |              |             |

## codersdk.ConnectionLatency

```json
{
  "p50": 31.312,
  "p95": 119.832
}
```

### Properties

| Name  | Type   | Required | Restrictions | Description |
|-------|--------|----------|--------------|-------------|
| `p50` | number | false    |              |             |
| `p95` | number | false    |              |             |

## codersdk.ConvertLoginRequest

```json
{
  "password": "string",
  "to_type": ""
}
```

### Properties

| Name       | Type                                     | Required | Restrictions | Description                              |
|------------|------------------------------------------|----------|--------------|------------------------------------------|
| `password` | string                                   | true     |              |                                          |
| `to_type`  | [codersdk.LoginType](#codersdklogintype) | true     |              | To type is the login type to convert to. |

## codersdk.CreateFirstUserRequest

```json
{
  "email": "string",
  "name": "string",
  "password": "string",
  "trial": true,
  "trial_info": {
    "company_name": "string",
    "country": "string",
    "developers": "string",
    "first_name": "string",
    "job_title": "string",
    "last_name": "string",
    "phone_number": "string"
  },
  "username": "string"
}
```

### Properties

| Name         | Type                                                                   | Required | Restrictions | Description |
|--------------|------------------------------------------------------------------------|----------|--------------|-------------|
| `email`      | string                                                                 | true     |              |             |
| `name`       | string                                                                 | false    |              |             |
| `password`   | string                                                                 | true     |              |             |
| `trial`      | boolean                                                                | false    |              |             |
| `trial_info` | [codersdk.CreateFirstUserTrialInfo](#codersdkcreatefirstusertrialinfo) | false    |              |             |
| `username`   | string                                                                 | true     |              |             |

## codersdk.CreateFirstUserResponse

```json
{
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
}
```

### Properties

| Name              | Type   | Required | Restrictions | Description |
|-------------------|--------|----------|--------------|-------------|
| `organization_id` | string | false    |              |             |
| `user_id`         | string | false    |              |             |

## codersdk.CreateFirstUserTrialInfo

```json
{
  "company_name": "string",
  "country": "string",
  "developers": "string",
  "first_name": "string",
  "job_title": "string",
  "last_name": "string",
  "phone_number": "string"
}
```

### Properties

| Name           | Type   | Required | Restrictions | Description |
|----------------|--------|----------|--------------|-------------|
| `company_name` | string | false    |              |             |
| `country`      | string | false    |              |             |
| `developers`   | string | false    |              |             |
| `first_name`   | string | false    |              |             |
| `job_title`    | string | false    |              |             |
| `last_name`    | string | false    |              |             |
| `phone_number` | string | false    |              |             |

## codersdk.CreateGroupRequest

```json
{
  "avatar_url": "string",
  "display_name": "string",
  "name": "string",
  "quota_allowance": 0
}
```

### Properties

| Name              | Type    | Required | Restrictions | Description |
|-------------------|---------|----------|--------------|-------------|
| `avatar_url`      | string  | false    |              |             |
| `display_name`    | string  | false    |              |             |
| `name`            | string  | true     |              |             |
| `quota_allowance` | integer | false    |              |             |

## codersdk.CreateOrganizationRequest

```json
{
  "description": "string",
  "display_name": "string",
  "icon": "string",
  "name": "string"
}
```

### Properties

| Name           | Type   | Required | Restrictions | Description                                                            |
|----------------|--------|----------|--------------|------------------------------------------------------------------------|
| `description`  | string | false    |              |                                                                        |
| `display_name` | string | false    |              | Display name will default to the same value as `Name` if not provided. |
| `icon`         | string | false    |              |                                                                        |
| `name`         | string | true     |              |                                                                        |

## codersdk.CreateProvisionerKeyResponse

```json
{
  "key": "string"
}
```

### Properties

| Name  | Type   | Required | Restrictions | Description |
|-------|--------|----------|--------------|-------------|
| `key` | string | false    |              |             |

## codersdk.CreateTemplateRequest

```json
{
  "activity_bump_ms": 0,
  "allow_user_autostart": true,
  "allow_user_autostop": true,
  "allow_user_cancel_workspace_jobs": true,
  "autostart_requirement": {
    "days_of_week": [
      "monday"
    ]
  },
  "autostop_requirement": {
    "days_of_week": [
      "monday"
    ],
    "weeks": 0
  },
  "default_ttl_ms": 0,
  "delete_ttl_ms": 0,
  "description": "string",
  "disable_everyone_group_access": true,
  "display_name": "string",
  "dormant_ttl_ms": 0,
  "failure_ttl_ms": 0,
  "icon": "string",
  "max_port_share_level": "owner",
  "name": "string",
  "require_active_version": true,
  "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1"
}
```

### Properties

| Name                               | Type                                                                           | Required | Restrictions | Description                                                                                                                                                                                                                                                                                                         |
|------------------------------------|--------------------------------------------------------------------------------|----------|--------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `activity_bump_ms`                 | integer                                                                        | false    |              | Activity bump ms allows optionally specifying the activity bump duration for all workspaces created from this template. Defaults to 1h but can be set to 0 to disable activity bumping.                                                                                                                             |
| `allow_user_autostart`             | boolean                                                                        | false    |              | Allow user autostart allows users to set a schedule for autostarting their workspace. By default this is true. This can only be disabled when using an enterprise license.                                                                                                                                          |
| `allow_user_autostop`              | boolean                                                                        | false    |              | Allow user autostop allows users to set a custom workspace TTL to use in place of the template's DefaultTTL field. By default this is true. If false, the DefaultTTL will always be used. This can only be disabled when using an enterprise license.                                                               |
| `allow_user_cancel_workspace_jobs` | boolean                                                                        | false    |              | Allow users to cancel in-progress workspace jobs. *bool as the default value is "true".                                                                                                                                                                                                                             |
| `autostart_requirement`            | [codersdk.TemplateAutostartRequirement](#codersdktemplateautostartrequirement) | false    |              | Autostart requirement allows optionally specifying the autostart allowed days for workspaces created from this template. This is an enterprise feature.                                                                                                                                                             |
| `autostop_requirement`             | [codersdk.TemplateAutostopRequirement](#codersdktemplateautostoprequirement)   | false    |              | Autostop requirement allows optionally specifying the autostop requirement for workspaces created from this template. This is an enterprise feature.                                                                                                                                                                |
| `default_ttl_ms`                   | integer                                                                        | false    |              | Default ttl ms allows optionally specifying the default TTL for all workspaces created from this template.                                                                                                                                                                                                          |
| `delete_ttl_ms`                    | integer                                                                        | false    |              | Delete ttl ms allows optionally specifying the max lifetime before Coder permanently deletes dormant workspaces created from this template.                                                                                                                                                                         |
| `description`                      | string                                                                         | false    |              | Description is a description of what the template contains. It must be less than 128 bytes.                                                                                                                                                                                                                         |
| `disable_everyone_group_access`    | boolean                                                                        | false    |              | Disable everyone group access allows optionally disabling the default behavior of granting the 'everyone' group access to use the template. If this is set to true, the template will not be available to all users, and must be explicitly granted to users or groups in the permissions settings of the template. |
| `display_name`                     | string                                                                         | false    |              | Display name is the displayed name of the template.                                                                                                                                                                                                                                                                 |
| `dormant_ttl_ms`                   | integer                                                                        | false    |              | Dormant ttl ms allows optionally specifying the max lifetime before Coder locks inactive workspaces created from this template.                                                                                                                                                                                     |
| `failure_ttl_ms`                   | integer                                                                        | false    |              | Failure ttl ms allows optionally specifying the max lifetime before Coder stops all resources for failed workspaces created from this template.                                                                                                                                                                     |
| `icon`                             | string                                                                         | false    |              | Icon is a relative path or external URL that specifies an icon to be displayed in the dashboard.                                                                                                                                                                                                                    |
| `max_port_share_level`             | [codersdk.WorkspaceAgentPortShareLevel](#codersdkworkspaceagentportsharelevel) | false    |              | Max port share level allows optionally specifying the maximum port share level for workspaces created from the template.                                                                                                                                                                                            |
| `name`                             | string                                                                         | true     |              | Name is the name of the template.                                                                                                                                                                                                                                                                                   |
| `require_active_version`           | boolean                                                                        | false    |              | Require active version mandates that workspaces are built with the active template version.                                                                                                                                                                                                                         |
|`template_version_id`|string|true||Template version ID is an in-progress or completed job to use as an initial version of the template.
This is required on creation to enable a user-flow of validating a template works. There is no reason the data-model cannot support empty templates, but it doesn't make sense for users.|

## codersdk.CreateTemplateVersionDryRunRequest

```json
{
  "rich_parameter_values": [
    {
      "name": "string",
      "value": "string"
    }
  ],
  "user_variable_values": [
    {
      "name": "string",
      "value": "string"
    }
  ],
  "workspace_name": "string"
}
```

### Properties

| Name                    | Type                                                                          | Required | Restrictions | Description |
|-------------------------|-------------------------------------------------------------------------------|----------|--------------|-------------|
| `rich_parameter_values` | array of [codersdk.WorkspaceBuildParameter](#codersdkworkspacebuildparameter) | false    |              |             |
| `user_variable_values`  | array of [codersdk.VariableValue](#codersdkvariablevalue)                     | false    |              |             |
| `workspace_name`        | string                                                                        | false    |              |             |

## codersdk.CreateTemplateVersionRequest

```json
{
  "example_id": "string",
  "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
  "message": "string",
  "name": "string",
  "provisioner": "terraform",
  "storage_method": "file",
  "tags": {
    "property1": "string",
    "property2": "string"
  },
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "user_variable_values": [
    {
      "name": "string",
      "value": "string"
    }
  ]
}
```

### Properties

| Name                   | Type                                                                   | Required | Restrictions | Description                                                  |
|------------------------|------------------------------------------------------------------------|----------|--------------|--------------------------------------------------------------|
| `example_id`           | string                                                                 | false    |              |                                                              |
| `file_id`              | string                                                                 | false    |              |                                                              |
| `message`              | string                                                                 | false    |              |                                                              |
| `name`                 | string                                                                 | false    |              |                                                              |
| `provisioner`          | string                                                                 | true     |              |                                                              |
| `storage_method`       | [codersdk.ProvisionerStorageMethod](#codersdkprovisionerstoragemethod) | true     |              |                                                              |
| `tags`                 | object                                                                 | false    |              |                                                              |
| » `[any property]`     | string                                                                 | false    |              |                                                              |
| `template_id`          | string                                                                 | false    |              | Template ID optionally associates a version with a template. |
| `user_variable_values` | array of [codersdk.VariableValue](#codersdkvariablevalue)              | false    |              |                                                              |

#### Enumerated Values

| Property         | Value       |
|------------------|-------------|
| `provisioner`    | `terraform` |
| `provisioner`    | `echo`      |
| `storage_method` | `file`      |

## codersdk.CreateTestAuditLogRequest

```json
{
  "action": "create",
  "additional_fields": [
    0
  ],
  "build_reason": "autostart",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "request_id": "266ea41d-adf5-480b-af50-15b940c2b846",
  "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
  "resource_type": "template",
  "time": "2019-08-24T14:15:22Z"
}
```

### Properties

| Name                | Type                                           | Required | Restrictions | Description |
|---------------------|------------------------------------------------|----------|--------------|-------------|
| `action`            | [codersdk.AuditAction](#codersdkauditaction)   | false    |              |             |
| `additional_fields` | array of integer                               | false    |              |             |
| `build_reason`      | [codersdk.BuildReason](#codersdkbuildreason)   | false    |              |             |
| `organization_id`   | string                                         | false    |              |             |
| `request_id`        | string                                         | false    |              |             |
| `resource_id`       | string                                         | false    |              |             |
| `resource_type`     | [codersdk.ResourceType](#codersdkresourcetype) | false    |              |             |
| `time`              | string                                         | false    |              |             |

#### Enumerated Values

| Property        | Value              |
|-----------------|--------------------|
| `action`        | `create`           |
| `action`        | `write`            |
| `action`        | `delete`           |
| `action`        | `start`            |
| `action`        | `stop`             |
| `build_reason`  | `autostart`        |
| `build_reason`  | `autostop`         |
| `build_reason`  | `initiator`        |
| `resource_type` | `template`         |
| `resource_type` | `template_version` |
| `resource_type` | `user`             |
| `resource_type` | `workspace`        |
| `resource_type` | `workspace_build`  |
| `resource_type` | `git_ssh_key`      |
| `resource_type` | `auditable_group`  |

## codersdk.CreateTokenRequest

```json
{
  "lifetime": 0,
  "scope": "all",
  "token_name": "string"
}
```

### Properties

| Name         | Type                                         | Required | Restrictions | Description |
|--------------|----------------------------------------------|----------|--------------|-------------|
| `lifetime`   | integer                                      | false    |              |             |
| `scope`      | [codersdk.APIKeyScope](#codersdkapikeyscope) | false    |              |             |
| `token_name` | string                                       | false    |              |             |

#### Enumerated Values

| Property | Value                 |
|----------|-----------------------|
| `scope`  | `all`                 |
| `scope`  | `application_connect` |

## codersdk.CreateUserRequestWithOrgs

```json
{
  "email": "user@example.com",
  "login_type": "",
  "name": "string",
  "organization_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "password": "string",
  "user_status": "active",
  "username": "string"
}
```

### Properties

| Name               | Type                                       | Required | Restrictions | Description                                                                         |
|--------------------|--------------------------------------------|----------|--------------|-------------------------------------------------------------------------------------|
| `email`            | string                                     | true     |              |                                                                                     |
| `login_type`       | [codersdk.LoginType](#codersdklogintype)   | false    |              | Login type defaults to LoginTypePassword.                                           |
| `name`             | string                                     | false    |              |                                                                                     |
| `organization_ids` | array of string                            | false    |              | Organization ids is a list of organization IDs that the user should be a member of. |
| `password`         | string                                     | false    |              |                                                                                     |
| `user_status`      | [codersdk.UserStatus](#codersdkuserstatus) | false    |              | User status defaults to UserStatusDormant.                                          |
| `username`         | string                                     | true     |              |                                                                                     |

## codersdk.CreateWorkspaceBuildRequest

```json
{
  "dry_run": true,
  "log_level": "debug",
  "orphan": true,
  "rich_parameter_values": [
    {
      "name": "string",
      "value": "string"
    }
  ],
  "state": [
    0
  ],
  "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
  "transition": "start"
}
```

### Properties

| Name                    | Type                                                                          | Required | Restrictions | Description                                                                                                                                                                                                   |
|-------------------------|-------------------------------------------------------------------------------|----------|--------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `dry_run`               | boolean                                                                       | false    |              |                                                                                                                                                                                                               |
| `log_level`             | [codersdk.ProvisionerLogLevel](#codersdkprovisionerloglevel)                  | false    |              | Log level changes the default logging verbosity of a provider ("info" if empty).                                                                                                                              |
| `orphan`                | boolean                                                                       | false    |              | Orphan may be set for the Destroy transition.                                                                                                                                                                 |
| `rich_parameter_values` | array of [codersdk.WorkspaceBuildParameter](#codersdkworkspacebuildparameter) | false    |              | Rich parameter values are optional. It will write params to the 'workspace' scope. This will overwrite any existing parameters with the same name. This will not delete old params not included in this list. |
| `state`                 | array of integer                                                              | false    |              |                                                                                                                                                                                                               |
| `template_version_id`   | string                                                                        | false    |              |                                                                                                                                                                                                               |
| `transition`            | [codersdk.WorkspaceTransition](#codersdkworkspacetransition)                  | true     |              |                                                                                                                                                                                                               |

#### Enumerated Values

| Property     | Value    |
|--------------|----------|
| `log_level`  | `debug`  |
| `transition` | `start`  |
| `transition` | `stop`   |
| `transition` | `delete` |

## codersdk.CreateWorkspaceProxyRequest

```json
{
  "display_name": "string",
  "icon": "string",
  "name": "string"
}
```

### Properties

| Name           | Type   | Required | Restrictions | Description |
|----------------|--------|----------|--------------|-------------|
| `display_name` | string | false    |              |             |
| `icon`         | string | false    |              |             |
| `name`         | string | true     |              |             |

## codersdk.CreateWorkspaceRequest

```json
{
  "automatic_updates": "always",
  "autostart_schedule": "string",
  "name": "string",
  "rich_parameter_values": [
    {
      "name": "string",
      "value": "string"
    }
  ],
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
  "ttl_ms": 0
}
```

CreateWorkspaceRequest provides options for creating a new workspace. Only one of TemplateID or TemplateVersionID can be specified, not both. If TemplateID is specified, the active version of the template will be used.

### Properties

| Name                    | Type                                                                          | Required | Restrictions | Description                                                                                             |
|-------------------------|-------------------------------------------------------------------------------|----------|--------------|---------------------------------------------------------------------------------------------------------|
| `automatic_updates`     | [codersdk.AutomaticUpdates](#codersdkautomaticupdates)                        | false    |              |                                                                                                         |
| `autostart_schedule`    | string                                                                        | false    |              |                                                                                                         |
| `name`                  | string                                                                        | true     |              |                                                                                                         |
| `rich_parameter_values` | array of [codersdk.WorkspaceBuildParameter](#codersdkworkspacebuildparameter) | false    |              | Rich parameter values allows for additional parameters to be provided during the initial provision.     |
| `template_id`           | string                                                                        | false    |              | Template ID specifies which template should be used for creating the workspace.                         |
| `template_version_id`   | string                                                                        | false    |              | Template version ID can be used to specify a specific version of a template for creating the workspace. |
| `ttl_ms`                | integer                                                                       | false    |              |                                                                                                         |

## codersdk.CryptoKey

```json
{
  "deletes_at": "2019-08-24T14:15:22Z",
  "feature": "workspace_apps_api_key",
  "secret": "string",
  "sequence": 0,
  "starts_at": "2019-08-24T14:15:22Z"
}
```

### Properties

| Name         | Type                                                   | Required | Restrictions | Description |
|--------------|--------------------------------------------------------|----------|--------------|-------------|
| `deletes_at` | string                                                 | false    |              |             |
| `feature`    | [codersdk.CryptoKeyFeature](#codersdkcryptokeyfeature) | false    |              |             |
| `secret`     | string                                                 | false    |              |             |
| `sequence`   | integer                                                | false    |              |             |
| `starts_at`  | string                                                 | false    |              |             |

## codersdk.CryptoKeyFeature

```json
"workspace_apps_api_key"
```

### Properties

#### Enumerated Values

| Value                    |
|--------------------------|
| `workspace_apps_api_key` |
| `workspace_apps_token`   |
| `oidc_convert`           |
| `tailnet_resume`         |

## codersdk.CustomRoleRequest

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

### Properties

| Name                       | Type                                                | Required | Restrictions | Description                                                                    |
|----------------------------|-----------------------------------------------------|----------|--------------|--------------------------------------------------------------------------------|
| `display_name`             | string                                              | false    |              |                                                                                |
| `name`                     | string                                              | false    |              |                                                                                |
| `organization_permissions` | array of [codersdk.Permission](#codersdkpermission) | false    |              | Organization permissions are specific to the organization the role belongs to. |
| `site_permissions`         | array of [codersdk.Permission](#codersdkpermission) | false    |              |                                                                                |
| `user_permissions`         | array of [codersdk.Permission](#codersdkpermission) | false    |              |                                                                                |

## codersdk.DAUEntry

```json
{
  "amount": 0,
  "date": "string"
}
```

### Properties

| Name     | Type    | Required | Restrictions | Description                                                                              |
|----------|---------|----------|--------------|------------------------------------------------------------------------------------------|
| `amount` | integer | false    |              |                                                                                          |
| `date`   | string  | false    |              | Date is a string formatted as 2024-01-31. Timezone and time information is not included. |

## codersdk.DAUsResponse

```json
{
  "entries": [
    {
      "amount": 0,
      "date": "string"
    }
  ],
  "tz_hour_offset": 0
}
```

### Properties

| Name             | Type                                            | Required | Restrictions | Description |
|------------------|-------------------------------------------------|----------|--------------|-------------|
| `entries`        | array of [codersdk.DAUEntry](#codersdkdauentry) | false    |              |             |
| `tz_hour_offset` | integer                                         | false    |              |             |

## codersdk.DERP

```json
{
  "config": {
    "block_direct": true,
    "force_websockets": true,
    "path": "string",
    "url": "string"
  },
  "server": {
    "enable": true,
    "region_code": "string",
    "region_id": 0,
    "region_name": "string",
    "relay_url": {
      "forceQuery": true,
      "fragment": "string",
      "host": "string",
      "omitHost": true,
      "opaque": "string",
      "path": "string",
      "rawFragment": "string",
      "rawPath": "string",
      "rawQuery": "string",
      "scheme": "string",
      "user": {}
    },
    "stun_addresses": [
      "string"
    ]
  }
}
```

### Properties

| Name     | Type                                                   | Required | Restrictions | Description |
|----------|--------------------------------------------------------|----------|--------------|-------------|
| `config` | [codersdk.DERPConfig](#codersdkderpconfig)             | false    |              |             |
| `server` | [codersdk.DERPServerConfig](#codersdkderpserverconfig) | false    |              |             |

## codersdk.DERPConfig

```json
{
  "block_direct": true,
  "force_websockets": true,
  "path": "string",
  "url": "string"
}
```

### Properties

| Name               | Type    | Required | Restrictions | Description |
|--------------------|---------|----------|--------------|-------------|
| `block_direct`     | boolean | false    |              |             |
| `force_websockets` | boolean | false    |              |             |
| `path`             | string  | false    |              |             |
| `url`              | string  | false    |              |             |

## codersdk.DERPRegion

```json
{
  "latency_ms": 0,
  "preferred": true
}
```

### Properties

| Name         | Type    | Required | Restrictions | Description |
|--------------|---------|----------|--------------|-------------|
| `latency_ms` | number  | false    |              |             |
| `preferred`  | boolean | false    |              |             |

## codersdk.DERPServerConfig

```json
{
  "enable": true,
  "region_code": "string",
  "region_id": 0,
  "region_name": "string",
  "relay_url": {
    "forceQuery": true,
    "fragment": "string",
    "host": "string",
    "omitHost": true,
    "opaque": "string",
    "path": "string",
    "rawFragment": "string",
    "rawPath": "string",
    "rawQuery": "string",
    "scheme": "string",
    "user": {}
  },
  "stun_addresses": [
    "string"
  ]
}
```

### Properties

| Name             | Type                       | Required | Restrictions | Description |
|------------------|----------------------------|----------|--------------|-------------|
| `enable`         | boolean                    | false    |              |             |
| `region_code`    | string                     | false    |              |             |
| `region_id`      | integer                    | false    |              |             |
| `region_name`    | string                     | false    |              |             |
| `relay_url`      | [serpent.URL](#serpenturl) | false    |              |             |
| `stun_addresses` | array of string            | false    |              |             |

## codersdk.DangerousConfig

```json
{
  "allow_all_cors": true,
  "allow_path_app_sharing": true,
  "allow_path_app_site_owner_access": true
}
```

### Properties

| Name                               | Type    | Required | Restrictions | Description |
|------------------------------------|---------|----------|--------------|-------------|
| `allow_all_cors`                   | boolean | false    |              |             |
| `allow_path_app_sharing`           | boolean | false    |              |             |
| `allow_path_app_site_owner_access` | boolean | false    |              |             |

## codersdk.DeleteWebpushSubscription

```json
{
  "endpoint": "string"
}
```

### Properties

| Name       | Type   | Required | Restrictions | Description |
|------------|--------|----------|--------------|-------------|
| `endpoint` | string | false    |              |             |

## codersdk.DeleteWorkspaceAgentPortShareRequest

```json
{
  "agent_name": "string",
  "port": 0
}
```

### Properties

| Name         | Type    | Required | Restrictions | Description |
|--------------|---------|----------|--------------|-------------|
| `agent_name` | string  | false    |              |             |
| `port`       | integer | false    |              |             |

## codersdk.DeploymentConfig

```json
{
  "config": {
    "access_url": {
      "forceQuery": true,
      "fragment": "string",
      "host": "string",
      "omitHost": true,
      "opaque": "string",
      "path": "string",
      "rawFragment": "string",
      "rawPath": "string",
      "rawQuery": "string",
      "scheme": "string",
      "user": {}
    },
    "additional_csp_policy": [
      "string"
    ],
    "address": {
      "host": "string",
      "port": "string"
    },
    "agent_fallback_troubleshooting_url": {
      "forceQuery": true,
      "fragment": "string",
      "host": "string",
      "omitHost": true,
      "opaque": "string",
      "path": "string",
      "rawFragment": "string",
      "rawPath": "string",
      "rawQuery": "string",
      "scheme": "string",
      "user": {}
    },
    "agent_stat_refresh_interval": 0,
    "allow_workspace_renames": true,
    "autobuild_poll_interval": 0,
    "browser_only": true,
    "cache_directory": "string",
    "cli_upgrade_message": "string",
    "config": "string",
    "config_ssh": {
      "deploymentName": "string",
      "sshconfigOptions": [
        "string"
      ]
    },
    "dangerous": {
      "allow_all_cors": true,
      "allow_path_app_sharing": true,
      "allow_path_app_site_owner_access": true
    },
    "derp": {
      "config": {
        "block_direct": true,
        "force_websockets": true,
        "path": "string",
        "url": "string"
      },
      "server": {
        "enable": true,
        "region_code": "string",
        "region_id": 0,
        "region_name": "string",
        "relay_url": {
          "forceQuery": true,
          "fragment": "string",
          "host": "string",
          "omitHost": true,
          "opaque": "string",
          "path": "string",
          "rawFragment": "string",
          "rawPath": "string",
          "rawQuery": "string",
          "scheme": "string",
          "user": {}
        },
        "stun_addresses": [
          "string"
        ]
      }
    },
    "disable_owner_workspace_exec": true,
    "disable_password_auth": true,
    "disable_path_apps": true,
    "docs_url": {
      "forceQuery": true,
      "fragment": "string",
      "host": "string",
      "omitHost": true,
      "opaque": "string",
      "path": "string",
      "rawFragment": "string",
      "rawPath": "string",
      "rawQuery": "string",
      "scheme": "string",
      "user": {}
    },
    "enable_terraform_debug_mode": true,
    "ephemeral_deployment": true,
    "experiments": [
      "string"
    ],
    "external_auth": {
      "value": [
        {
          "app_install_url": "string",
          "app_installations_url": "string",
          "auth_url": "string",
          "client_id": "string",
          "device_code_url": "string",
          "device_flow": true,
          "display_icon": "string",
          "display_name": "string",
          "id": "string",
          "no_refresh": true,
          "regex": "string",
          "scopes": [
            "string"
          ],
          "token_url": "string",
          "type": "string",
          "validate_url": "string"
        }
      ]
    },
    "external_token_encryption_keys": [
      "string"
    ],
    "healthcheck": {
      "refresh": 0,
      "threshold_database": 0
    },
    "http_address": "string",
    "in_memory_database": true,
    "job_hang_detector_interval": 0,
    "logging": {
      "human": "string",
      "json": "string",
      "log_filter": [
        "string"
      ],
      "stackdriver": "string"
    },
    "metrics_cache_refresh_interval": 0,
    "notifications": {
      "dispatch_timeout": 0,
      "email": {
        "auth": {
          "identity": "string",
          "password": "string",
          "password_file": "string",
          "username": "string"
        },
        "force_tls": true,
        "from": "string",
        "hello": "string",
        "smarthost": "string",
        "tls": {
          "ca_file": "string",
          "cert_file": "string",
          "insecure_skip_verify": true,
          "key_file": "string",
          "server_name": "string",
          "start_tls": true
        }
      },
      "fetch_interval": 0,
      "inbox": {
        "enabled": true
      },
      "lease_count": 0,
      "lease_period": 0,
      "max_send_attempts": 0,
      "method": "string",
      "retry_interval": 0,
      "sync_buffer_size": 0,
      "sync_interval": 0,
      "webhook": {
        "endpoint": {
          "forceQuery": true,
          "fragment": "string",
          "host": "string",
          "omitHost": true,
          "opaque": "string",
          "path": "string",
          "rawFragment": "string",
          "rawPath": "string",
          "rawQuery": "string",
          "scheme": "string",
          "user": {}
        }
      }
    },
    "oauth2": {
      "github": {
        "allow_everyone": true,
        "allow_signups": true,
        "allowed_orgs": [
          "string"
        ],
        "allowed_teams": [
          "string"
        ],
        "client_id": "string",
        "client_secret": "string",
        "default_provider_enable": true,
        "device_flow": true,
        "enterprise_base_url": "string"
      }
    },
    "oidc": {
      "allow_signups": true,
      "auth_url_params": {},
      "client_cert_file": "string",
      "client_id": "string",
      "client_key_file": "string",
      "client_secret": "string",
      "email_domain": [
        "string"
      ],
      "email_field": "string",
      "group_allow_list": [
        "string"
      ],
      "group_auto_create": true,
      "group_mapping": {},
      "group_regex_filter": {},
      "groups_field": "string",
      "icon_url": {
        "forceQuery": true,
        "fragment": "string",
        "host": "string",
        "omitHost": true,
        "opaque": "string",
        "path": "string",
        "rawFragment": "string",
        "rawPath": "string",
        "rawQuery": "string",
        "scheme": "string",
        "user": {}
      },
      "ignore_email_verified": true,
      "ignore_user_info": true,
      "issuer_url": "string",
      "name_field": "string",
      "organization_assign_default": true,
      "organization_field": "string",
      "organization_mapping": {},
      "scopes": [
        "string"
      ],
      "sign_in_text": "string",
      "signups_disabled_text": "string",
      "skip_issuer_checks": true,
      "source_user_info_from_access_token": true,
      "user_role_field": "string",
      "user_role_mapping": {},
      "user_roles_default": [
        "string"
      ],
      "username_field": "string"
    },
    "pg_auth": "string",
    "pg_connection_url": "string",
    "pprof": {
      "address": {
        "host": "string",
        "port": "string"
      },
      "enable": true
    },
    "prometheus": {
      "address": {
        "host": "string",
        "port": "string"
      },
      "aggregate_agent_stats_by": [
        "string"
      ],
      "collect_agent_stats": true,
      "collect_db_metrics": true,
      "enable": true
    },
    "provisioner": {
      "daemon_poll_interval": 0,
      "daemon_poll_jitter": 0,
      "daemon_psk": "string",
      "daemon_types": [
        "string"
      ],
      "daemons": 0,
      "force_cancel_interval": 0
    },
    "proxy_health_status_interval": 0,
    "proxy_trusted_headers": [
      "string"
    ],
    "proxy_trusted_origins": [
      "string"
    ],
    "rate_limit": {
      "api": 0,
      "disable_all": true
    },
    "redirect_to_access_url": true,
    "scim_api_key": "string",
    "secure_auth_cookie": true,
    "session_lifetime": {
      "default_duration": 0,
      "default_token_lifetime": 0,
      "disable_expiry_refresh": true,
      "max_token_lifetime": 0
    },
    "ssh_keygen_algorithm": "string",
    "strict_transport_security": 0,
    "strict_transport_security_options": [
      "string"
    ],
    "support": {
      "links": {
        "value": [
          {
            "icon": "bug",
            "name": "string",
            "target": "string"
          }
        ]
      }
    },
    "swagger": {
      "enable": true
    },
    "telemetry": {
      "enable": true,
      "trace": true,
      "url": {
        "forceQuery": true,
        "fragment": "string",
        "host": "string",
        "omitHost": true,
        "opaque": "string",
        "path": "string",
        "rawFragment": "string",
        "rawPath": "string",
        "rawQuery": "string",
        "scheme": "string",
        "user": {}
      }
    },
    "terms_of_service_url": "string",
    "tls": {
      "address": {
        "host": "string",
        "port": "string"
      },
      "allow_insecure_ciphers": true,
      "cert_file": [
        "string"
      ],
      "client_auth": "string",
      "client_ca_file": "string",
      "client_cert_file": "string",
      "client_key_file": "string",
      "enable": true,
      "key_file": [
        "string"
      ],
      "min_version": "string",
      "redirect_http": true,
      "supported_ciphers": [
        "string"
      ]
    },
    "trace": {
      "capture_logs": true,
      "data_dog": true,
      "enable": true,
      "honeycomb_api_key": "string"
    },
    "update_check": true,
    "user_quiet_hours_schedule": {
      "allow_user_custom": true,
      "default_schedule": "string"
    },
    "verbose": true,
    "web_terminal_renderer": "string",
    "wgtunnel_host": "string",
    "wildcard_access_url": "string",
    "write_config": true
  },
  "options": [
    {
      "annotations": {
        "property1": "string",
        "property2": "string"
      },
      "default": "string",
      "description": "string",
      "env": "string",
      "flag": "string",
      "flag_shorthand": "string",
      "group": {
        "description": "string",
        "name": "string",
        "parent": {
          "description": "string",
          "name": "string",
          "parent": {},
          "yaml": "string"
        },
        "yaml": "string"
      },
      "hidden": true,
      "name": "string",
      "required": true,
      "use_instead": [
        {}
      ],
      "value": null,
      "value_source": "",
      "yaml": "string"
    }
  ]
}
```

### Properties

| Name      | Type                                                   | Required | Restrictions | Description |
|-----------|--------------------------------------------------------|----------|--------------|-------------|
| `config`  | [codersdk.DeploymentValues](#codersdkdeploymentvalues) | false    |              |             |
| `options` | array of [serpent.Option](#serpentoption)              | false    |              |             |

## codersdk.DeploymentStats

```json
{
  "aggregated_from": "2019-08-24T14:15:22Z",
  "collected_at": "2019-08-24T14:15:22Z",
  "next_update_at": "2019-08-24T14:15:22Z",
  "session_count": {
    "jetbrains": 0,
    "reconnecting_pty": 0,
    "ssh": 0,
    "vscode": 0
  },
  "workspaces": {
    "building": 0,
    "connection_latency_ms": {
      "p50": 0,
      "p95": 0
    },
    "failed": 0,
    "pending": 0,
    "running": 0,
    "rx_bytes": 0,
    "stopped": 0,
    "tx_bytes": 0
  }
}
```

### Properties

| Name              | Type                                                                         | Required | Restrictions | Description                                                                                                                 |
|-------------------|------------------------------------------------------------------------------|----------|--------------|-----------------------------------------------------------------------------------------------------------------------------|
| `aggregated_from` | string                                                                       | false    |              | Aggregated from is the time in which stats are aggregated from. This might be back in time a specific duration or interval. |
| `collected_at`    | string                                                                       | false    |              | Collected at is the time in which stats are collected at.                                                                   |
| `next_update_at`  | string                                                                       | false    |              | Next update at is the time when the next batch of stats will be updated.                                                    |
| `session_count`   | [codersdk.SessionCountDeploymentStats](#codersdksessioncountdeploymentstats) | false    |              |                                                                                                                             |
| `workspaces`      | [codersdk.WorkspaceDeploymentStats](#codersdkworkspacedeploymentstats)       | false    |              |                                                                                                                             |

## codersdk.DeploymentValues

```json
{
  "access_url": {
    "forceQuery": true,
    "fragment": "string",
    "host": "string",
    "omitHost": true,
    "opaque": "string",
    "path": "string",
    "rawFragment": "string",
    "rawPath": "string",
    "rawQuery": "string",
    "scheme": "string",
    "user": {}
  },
  "additional_csp_policy": [
    "string"
  ],
  "address": {
    "host": "string",
    "port": "string"
  },
  "agent_fallback_troubleshooting_url": {
    "forceQuery": true,
    "fragment": "string",
    "host": "string",
    "omitHost": true,
    "opaque": "string",
    "path": "string",
    "rawFragment": "string",
    "rawPath": "string",
    "rawQuery": "string",
    "scheme": "string",
    "user": {}
  },
  "agent_stat_refresh_interval": 0,
  "allow_workspace_renames": true,
  "autobuild_poll_interval": 0,
  "browser_only": true,
  "cache_directory": "string",
  "cli_upgrade_message": "string",
  "config": "string",
  "config_ssh": {
    "deploymentName": "string",
    "sshconfigOptions": [
      "string"
    ]
  },
  "dangerous": {
    "allow_all_cors": true,
    "allow_path_app_sharing": true,
    "allow_path_app_site_owner_access": true
  },
  "derp": {
    "config": {
      "block_direct": true,
      "force_websockets": true,
      "path": "string",
      "url": "string"
    },
    "server": {
      "enable": true,
      "region_code": "string",
      "region_id": 0,
      "region_name": "string",
      "relay_url": {
        "forceQuery": true,
        "fragment": "string",
        "host": "string",
        "omitHost": true,
        "opaque": "string",
        "path": "string",
        "rawFragment": "string",
        "rawPath": "string",
        "rawQuery": "string",
        "scheme": "string",
        "user": {}
      },
      "stun_addresses": [
        "string"
      ]
    }
  },
  "disable_owner_workspace_exec": true,
  "disable_password_auth": true,
  "disable_path_apps": true,
  "docs_url": {
    "forceQuery": true,
    "fragment": "string",
    "host": "string",
    "omitHost": true,
    "opaque": "string",
    "path": "string",
    "rawFragment": "string",
    "rawPath": "string",
    "rawQuery": "string",
    "scheme": "string",
    "user": {}
  },
  "enable_terraform_debug_mode": true,
  "ephemeral_deployment": true,
  "experiments": [
    "string"
  ],
  "external_auth": {
    "value": [
      {
        "app_install_url": "string",
        "app_installations_url": "string",
        "auth_url": "string",
        "client_id": "string",
        "device_code_url": "string",
        "device_flow": true,
        "display_icon": "string",
        "display_name": "string",
        "id": "string",
        "no_refresh": true,
        "regex": "string",
        "scopes": [
          "string"
        ],
        "token_url": "string",
        "type": "string",
        "validate_url": "string"
      }
    ]
  },
  "external_token_encryption_keys": [
    "string"
  ],
  "healthcheck": {
    "refresh": 0,
    "threshold_database": 0
  },
  "http_address": "string",
  "in_memory_database": true,
  "job_hang_detector_interval": 0,
  "logging": {
    "human": "string",
    "json": "string",
    "log_filter": [
      "string"
    ],
    "stackdriver": "string"
  },
  "metrics_cache_refresh_interval": 0,
  "notifications": {
    "dispatch_timeout": 0,
    "email": {
      "auth": {
        "identity": "string",
        "password": "string",
        "password_file": "string",
        "username": "string"
      },
      "force_tls": true,
      "from": "string",
      "hello": "string",
      "smarthost": "string",
      "tls": {
        "ca_file": "string",
        "cert_file": "string",
        "insecure_skip_verify": true,
        "key_file": "string",
        "server_name": "string",
        "start_tls": true
      }
    },
    "fetch_interval": 0,
    "inbox": {
      "enabled": true
    },
    "lease_count": 0,
    "lease_period": 0,
    "max_send_attempts": 0,
    "method": "string",
    "retry_interval": 0,
    "sync_buffer_size": 0,
    "sync_interval": 0,
    "webhook": {
      "endpoint": {
        "forceQuery": true,
        "fragment": "string",
        "host": "string",
        "omitHost": true,
        "opaque": "string",
        "path": "string",
        "rawFragment": "string",
        "rawPath": "string",
        "rawQuery": "string",
        "scheme": "string",
        "user": {}
      }
    }
  },
  "oauth2": {
    "github": {
      "allow_everyone": true,
      "allow_signups": true,
      "allowed_orgs": [
        "string"
      ],
      "allowed_teams": [
        "string"
      ],
      "client_id": "string",
      "client_secret": "string",
      "default_provider_enable": true,
      "device_flow": true,
      "enterprise_base_url": "string"
    }
  },
  "oidc": {
    "allow_signups": true,
    "auth_url_params": {},
    "client_cert_file": "string",
    "client_id": "string",
    "client_key_file": "string",
    "client_secret": "string",
    "email_domain": [
      "string"
    ],
    "email_field": "string",
    "group_allow_list": [
      "string"
    ],
    "group_auto_create": true,
    "group_mapping": {},
    "group_regex_filter": {},
    "groups_field": "string",
    "icon_url": {
      "forceQuery": true,
      "fragment": "string",
      "host": "string",
      "omitHost": true,
      "opaque": "string",
      "path": "string",
      "rawFragment": "string",
      "rawPath": "string",
      "rawQuery": "string",
      "scheme": "string",
      "user": {}
    },
    "ignore_email_verified": true,
    "ignore_user_info": true,
    "issuer_url": "string",
    "name_field": "string",
    "organization_assign_default": true,
    "organization_field": "string",
    "organization_mapping": {},
    "scopes": [
      "string"
    ],
    "sign_in_text": "string",
    "signups_disabled_text": "string",
    "skip_issuer_checks": true,
    "source_user_info_from_access_token": true,
    "user_role_field": "string",
    "user_role_mapping": {},
    "user_roles_default": [
      "string"
    ],
    "username_field": "string"
  },
  "pg_auth": "string",
  "pg_connection_url": "string",
  "pprof": {
    "address": {
      "host": "string",
      "port": "string"
    },
    "enable": true
  },
  "prometheus": {
    "address": {
      "host": "string",
      "port": "string"
    },
    "aggregate_agent_stats_by": [
      "string"
    ],
    "collect_agent_stats": true,
    "collect_db_metrics": true,
    "enable": true
  },
  "provisioner": {
    "daemon_poll_interval": 0,
    "daemon_poll_jitter": 0,
    "daemon_psk": "string",
    "daemon_types": [
      "string"
    ],
    "daemons": 0,
    "force_cancel_interval": 0
  },
  "proxy_health_status_interval": 0,
  "proxy_trusted_headers": [
    "string"
  ],
  "proxy_trusted_origins": [
    "string"
  ],
  "rate_limit": {
    "api": 0,
    "disable_all": true
  },
  "redirect_to_access_url": true,
  "scim_api_key": "string",
  "secure_auth_cookie": true,
  "session_lifetime": {
    "default_duration": 0,
    "default_token_lifetime": 0,
    "disable_expiry_refresh": true,
    "max_token_lifetime": 0
  },
  "ssh_keygen_algorithm": "string",
  "strict_transport_security": 0,
  "strict_transport_security_options": [
    "string"
  ],
  "support": {
    "links": {
      "value": [
        {
          "icon": "bug",
          "name": "string",
          "target": "string"
        }
      ]
    }
  },
  "swagger": {
    "enable": true
  },
  "telemetry": {
    "enable": true,
    "trace": true,
    "url": {
      "forceQuery": true,
      "fragment": "string",
      "host": "string",
      "omitHost": true,
      "opaque": "string",
      "path": "string",
      "rawFragment": "string",
      "rawPath": "string",
      "rawQuery": "string",
      "scheme": "string",
      "user": {}
    }
  },
  "terms_of_service_url": "string",
  "tls": {
    "address": {
      "host": "string",
      "port": "string"
    },
    "allow_insecure_ciphers": true,
    "cert_file": [
      "string"
    ],
    "client_auth": "string",
    "client_ca_file": "string",
    "client_cert_file": "string",
    "client_key_file": "string",
    "enable": true,
    "key_file": [
      "string"
    ],
    "min_version": "string",
    "redirect_http": true,
    "supported_ciphers": [
      "string"
    ]
  },
  "trace": {
    "capture_logs": true,
    "data_dog": true,
    "enable": true,
    "honeycomb_api_key": "string"
  },
  "update_check": true,
  "user_quiet_hours_schedule": {
    "allow_user_custom": true,
    "default_schedule": "string"
  },
  "verbose": true,
  "web_terminal_renderer": "string",
  "wgtunnel_host": "string",
  "wildcard_access_url": "string",
  "write_config": true
}
```

### Properties

| Name                                 | Type                                                                                                 | Required | Restrictions | Description                                                        |
|--------------------------------------|------------------------------------------------------------------------------------------------------|----------|--------------|--------------------------------------------------------------------|
| `access_url`                         | [serpent.URL](#serpenturl)                                                                           | false    |              |                                                                    |
| `additional_csp_policy`              | array of string                                                                                      | false    |              |                                                                    |
| `address`                            | [serpent.HostPort](#serpenthostport)                                                                 | false    |              | Deprecated: Use HTTPAddress or TLS.Address instead.                |
| `agent_fallback_troubleshooting_url` | [serpent.URL](#serpenturl)                                                                           | false    |              |                                                                    |
| `agent_stat_refresh_interval`        | integer                                                                                              | false    |              |                                                                    |
| `allow_workspace_renames`            | boolean                                                                                              | false    |              |                                                                    |
| `autobuild_poll_interval`            | integer                                                                                              | false    |              |                                                                    |
| `browser_only`                       | boolean                                                                                              | false    |              |                                                                    |
| `cache_directory`                    | string                                                                                               | false    |              |                                                                    |
| `cli_upgrade_message`                | string                                                                                               | false    |              |                                                                    |
| `config`                             | string                                                                                               | false    |              |                                                                    |
| `config_ssh`                         | [codersdk.SSHConfig](#codersdksshconfig)                                                             | false    |              |                                                                    |
| `dangerous`                          | [codersdk.DangerousConfig](#codersdkdangerousconfig)                                                 | false    |              |                                                                    |
| `derp`                               | [codersdk.DERP](#codersdkderp)                                                                       | false    |              |                                                                    |
| `disable_owner_workspace_exec`       | boolean                                                                                              | false    |              |                                                                    |
| `disable_password_auth`              | boolean                                                                                              | false    |              |                                                                    |
| `disable_path_apps`                  | boolean                                                                                              | false    |              |                                                                    |
| `docs_url`                           | [serpent.URL](#serpenturl)                                                                           | false    |              |                                                                    |
| `enable_terraform_debug_mode`        | boolean                                                                                              | false    |              |                                                                    |
| `ephemeral_deployment`               | boolean                                                                                              | false    |              |                                                                    |
| `experiments`                        | array of string                                                                                      | false    |              |                                                                    |
| `external_auth`                      | [serpent.Struct-array_codersdk_ExternalAuthConfig](#serpentstruct-array_codersdk_externalauthconfig) | false    |              |                                                                    |
| `external_token_encryption_keys`     | array of string                                                                                      | false    |              |                                                                    |
| `healthcheck`                        | [codersdk.HealthcheckConfig](#codersdkhealthcheckconfig)                                             | false    |              |                                                                    |
| `http_address`                       | string                                                                                               | false    |              | Http address is a string because it may be set to zero to disable. |
| `in_memory_database`                 | boolean                                                                                              | false    |              |                                                                    |
| `job_hang_detector_interval`         | integer                                                                                              | false    |              |                                                                    |
| `logging`                            | [codersdk.LoggingConfig](#codersdkloggingconfig)                                                     | false    |              |                                                                    |
| `metrics_cache_refresh_interval`     | integer                                                                                              | false    |              |                                                                    |
| `notifications`                      | [codersdk.NotificationsConfig](#codersdknotificationsconfig)                                         | false    |              |                                                                    |
| `oauth2`                             | [codersdk.OAuth2Config](#codersdkoauth2config)                                                       | false    |              |                                                                    |
| `oidc`                               | [codersdk.OIDCConfig](#codersdkoidcconfig)                                                           | false    |              |                                                                    |
| `pg_auth`                            | string                                                                                               | false    |              |                                                                    |
| `pg_connection_url`                  | string                                                                                               | false    |              |                                                                    |
| `pprof`                              | [codersdk.PprofConfig](#codersdkpprofconfig)                                                         | false    |              |                                                                    |
| `prometheus`                         | [codersdk.PrometheusConfig](#codersdkprometheusconfig)                                               | false    |              |                                                                    |
| `provisioner`                        | [codersdk.ProvisionerConfig](#codersdkprovisionerconfig)                                             | false    |              |                                                                    |
| `proxy_health_status_interval`       | integer                                                                                              | false    |              |                                                                    |
| `proxy_trusted_headers`              | array of string                                                                                      | false    |              |                                                                    |
| `proxy_trusted_origins`              | array of string                                                                                      | false    |              |                                                                    |
| `rate_limit`                         | [codersdk.RateLimitConfig](#codersdkratelimitconfig)                                                 | false    |              |                                                                    |
| `redirect_to_access_url`             | boolean                                                                                              | false    |              |                                                                    |
| `scim_api_key`                       | string                                                                                               | false    |              |                                                                    |
| `secure_auth_cookie`                 | boolean                                                                                              | false    |              |                                                                    |
| `session_lifetime`                   | [codersdk.SessionLifetime](#codersdksessionlifetime)                                                 | false    |              |                                                                    |
| `ssh_keygen_algorithm`               | string                                                                                               | false    |              |                                                                    |
| `strict_transport_security`          | integer                                                                                              | false    |              |                                                                    |
| `strict_transport_security_options`  | array of string                                                                                      | false    |              |                                                                    |
| `support`                            | [codersdk.SupportConfig](#codersdksupportconfig)                                                     | false    |              |                                                                    |
| `swagger`                            | [codersdk.SwaggerConfig](#codersdkswaggerconfig)                                                     | false    |              |                                                                    |
| `telemetry`                          | [codersdk.TelemetryConfig](#codersdktelemetryconfig)                                                 | false    |              |                                                                    |
| `terms_of_service_url`               | string                                                                                               | false    |              |                                                                    |
| `tls`                                | [codersdk.TLSConfig](#codersdktlsconfig)                                                             | false    |              |                                                                    |
| `trace`                              | [codersdk.TraceConfig](#codersdktraceconfig)                                                         | false    |              |                                                                    |
| `update_check`                       | boolean                                                                                              | false    |              |                                                                    |
| `user_quiet_hours_schedule`          | [codersdk.UserQuietHoursScheduleConfig](#codersdkuserquiethoursscheduleconfig)                       | false    |              |                                                                    |
| `verbose`                            | boolean                                                                                              | false    |              |                                                                    |
| `web_terminal_renderer`              | string                                                                                               | false    |              |                                                                    |
| `wgtunnel_host`                      | string                                                                                               | false    |              |                                                                    |
| `wildcard_access_url`                | string                                                                                               | false    |              |                                                                    |
| `write_config`                       | boolean                                                                                              | false    |              |                                                                    |

## codersdk.DisplayApp

```json
"vscode"
```

### Properties

#### Enumerated Values

| Value                    |
|--------------------------|
| `vscode`                 |
| `vscode_insiders`        |
| `web_terminal`           |
| `port_forwarding_helper` |
| `ssh_helper`             |

## codersdk.Entitlement

```json
"entitled"
```

### Properties

#### Enumerated Values

| Value          |
|----------------|
| `entitled`     |
| `grace_period` |
| `not_entitled` |

## codersdk.Entitlements

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

### Properties

| Name                | Type                                 | Required | Restrictions | Description |
|---------------------|--------------------------------------|----------|--------------|-------------|
| `errors`            | array of string                      | false    |              |             |
| `features`          | object                               | false    |              |             |
| » `[any property]`  | [codersdk.Feature](#codersdkfeature) | false    |              |             |
| `has_license`       | boolean                              | false    |              |             |
| `refreshed_at`      | string                               | false    |              |             |
| `require_telemetry` | boolean                              | false    |              |             |
| `trial`             | boolean                              | false    |              |             |
| `warnings`          | array of string                      | false    |              |             |

## codersdk.Experiment

```json
"example"
```

### Properties

#### Enumerated Values

| Value                  |
|------------------------|
| `example`              |
| `auto-fill-parameters` |
| `notifications`        |
| `workspace-usage`      |
| `web-push`             |
| `dynamic-parameters`   |

## codersdk.ExternalAuth

```json
{
  "app_install_url": "string",
  "app_installable": true,
  "authenticated": true,
  "device": true,
  "display_name": "string",
  "installations": [
    {
      "account": {
        "avatar_url": "string",
        "id": 0,
        "login": "string",
        "name": "string",
        "profile_url": "string"
      },
      "configure_url": "string",
      "id": 0
    }
  ],
  "user": {
    "avatar_url": "string",
    "id": 0,
    "login": "string",
    "name": "string",
    "profile_url": "string"
  }
}
```

### Properties

| Name              | Type                                                                                  | Required | Restrictions | Description                                                             |
|-------------------|---------------------------------------------------------------------------------------|----------|--------------|-------------------------------------------------------------------------|
| `app_install_url` | string                                                                                | false    |              | App install URL is the URL to install the app.                          |
| `app_installable` | boolean                                                                               | false    |              | App installable is true if the request for app installs was successful. |
| `authenticated`   | boolean                                                                               | false    |              |                                                                         |
| `device`          | boolean                                                                               | false    |              |                                                                         |
| `display_name`    | string                                                                                | false    |              |                                                                         |
| `installations`   | array of [codersdk.ExternalAuthAppInstallation](#codersdkexternalauthappinstallation) | false    |              | Installations are the installations that the user has access to.        |
| `user`            | [codersdk.ExternalAuthUser](#codersdkexternalauthuser)                                | false    |              | User is the user that authenticated with the provider.                  |

## codersdk.ExternalAuthAppInstallation

```json
{
  "account": {
    "avatar_url": "string",
    "id": 0,
    "login": "string",
    "name": "string",
    "profile_url": "string"
  },
  "configure_url": "string",
  "id": 0
}
```

### Properties

| Name            | Type                                                   | Required | Restrictions | Description |
|-----------------|--------------------------------------------------------|----------|--------------|-------------|
| `account`       | [codersdk.ExternalAuthUser](#codersdkexternalauthuser) | false    |              |             |
| `configure_url` | string                                                 | false    |              |             |
| `id`            | integer                                                | false    |              |             |

## codersdk.ExternalAuthConfig

```json
{
  "app_install_url": "string",
  "app_installations_url": "string",
  "auth_url": "string",
  "client_id": "string",
  "device_code_url": "string",
  "device_flow": true,
  "display_icon": "string",
  "display_name": "string",
  "id": "string",
  "no_refresh": true,
  "regex": "string",
  "scopes": [
    "string"
  ],
  "token_url": "string",
  "type": "string",
  "validate_url": "string"
}
```

### Properties

| Name                    | Type    | Required | Restrictions | Description                                                                             |
|-------------------------|---------|----------|--------------|-----------------------------------------------------------------------------------------|
| `app_install_url`       | string  | false    |              |                                                                                         |
| `app_installations_url` | string  | false    |              |                                                                                         |
| `auth_url`              | string  | false    |              |                                                                                         |
| `client_id`             | string  | false    |              |                                                                                         |
| `device_code_url`       | string  | false    |              |                                                                                         |
| `device_flow`           | boolean | false    |              |                                                                                         |
| `display_icon`          | string  | false    |              | Display icon is a URL to an icon to display in the UI.                                  |
| `display_name`          | string  | false    |              | Display name is shown in the UI to identify the auth config.                            |
| `id`                    | string  | false    |              | ID is a unique identifier for the auth config. It defaults to `type` when not provided. |
| `no_refresh`            | boolean | false    |              |                                                                                         |
|`regex`|string|false||Regex allows API requesters to match an auth config by a string (e.g. coder.com) instead of by it's type.
Git clone makes use of this by parsing the URL from: 'Username for "https://github.com":' And sending it to the Coder server to match against the Regex.|
|`scopes`|array of string|false|||
|`token_url`|string|false|||
|`type`|string|false||Type is the type of external auth config.|
|`validate_url`|string|false|||

## codersdk.ExternalAuthDevice

```json
{
  "device_code": "string",
  "expires_in": 0,
  "interval": 0,
  "user_code": "string",
  "verification_uri": "string"
}
```

### Properties

| Name               | Type    | Required | Restrictions | Description |
|--------------------|---------|----------|--------------|-------------|
| `device_code`      | string  | false    |              |             |
| `expires_in`       | integer | false    |              |             |
| `interval`         | integer | false    |              |             |
| `user_code`        | string  | false    |              |             |
| `verification_uri` | string  | false    |              |             |

## codersdk.ExternalAuthLink

```json
{
  "authenticated": true,
  "created_at": "2019-08-24T14:15:22Z",
  "expires": "2019-08-24T14:15:22Z",
  "has_refresh_token": true,
  "provider_id": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "validate_error": "string"
}
```

### Properties

| Name                | Type    | Required | Restrictions | Description |
|---------------------|---------|----------|--------------|-------------|
| `authenticated`     | boolean | false    |              |             |
| `created_at`        | string  | false    |              |             |
| `expires`           | string  | false    |              |             |
| `has_refresh_token` | boolean | false    |              |             |
| `provider_id`       | string  | false    |              |             |
| `updated_at`        | string  | false    |              |             |
| `validate_error`    | string  | false    |              |             |

## codersdk.ExternalAuthUser

```json
{
  "avatar_url": "string",
  "id": 0,
  "login": "string",
  "name": "string",
  "profile_url": "string"
}
```

### Properties

| Name          | Type    | Required | Restrictions | Description |
|---------------|---------|----------|--------------|-------------|
| `avatar_url`  | string  | false    |              |             |
| `id`          | integer | false    |              |             |
| `login`       | string  | false    |              |             |
| `name`        | string  | false    |              |             |
| `profile_url` | string  | false    |              |             |

## codersdk.Feature

```json
{
  "actual": 0,
  "enabled": true,
  "entitlement": "entitled",
  "limit": 0
}
```

### Properties

| Name          | Type                                         | Required | Restrictions | Description |
|---------------|----------------------------------------------|----------|--------------|-------------|
| `actual`      | integer                                      | false    |              |             |
| `enabled`     | boolean                                      | false    |              |             |
| `entitlement` | [codersdk.Entitlement](#codersdkentitlement) | false    |              |             |
| `limit`       | integer                                      | false    |              |             |

## codersdk.GenerateAPIKeyResponse

```json
{
  "key": "string"
}
```

### Properties

| Name  | Type   | Required | Restrictions | Description |
|-------|--------|----------|--------------|-------------|
| `key` | string | false    |              |             |

## codersdk.GetInboxNotificationResponse

```json
{
  "notification": {
    "actions": [
      {
        "label": "string",
        "url": "string"
      }
    ],
    "content": "string",
    "created_at": "2019-08-24T14:15:22Z",
    "icon": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "read_at": "string",
    "targets": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
    "title": "string",
    "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
  },
  "unread_count": 0
}
```

### Properties

| Name           | Type                                                     | Required | Restrictions | Description |
|----------------|----------------------------------------------------------|----------|--------------|-------------|
| `notification` | [codersdk.InboxNotification](#codersdkinboxnotification) | false    |              |             |
| `unread_count` | integer                                                  | false    |              |             |

## codersdk.GetUserStatusCountsResponse

```json
{
  "status_counts": {
    "property1": [
      {
        "count": 10,
        "date": "2019-08-24T14:15:22Z"
      }
    ],
    "property2": [
      {
        "count": 10,
        "date": "2019-08-24T14:15:22Z"
      }
    ]
  }
}
```

### Properties

| Name               | Type                                                                      | Required | Restrictions | Description |
|--------------------|---------------------------------------------------------------------------|----------|--------------|-------------|
| `status_counts`    | object                                                                    | false    |              |             |
| » `[any property]` | array of [codersdk.UserStatusChangeCount](#codersdkuserstatuschangecount) | false    |              |             |

## codersdk.GetUsersResponse

```json
{
  "count": 0,
  "users": [
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
  ]
}
```

### Properties

| Name    | Type                                    | Required | Restrictions | Description |
|---------|-----------------------------------------|----------|--------------|-------------|
| `count` | integer                                 | false    |              |             |
| `users` | array of [codersdk.User](#codersdkuser) | false    |              |             |

## codersdk.GitSSHKey

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "public_key": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
}
```

### Properties

| Name         | Type   | Required | Restrictions | Description                                                                                                                                                                                       |
|--------------|--------|----------|--------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `created_at` | string | false    |              |                                                                                                                                                                                                   |
| `public_key` | string | false    |              | Public key is the SSH public key in OpenSSH format. Example: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAID3OmYJvT7q1cF1azbybYy0OZ9yrXfA+M6Lr4vzX5zlp\n" Note: The key includes a trailing newline (\n). |
| `updated_at` | string | false    |              |                                                                                                                                                                                                   |
| `user_id`    | string | false    |              |                                                                                                                                                                                                   |

## codersdk.GithubAuthMethod

```json
{
  "default_provider_configured": true,
  "enabled": true
}
```

### Properties

| Name                          | Type    | Required | Restrictions | Description |
|-------------------------------|---------|----------|--------------|-------------|
| `default_provider_configured` | boolean | false    |              |             |
| `enabled`                     | boolean | false    |              |             |

## codersdk.Group

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

### Properties

| Name                        | Type                                                  | Required | Restrictions | Description                                                                                                                                                           |
|-----------------------------|-------------------------------------------------------|----------|--------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `avatar_url`                | string                                                | false    |              |                                                                                                                                                                       |
| `display_name`              | string                                                | false    |              |                                                                                                                                                                       |
| `id`                        | string                                                | false    |              |                                                                                                                                                                       |
| `members`                   | array of [codersdk.ReducedUser](#codersdkreduceduser) | false    |              |                                                                                                                                                                       |
| `name`                      | string                                                | false    |              |                                                                                                                                                                       |
| `organization_display_name` | string                                                | false    |              |                                                                                                                                                                       |
| `organization_id`           | string                                                | false    |              |                                                                                                                                                                       |
| `organization_name`         | string                                                | false    |              |                                                                                                                                                                       |
| `quota_allowance`           | integer                                               | false    |              |                                                                                                                                                                       |
| `source`                    | [codersdk.GroupSource](#codersdkgroupsource)          | false    |              |                                                                                                                                                                       |
| `total_member_count`        | integer                                               | false    |              | How many members are in this group. Shows the total count, even if the user is not authorized to read group member details. May be greater than `len(Group.Members)`. |

## codersdk.GroupSource

```json
"user"
```

### Properties

#### Enumerated Values

| Value  |
|--------|
| `user` |
| `oidc` |

## codersdk.GroupSyncSettings

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

### Properties

| Name                         | Type                           | Required | Restrictions | Description                                                                                                                                                                                                                                                                            |
|------------------------------|--------------------------------|----------|--------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `auto_create_missing_groups` | boolean                        | false    |              | Auto create missing groups controls whether groups returned by the OIDC provider are automatically created in Coder if they are missing.                                                                                                                                               |
| `field`                      | string                         | false    |              | Field is the name of the claim field that specifies what groups a user should be in. If empty, no groups will be synced.                                                                                                                                                               |
| `legacy_group_name_mapping`  | object                         | false    |              | Legacy group name mapping is deprecated. It remaps an IDP group name to a Coder group name. Since configuration is now done at runtime, group IDs are used to account for group renames. For legacy configurations, this config option has to remain. Deprecated: Use Mapping instead. |
| » `[any property]`           | string                         | false    |              |                                                                                                                                                                                                                                                                                        |
| `mapping`                    | object                         | false    |              | Mapping is a map from OIDC groups to Coder group IDs                                                                                                                                                                                                                                   |
| » `[any property]`           | array of string                | false    |              |                                                                                                                                                                                                                                                                                        |
| `regex_filter`               | [regexp.Regexp](#regexpregexp) | false    |              | Regex filter is a regular expression that filters the groups returned by the OIDC provider. Any group not matched by this regex will be ignored. If the group filter is nil, then no group filtering will occur.                                                                       |

## codersdk.Healthcheck

```json
{
  "interval": 0,
  "threshold": 0,
  "url": "string"
}
```

### Properties

| Name        | Type    | Required | Restrictions | Description                                                                                      |
|-------------|---------|----------|--------------|--------------------------------------------------------------------------------------------------|
| `interval`  | integer | false    |              | Interval specifies the seconds between each health check.                                        |
| `threshold` | integer | false    |              | Threshold specifies the number of consecutive failed health checks before returning "unhealthy". |
| `url`       | string  | false    |              | URL specifies the endpoint to check for the app health.                                          |

## codersdk.HealthcheckConfig

```json
{
  "refresh": 0,
  "threshold_database": 0
}
```

### Properties

| Name                 | Type    | Required | Restrictions | Description |
|----------------------|---------|----------|--------------|-------------|
| `refresh`            | integer | false    |              |             |
| `threshold_database` | integer | false    |              |             |

## codersdk.InboxNotification

```json
{
  "actions": [
    {
      "label": "string",
      "url": "string"
    }
  ],
  "content": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "read_at": "string",
  "targets": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "title": "string",
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
}
```

### Properties

| Name          | Type                                                                          | Required | Restrictions | Description |
|---------------|-------------------------------------------------------------------------------|----------|--------------|-------------|
| `actions`     | array of [codersdk.InboxNotificationAction](#codersdkinboxnotificationaction) | false    |              |             |
| `content`     | string                                                                        | false    |              |             |
| `created_at`  | string                                                                        | false    |              |             |
| `icon`        | string                                                                        | false    |              |             |
| `id`          | string                                                                        | false    |              |             |
| `read_at`     | string                                                                        | false    |              |             |
| `targets`     | array of string                                                               | false    |              |             |
| `template_id` | string                                                                        | false    |              |             |
| `title`       | string                                                                        | false    |              |             |
| `user_id`     | string                                                                        | false    |              |             |

## codersdk.InboxNotificationAction

```json
{
  "label": "string",
  "url": "string"
}
```

### Properties

| Name    | Type   | Required | Restrictions | Description |
|---------|--------|----------|--------------|-------------|
| `label` | string | false    |              |             |
| `url`   | string | false    |              |             |

## codersdk.InsightsReportInterval

```json
"day"
```

### Properties

#### Enumerated Values

| Value  |
|--------|
| `day`  |
| `week` |

## codersdk.IssueReconnectingPTYSignedTokenRequest

```json
{
  "agentID": "bc282582-04f9-45ce-b904-3e3bfab66958",
  "url": "string"
}
```

### Properties

| Name      | Type   | Required | Restrictions | Description                                                            |
|-----------|--------|----------|--------------|------------------------------------------------------------------------|
| `agentID` | string | true     |              |                                                                        |
| `url`     | string | true     |              | URL is the URL of the reconnecting-pty endpoint you are connecting to. |

## codersdk.IssueReconnectingPTYSignedTokenResponse

```json
{
  "signed_token": "string"
}
```

### Properties

| Name           | Type   | Required | Restrictions | Description |
|----------------|--------|----------|--------------|-------------|
| `signed_token` | string | false    |              |             |

## codersdk.JFrogXrayScan

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

### Properties

| Name           | Type    | Required | Restrictions | Description |
|----------------|---------|----------|--------------|-------------|
| `agent_id`     | string  | false    |              |             |
| `critical`     | integer | false    |              |             |
| `high`         | integer | false    |              |             |
| `medium`       | integer | false    |              |             |
| `results_url`  | string  | false    |              |             |
| `workspace_id` | string  | false    |              |             |

## codersdk.JobErrorCode

```json
"REQUIRED_TEMPLATE_VARIABLES"
```

### Properties

#### Enumerated Values

| Value                         |
|-------------------------------|
| `REQUIRED_TEMPLATE_VARIABLES` |

## codersdk.License

```json
{
  "claims": {},
  "id": 0,
  "uploaded_at": "2019-08-24T14:15:22Z",
  "uuid": "095be615-a8ad-4c33-8e9c-c7612fbf6c9f"
}
```

### Properties

| Name          | Type    | Required | Restrictions | Description                                                                                                                                                                                             |
|---------------|---------|----------|--------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `claims`      | object  | false    |              | Claims are the JWT claims asserted by the license.  Here we use a generic string map to ensure that all data from the server is parsed verbatim, not just the fields this version of Coder understands. |
| `id`          | integer | false    |              |                                                                                                                                                                                                         |
| `uploaded_at` | string  | false    |              |                                                                                                                                                                                                         |
| `uuid`        | string  | false    |              |                                                                                                                                                                                                         |

## codersdk.LinkConfig

```json
{
  "icon": "bug",
  "name": "string",
  "target": "string"
}
```

### Properties

| Name     | Type   | Required | Restrictions | Description |
|----------|--------|----------|--------------|-------------|
| `icon`   | string | false    |              |             |
| `name`   | string | false    |              |             |
| `target` | string | false    |              |             |

#### Enumerated Values

| Property | Value  |
|----------|--------|
| `icon`   | `bug`  |
| `icon`   | `chat` |
| `icon`   | `docs` |

## codersdk.ListInboxNotificationsResponse

```json
{
  "notifications": [
    {
      "actions": [
        {
          "label": "string",
          "url": "string"
        }
      ],
      "content": "string",
      "created_at": "2019-08-24T14:15:22Z",
      "icon": "string",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "read_at": "string",
      "targets": [
        "497f6eca-6276-4993-bfeb-53cbbbba6f08"
      ],
      "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
      "title": "string",
      "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
    }
  ],
  "unread_count": 0
}
```

### Properties

| Name            | Type                                                              | Required | Restrictions | Description |
|-----------------|-------------------------------------------------------------------|----------|--------------|-------------|
| `notifications` | array of [codersdk.InboxNotification](#codersdkinboxnotification) | false    |              |             |
| `unread_count`  | integer                                                           | false    |              |             |

## codersdk.LogLevel

```json
"trace"
```

### Properties

#### Enumerated Values

| Value   |
|---------|
| `trace` |
| `debug` |
| `info`  |
| `warn`  |
| `error` |

## codersdk.LogSource

```json
"provisioner_daemon"
```

### Properties

#### Enumerated Values

| Value                |
|----------------------|
| `provisioner_daemon` |
| `provisioner`        |

## codersdk.LoggingConfig

```json
{
  "human": "string",
  "json": "string",
  "log_filter": [
    "string"
  ],
  "stackdriver": "string"
}
```

### Properties

| Name          | Type            | Required | Restrictions | Description |
|---------------|-----------------|----------|--------------|-------------|
| `human`       | string          | false    |              |             |
| `json`        | string          | false    |              |             |
| `log_filter`  | array of string | false    |              |             |
| `stackdriver` | string          | false    |              |             |

## codersdk.LoginType

```json
""
```

### Properties

#### Enumerated Values

| Value      |
|------------|
| ``         |
| `password` |
| `github`   |
| `oidc`     |
| `token`    |
| `none`     |

## codersdk.LoginWithPasswordRequest

```json
{
  "email": "user@example.com",
  "password": "string"
}
```

### Properties

| Name       | Type   | Required | Restrictions | Description |
|------------|--------|----------|--------------|-------------|
| `email`    | string | true     |              |             |
| `password` | string | true     |              |             |

## codersdk.LoginWithPasswordResponse

```json
{
  "session_token": "string"
}
```

### Properties

| Name            | Type   | Required | Restrictions | Description |
|-----------------|--------|----------|--------------|-------------|
| `session_token` | string | true     |              |             |

## codersdk.MatchedProvisioners

```json
{
  "available": 0,
  "count": 0,
  "most_recently_seen": "2019-08-24T14:15:22Z"
}
```

### Properties

| Name                 | Type    | Required | Restrictions | Description                                                                                                                                                         |
|----------------------|---------|----------|--------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `available`          | integer | false    |              | Available is the number of provisioner daemons that are available to take jobs. This may be less than the count if some provisioners are busy or have been stopped. |
| `count`              | integer | false    |              | Count is the number of provisioner daemons that matched the given tags. If the count is 0, it means no provisioner daemons matched the requested tags.              |
| `most_recently_seen` | string  | false    |              | Most recently seen is the most recently seen time of the set of matched provisioners. If no provisioners matched, this field will be null.                          |

## codersdk.MinimalOrganization

```json
{
  "display_name": "string",
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string"
}
```

### Properties

| Name           | Type   | Required | Restrictions | Description |
|----------------|--------|----------|--------------|-------------|
| `display_name` | string | false    |              |             |
| `icon`         | string | false    |              |             |
| `id`           | string | true     |              |             |
| `name`         | string | false    |              |             |

## codersdk.MinimalUser

```json
{
  "avatar_url": "http://example.com",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "username": "string"
}
```

### Properties

| Name         | Type   | Required | Restrictions | Description |
|--------------|--------|----------|--------------|-------------|
| `avatar_url` | string | false    |              |             |
| `id`         | string | true     |              |             |
| `username`   | string | true     |              |             |

## codersdk.NotificationMethodsResponse

```json
{
  "available": [
    "string"
  ],
  "default": "string"
}
```

### Properties

| Name        | Type            | Required | Restrictions | Description |
|-------------|-----------------|----------|--------------|-------------|
| `available` | array of string | false    |              |             |
| `default`   | string          | false    |              |             |

## codersdk.NotificationPreference

```json
{
  "disabled": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Properties

| Name         | Type    | Required | Restrictions | Description |
|--------------|---------|----------|--------------|-------------|
| `disabled`   | boolean | false    |              |             |
| `id`         | string  | false    |              |             |
| `updated_at` | string  | false    |              |             |

## codersdk.NotificationTemplate

```json
{
  "actions": "string",
  "body_template": "string",
  "enabled_by_default": true,
  "group": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "kind": "string",
  "method": "string",
  "name": "string",
  "title_template": "string"
}
```

### Properties

| Name                 | Type    | Required | Restrictions | Description |
|----------------------|---------|----------|--------------|-------------|
| `actions`            | string  | false    |              |             |
| `body_template`      | string  | false    |              |             |
| `enabled_by_default` | boolean | false    |              |             |
| `group`              | string  | false    |              |             |
| `id`                 | string  | false    |              |             |
| `kind`               | string  | false    |              |             |
| `method`             | string  | false    |              |             |
| `name`               | string  | false    |              |             |
| `title_template`     | string  | false    |              |             |

## codersdk.NotificationsConfig

```json
{
  "dispatch_timeout": 0,
  "email": {
    "auth": {
      "identity": "string",
      "password": "string",
      "password_file": "string",
      "username": "string"
    },
    "force_tls": true,
    "from": "string",
    "hello": "string",
    "smarthost": "string",
    "tls": {
      "ca_file": "string",
      "cert_file": "string",
      "insecure_skip_verify": true,
      "key_file": "string",
      "server_name": "string",
      "start_tls": true
    }
  },
  "fetch_interval": 0,
  "inbox": {
    "enabled": true
  },
  "lease_count": 0,
  "lease_period": 0,
  "max_send_attempts": 0,
  "method": "string",
  "retry_interval": 0,
  "sync_buffer_size": 0,
  "sync_interval": 0,
  "webhook": {
    "endpoint": {
      "forceQuery": true,
      "fragment": "string",
      "host": "string",
      "omitHost": true,
      "opaque": "string",
      "path": "string",
      "rawFragment": "string",
      "rawPath": "string",
      "rawQuery": "string",
      "scheme": "string",
      "user": {}
    }
  }
}
```

### Properties

| Name                | Type                                                                       | Required | Restrictions | Description                                                                                                                                                                                                                                                                                                                                                                                                                                         |
|---------------------|----------------------------------------------------------------------------|----------|--------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `dispatch_timeout`  | integer                                                                    | false    |              | How long to wait while a notification is being sent before giving up.                                                                                                                                                                                                                                                                                                                                                                               |
| `email`             | [codersdk.NotificationsEmailConfig](#codersdknotificationsemailconfig)     | false    |              | Email settings.                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `fetch_interval`    | integer                                                                    | false    |              | How often to query the database for queued notifications.                                                                                                                                                                                                                                                                                                                                                                                           |
| `inbox`             | [codersdk.NotificationsInboxConfig](#codersdknotificationsinboxconfig)     | false    |              | Inbox settings.                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `lease_count`       | integer                                                                    | false    |              | How many notifications a notifier should lease per fetch interval.                                                                                                                                                                                                                                                                                                                                                                                  |
| `lease_period`      | integer                                                                    | false    |              | How long a notifier should lease a message. This is effectively how long a notification is 'owned' by a notifier, and once this period expires it will be available for lease by another notifier. Leasing is important in order for multiple running notifiers to not pick the same messages to deliver concurrently. This lease period will only expire if a notifier shuts down ungracefully; a dispatch of the notification releases the lease. |
| `max_send_attempts` | integer                                                                    | false    |              | The upper limit of attempts to send a notification.                                                                                                                                                                                                                                                                                                                                                                                                 |
| `method`            | string                                                                     | false    |              | Which delivery method to use (available options: 'smtp', 'webhook').                                                                                                                                                                                                                                                                                                                                                                                |
| `retry_interval`    | integer                                                                    | false    |              | The minimum time between retries.                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `sync_buffer_size`  | integer                                                                    | false    |              | The notifications system buffers message updates in memory to ease pressure on the database. This option controls how many updates are kept in memory. The lower this value the lower the change of state inconsistency in a non-graceful shutdown - but it also increases load on the database. It is recommended to keep this option at its default value.                                                                                        |
| `sync_interval`     | integer                                                                    | false    |              | The notifications system buffers message updates in memory to ease pressure on the database. This option controls how often it synchronizes its state with the database. The shorter this value the lower the change of state inconsistency in a non-graceful shutdown - but it also increases load on the database. It is recommended to keep this option at its default value.                                                                    |
| `webhook`           | [codersdk.NotificationsWebhookConfig](#codersdknotificationswebhookconfig) | false    |              | Webhook settings.                                                                                                                                                                                                                                                                                                                                                                                                                                   |

## codersdk.NotificationsEmailAuthConfig

```json
{
  "identity": "string",
  "password": "string",
  "password_file": "string",
  "username": "string"
}
```

### Properties

| Name            | Type   | Required | Restrictions | Description                                                |
|-----------------|--------|----------|--------------|------------------------------------------------------------|
| `identity`      | string | false    |              | Identity for PLAIN auth.                                   |
| `password`      | string | false    |              | Password for LOGIN/PLAIN auth.                             |
| `password_file` | string | false    |              | File from which to load the password for LOGIN/PLAIN auth. |
| `username`      | string | false    |              | Username for LOGIN/PLAIN auth.                             |

## codersdk.NotificationsEmailConfig

```json
{
  "auth": {
    "identity": "string",
    "password": "string",
    "password_file": "string",
    "username": "string"
  },
  "force_tls": true,
  "from": "string",
  "hello": "string",
  "smarthost": "string",
  "tls": {
    "ca_file": "string",
    "cert_file": "string",
    "insecure_skip_verify": true,
    "key_file": "string",
    "server_name": "string",
    "start_tls": true
  }
}
```

### Properties

| Name        | Type                                                                           | Required | Restrictions | Description                                                           |
|-------------|--------------------------------------------------------------------------------|----------|--------------|-----------------------------------------------------------------------|
| `auth`      | [codersdk.NotificationsEmailAuthConfig](#codersdknotificationsemailauthconfig) | false    |              | Authentication details.                                               |
| `force_tls` | boolean                                                                        | false    |              | Force tls causes a TLS connection to be attempted.                    |
| `from`      | string                                                                         | false    |              | The sender's address.                                                 |
| `hello`     | string                                                                         | false    |              | The hostname identifying the SMTP server.                             |
| `smarthost` | string                                                                         | false    |              | The intermediary SMTP host through which emails are sent (host:port). |
| `tls`       | [codersdk.NotificationsEmailTLSConfig](#codersdknotificationsemailtlsconfig)   | false    |              | Tls details.                                                          |

## codersdk.NotificationsEmailTLSConfig

```json
{
  "ca_file": "string",
  "cert_file": "string",
  "insecure_skip_verify": true,
  "key_file": "string",
  "server_name": "string",
  "start_tls": true
}
```

### Properties

| Name                   | Type    | Required | Restrictions | Description                                                  |
|------------------------|---------|----------|--------------|--------------------------------------------------------------|
| `ca_file`              | string  | false    |              | Ca file specifies the location of the CA certificate to use. |
| `cert_file`            | string  | false    |              | Cert file specifies the location of the certificate to use.  |
| `insecure_skip_verify` | boolean | false    |              | Insecure skip verify skips target certificate validation.    |
| `key_file`             | string  | false    |              | Key file specifies the location of the key to use.           |
| `server_name`          | string  | false    |              | Server name to verify the hostname for the targets.          |
| `start_tls`            | boolean | false    |              | Start tls attempts to upgrade plain connections to TLS.      |

## codersdk.NotificationsInboxConfig

```json
{
  "enabled": true
}
```

### Properties

| Name      | Type    | Required | Restrictions | Description |
|-----------|---------|----------|--------------|-------------|
| `enabled` | boolean | false    |              |             |

## codersdk.NotificationsSettings

```json
{
  "notifier_paused": true
}
```

### Properties

| Name              | Type    | Required | Restrictions | Description |
|-------------------|---------|----------|--------------|-------------|
| `notifier_paused` | boolean | false    |              |             |

## codersdk.NotificationsWebhookConfig

```json
{
  "endpoint": {
    "forceQuery": true,
    "fragment": "string",
    "host": "string",
    "omitHost": true,
    "opaque": "string",
    "path": "string",
    "rawFragment": "string",
    "rawPath": "string",
    "rawQuery": "string",
    "scheme": "string",
    "user": {}
  }
}
```

### Properties

| Name       | Type                       | Required | Restrictions | Description                                                          |
|------------|----------------------------|----------|--------------|----------------------------------------------------------------------|
| `endpoint` | [serpent.URL](#serpenturl) | false    |              | The URL to which the payload will be sent with an HTTP POST request. |

## codersdk.OAuth2AppEndpoints

```json
{
  "authorization": "string",
  "device_authorization": "string",
  "token": "string"
}
```

### Properties

| Name                   | Type   | Required | Restrictions | Description                       |
|------------------------|--------|----------|--------------|-----------------------------------|
| `authorization`        | string | false    |              |                                   |
| `device_authorization` | string | false    |              | Device authorization is optional. |
| `token`                | string | false    |              |                                   |

## codersdk.OAuth2Config

```json
{
  "github": {
    "allow_everyone": true,
    "allow_signups": true,
    "allowed_orgs": [
      "string"
    ],
    "allowed_teams": [
      "string"
    ],
    "client_id": "string",
    "client_secret": "string",
    "default_provider_enable": true,
    "device_flow": true,
    "enterprise_base_url": "string"
  }
}
```

### Properties

| Name     | Type                                                       | Required | Restrictions | Description |
|----------|------------------------------------------------------------|----------|--------------|-------------|
| `github` | [codersdk.OAuth2GithubConfig](#codersdkoauth2githubconfig) | false    |              |             |

## codersdk.OAuth2GithubConfig

```json
{
  "allow_everyone": true,
  "allow_signups": true,
  "allowed_orgs": [
    "string"
  ],
  "allowed_teams": [
    "string"
  ],
  "client_id": "string",
  "client_secret": "string",
  "default_provider_enable": true,
  "device_flow": true,
  "enterprise_base_url": "string"
}
```

### Properties

| Name                      | Type            | Required | Restrictions | Description |
|---------------------------|-----------------|----------|--------------|-------------|
| `allow_everyone`          | boolean         | false    |              |             |
| `allow_signups`           | boolean         | false    |              |             |
| `allowed_orgs`            | array of string | false    |              |             |
| `allowed_teams`           | array of string | false    |              |             |
| `client_id`               | string          | false    |              |             |
| `client_secret`           | string          | false    |              |             |
| `default_provider_enable` | boolean         | false    |              |             |
| `device_flow`             | boolean         | false    |              |             |
| `enterprise_base_url`     | string          | false    |              |             |

## codersdk.OAuth2ProviderApp

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

### Properties

| Name           | Type                                                       | Required | Restrictions | Description                                                                                                                                                                                             |
|----------------|------------------------------------------------------------|----------|--------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `callback_url` | string                                                     | false    |              |                                                                                                                                                                                                         |
| `endpoints`    | [codersdk.OAuth2AppEndpoints](#codersdkoauth2appendpoints) | false    |              | Endpoints are included in the app response for easier discovery. The OAuth2 spec does not have a defined place to find these (for comparison, OIDC has a '/.well-known/openid-configuration' endpoint). |
| `icon`         | string                                                     | false    |              |                                                                                                                                                                                                         |
| `id`           | string                                                     | false    |              |                                                                                                                                                                                                         |
| `name`         | string                                                     | false    |              |                                                                                                                                                                                                         |

## codersdk.OAuth2ProviderAppSecret

```json
{
  "client_secret_truncated": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "last_used_at": "string"
}
```

### Properties

| Name                      | Type   | Required | Restrictions | Description |
|---------------------------|--------|----------|--------------|-------------|
| `client_secret_truncated` | string | false    |              |             |
| `id`                      | string | false    |              |             |
| `last_used_at`            | string | false    |              |             |

## codersdk.OAuth2ProviderAppSecretFull

```json
{
  "client_secret_full": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08"
}
```

### Properties

| Name                 | Type   | Required | Restrictions | Description |
|----------------------|--------|----------|--------------|-------------|
| `client_secret_full` | string | false    |              |             |
| `id`                 | string | false    |              |             |

## codersdk.OAuthConversionResponse

```json
{
  "expires_at": "2019-08-24T14:15:22Z",
  "state_string": "string",
  "to_type": "",
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
}
```

### Properties

| Name           | Type                                     | Required | Restrictions | Description |
|----------------|------------------------------------------|----------|--------------|-------------|
| `expires_at`   | string                                   | false    |              |             |
| `state_string` | string                                   | false    |              |             |
| `to_type`      | [codersdk.LoginType](#codersdklogintype) | false    |              |             |
| `user_id`      | string                                   | false    |              |             |

## codersdk.OIDCAuthMethod

```json
{
  "enabled": true,
  "iconUrl": "string",
  "signInText": "string"
}
```

### Properties

| Name         | Type    | Required | Restrictions | Description |
|--------------|---------|----------|--------------|-------------|
| `enabled`    | boolean | false    |              |             |
| `iconUrl`    | string  | false    |              |             |
| `signInText` | string  | false    |              |             |

## codersdk.OIDCConfig

```json
{
  "allow_signups": true,
  "auth_url_params": {},
  "client_cert_file": "string",
  "client_id": "string",
  "client_key_file": "string",
  "client_secret": "string",
  "email_domain": [
    "string"
  ],
  "email_field": "string",
  "group_allow_list": [
    "string"
  ],
  "group_auto_create": true,
  "group_mapping": {},
  "group_regex_filter": {},
  "groups_field": "string",
  "icon_url": {
    "forceQuery": true,
    "fragment": "string",
    "host": "string",
    "omitHost": true,
    "opaque": "string",
    "path": "string",
    "rawFragment": "string",
    "rawPath": "string",
    "rawQuery": "string",
    "scheme": "string",
    "user": {}
  },
  "ignore_email_verified": true,
  "ignore_user_info": true,
  "issuer_url": "string",
  "name_field": "string",
  "organization_assign_default": true,
  "organization_field": "string",
  "organization_mapping": {},
  "scopes": [
    "string"
  ],
  "sign_in_text": "string",
  "signups_disabled_text": "string",
  "skip_issuer_checks": true,
  "source_user_info_from_access_token": true,
  "user_role_field": "string",
  "user_role_mapping": {},
  "user_roles_default": [
    "string"
  ],
  "username_field": "string"
}
```

### Properties

| Name                                 | Type                             | Required | Restrictions | Description                                                                                                                                                                                                                                                                                                                                                        |
|--------------------------------------|----------------------------------|----------|--------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `allow_signups`                      | boolean                          | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `auth_url_params`                    | object                           | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `client_cert_file`                   | string                           | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `client_id`                          | string                           | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `client_key_file`                    | string                           | false    |              | Client key file & ClientCertFile are used in place of ClientSecret for PKI auth.                                                                                                                                                                                                                                                                                   |
| `client_secret`                      | string                           | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `email_domain`                       | array of string                  | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `email_field`                        | string                           | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `group_allow_list`                   | array of string                  | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `group_auto_create`                  | boolean                          | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `group_mapping`                      | object                           | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `group_regex_filter`                 | [serpent.Regexp](#serpentregexp) | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `groups_field`                       | string                           | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `icon_url`                           | [serpent.URL](#serpenturl)       | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `ignore_email_verified`              | boolean                          | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `ignore_user_info`                   | boolean                          | false    |              | Ignore user info & UserInfoFromAccessToken are mutually exclusive. Only 1 can be set to true. Ideally this would be an enum with 3 states, ['none', 'userinfo', 'access_token']. However, for backward compatibility, `ignore_user_info` must remain. And `access_token` is a niche, non-spec compliant edge case. So it's use is rare, and should not be advised. |
| `issuer_url`                         | string                           | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `name_field`                         | string                           | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `organization_assign_default`        | boolean                          | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `organization_field`                 | string                           | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `organization_mapping`               | object                           | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `scopes`                             | array of string                  | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `sign_in_text`                       | string                           | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `signups_disabled_text`              | string                           | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `skip_issuer_checks`                 | boolean                          | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `source_user_info_from_access_token` | boolean                          | false    |              | Source user info from access token as mentioned above is an edge case. This allows sourcing the user_info from the access token itself instead of a user_info endpoint. This assumes the access token is a valid JWT with a set of claims to be merged with the id_token.                                                                                          |
| `user_role_field`                    | string                           | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `user_role_mapping`                  | object                           | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `user_roles_default`                 | array of string                  | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |
| `username_field`                     | string                           | false    |              |                                                                                                                                                                                                                                                                                                                                                                    |

## codersdk.Organization

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "description": "string",
  "display_name": "string",
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "is_default": true,
  "name": "string",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Properties

| Name           | Type    | Required | Restrictions | Description |
|----------------|---------|----------|--------------|-------------|
| `created_at`   | string  | true     |              |             |
| `description`  | string  | false    |              |             |
| `display_name` | string  | false    |              |             |
| `icon`         | string  | false    |              |             |
| `id`           | string  | true     |              |             |
| `is_default`   | boolean | true     |              |             |
| `name`         | string  | false    |              |             |
| `updated_at`   | string  | true     |              |             |

## codersdk.OrganizationMember

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

### Properties

| Name              | Type                                            | Required | Restrictions | Description |
|-------------------|-------------------------------------------------|----------|--------------|-------------|
| `created_at`      | string                                          | false    |              |             |
| `organization_id` | string                                          | false    |              |             |
| `roles`           | array of [codersdk.SlimRole](#codersdkslimrole) | false    |              |             |
| `updated_at`      | string                                          | false    |              |             |
| `user_id`         | string                                          | false    |              |             |

## codersdk.OrganizationMemberWithUserData

```json
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
```

### Properties

| Name              | Type                                            | Required | Restrictions | Description |
|-------------------|-------------------------------------------------|----------|--------------|-------------|
| `avatar_url`      | string                                          | false    |              |             |
| `created_at`      | string                                          | false    |              |             |
| `email`           | string                                          | false    |              |             |
| `global_roles`    | array of [codersdk.SlimRole](#codersdkslimrole) | false    |              |             |
| `name`            | string                                          | false    |              |             |
| `organization_id` | string                                          | false    |              |             |
| `roles`           | array of [codersdk.SlimRole](#codersdkslimrole) | false    |              |             |
| `updated_at`      | string                                          | false    |              |             |
| `user_id`         | string                                          | false    |              |             |
| `username`        | string                                          | false    |              |             |

## codersdk.OrganizationSyncSettings

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

### Properties

| Name                          | Type            | Required | Restrictions | Description                                                                                                                                                                         |
|-------------------------------|-----------------|----------|--------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `field`                       | string          | false    |              | Field selects the claim field to be used as the created user's organizations. If the field is the empty string, then no organization updates will ever come from the OIDC provider. |
| `mapping`                     | object          | false    |              | Mapping maps from an OIDC claim --> Coder organization uuid                                                                                                                         |
| » `[any property]`            | array of string | false    |              |                                                                                                                                                                                     |
| `organization_assign_default` | boolean         | false    |              | Organization assign default will ensure the default org is always included for every user, regardless of their claims. This preserves legacy behavior.                              |

## codersdk.PaginatedMembersResponse

```json
{
  "count": 0,
  "members": [
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
}
```

### Properties

| Name      | Type                                                                                        | Required | Restrictions | Description |
|-----------|---------------------------------------------------------------------------------------------|----------|--------------|-------------|
| `count`   | integer                                                                                     | false    |              |             |
| `members` | array of [codersdk.OrganizationMemberWithUserData](#codersdkorganizationmemberwithuserdata) | false    |              |             |

## codersdk.PatchGroupIDPSyncConfigRequest

```json
{
  "auto_create_missing_groups": true,
  "field": "string",
  "regex_filter": {}
}
```

### Properties

| Name                         | Type                           | Required | Restrictions | Description |
|------------------------------|--------------------------------|----------|--------------|-------------|
| `auto_create_missing_groups` | boolean                        | false    |              |             |
| `field`                      | string                         | false    |              |             |
| `regex_filter`               | [regexp.Regexp](#regexpregexp) | false    |              |             |

## codersdk.PatchGroupIDPSyncMappingRequest

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

### Properties

| Name      | Type            | Required | Restrictions | Description                                              |
|-----------|-----------------|----------|--------------|----------------------------------------------------------|
| `add`     | array of object | false    |              |                                                          |
| `» gets`  | string          | false    |              | The ID of the Coder resource the user should be added to |
| `» given` | string          | false    |              | The IdP claim the user has                               |
| `remove`  | array of object | false    |              |                                                          |
| `» gets`  | string          | false    |              | The ID of the Coder resource the user should be added to |
| `» given` | string          | false    |              | The IdP claim the user has                               |

## codersdk.PatchGroupRequest

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

### Properties

| Name              | Type            | Required | Restrictions | Description |
|-------------------|-----------------|----------|--------------|-------------|
| `add_users`       | array of string | false    |              |             |
| `avatar_url`      | string          | false    |              |             |
| `display_name`    | string          | false    |              |             |
| `name`            | string          | false    |              |             |
| `quota_allowance` | integer         | false    |              |             |
| `remove_users`    | array of string | false    |              |             |

## codersdk.PatchOrganizationIDPSyncConfigRequest

```json
{
  "assign_default": true,
  "field": "string"
}
```

### Properties

| Name             | Type    | Required | Restrictions | Description |
|------------------|---------|----------|--------------|-------------|
| `assign_default` | boolean | false    |              |             |
| `field`          | string  | false    |              |             |

## codersdk.PatchOrganizationIDPSyncMappingRequest

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

### Properties

| Name      | Type            | Required | Restrictions | Description                                              |
|-----------|-----------------|----------|--------------|----------------------------------------------------------|
| `add`     | array of object | false    |              |                                                          |
| `» gets`  | string          | false    |              | The ID of the Coder resource the user should be added to |
| `» given` | string          | false    |              | The IdP claim the user has                               |
| `remove`  | array of object | false    |              |                                                          |
| `» gets`  | string          | false    |              | The ID of the Coder resource the user should be added to |
| `» given` | string          | false    |              | The IdP claim the user has                               |

## codersdk.PatchRoleIDPSyncConfigRequest

```json
{
  "field": "string"
}
```

### Properties

| Name    | Type   | Required | Restrictions | Description |
|---------|--------|----------|--------------|-------------|
| `field` | string | false    |              |             |

## codersdk.PatchRoleIDPSyncMappingRequest

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

### Properties

| Name      | Type            | Required | Restrictions | Description                                              |
|-----------|-----------------|----------|--------------|----------------------------------------------------------|
| `add`     | array of object | false    |              |                                                          |
| `» gets`  | string          | false    |              | The ID of the Coder resource the user should be added to |
| `» given` | string          | false    |              | The IdP claim the user has                               |
| `remove`  | array of object | false    |              |                                                          |
| `» gets`  | string          | false    |              | The ID of the Coder resource the user should be added to |
| `» given` | string          | false    |              | The IdP claim the user has                               |

## codersdk.PatchTemplateVersionRequest

```json
{
  "message": "string",
  "name": "string"
}
```

### Properties

| Name      | Type   | Required | Restrictions | Description |
|-----------|--------|----------|--------------|-------------|
| `message` | string | false    |              |             |
| `name`    | string | false    |              |             |

## codersdk.PatchWorkspaceProxy

```json
{
  "display_name": "string",
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "regenerate_token": true
}
```

### Properties

| Name               | Type    | Required | Restrictions | Description |
|--------------------|---------|----------|--------------|-------------|
| `display_name`     | string  | true     |              |             |
| `icon`             | string  | true     |              |             |
| `id`               | string  | true     |              |             |
| `name`             | string  | true     |              |             |
| `regenerate_token` | boolean | false    |              |             |

## codersdk.Permission

```json
{
  "action": "application_connect",
  "negate": true,
  "resource_type": "*"
}
```

### Properties

| Name            | Type                                           | Required | Restrictions | Description                             |
|-----------------|------------------------------------------------|----------|--------------|-----------------------------------------|
| `action`        | [codersdk.RBACAction](#codersdkrbacaction)     | false    |              |                                         |
| `negate`        | boolean                                        | false    |              | Negate makes this a negative permission |
| `resource_type` | [codersdk.RBACResource](#codersdkrbacresource) | false    |              |                                         |

## codersdk.PostOAuth2ProviderAppRequest

```json
{
  "callback_url": "string",
  "icon": "string",
  "name": "string"
}
```

### Properties

| Name           | Type   | Required | Restrictions | Description |
|----------------|--------|----------|--------------|-------------|
| `callback_url` | string | true     |              |             |
| `icon`         | string | false    |              |             |
| `name`         | string | true     |              |             |

## codersdk.PostWorkspaceUsageRequest

```json
{
  "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
  "app_name": "vscode"
}
```

### Properties

| Name       | Type                                           | Required | Restrictions | Description |
|------------|------------------------------------------------|----------|--------------|-------------|
| `agent_id` | string                                         | false    |              |             |
| `app_name` | [codersdk.UsageAppName](#codersdkusageappname) | false    |              |             |

## codersdk.PprofConfig

```json
{
  "address": {
    "host": "string",
    "port": "string"
  },
  "enable": true
}
```

### Properties

| Name      | Type                                 | Required | Restrictions | Description |
|-----------|--------------------------------------|----------|--------------|-------------|
| `address` | [serpent.HostPort](#serpenthostport) | false    |              |             |
| `enable`  | boolean                              | false    |              |             |

## codersdk.Preset

```json
{
  "id": "string",
  "name": "string",
  "parameters": [
    {
      "name": "string",
      "value": "string"
    }
  ]
}
```

### Properties

| Name         | Type                                                          | Required | Restrictions | Description |
|--------------|---------------------------------------------------------------|----------|--------------|-------------|
| `id`         | string                                                        | false    |              |             |
| `name`       | string                                                        | false    |              |             |
| `parameters` | array of [codersdk.PresetParameter](#codersdkpresetparameter) | false    |              |             |

## codersdk.PresetParameter

```json
{
  "name": "string",
  "value": "string"
}
```

### Properties

| Name    | Type   | Required | Restrictions | Description |
|---------|--------|----------|--------------|-------------|
| `name`  | string | false    |              |             |
| `value` | string | false    |              |             |

## codersdk.PrometheusConfig

```json
{
  "address": {
    "host": "string",
    "port": "string"
  },
  "aggregate_agent_stats_by": [
    "string"
  ],
  "collect_agent_stats": true,
  "collect_db_metrics": true,
  "enable": true
}
```

### Properties

| Name                       | Type                                 | Required | Restrictions | Description |
|----------------------------|--------------------------------------|----------|--------------|-------------|
| `address`                  | [serpent.HostPort](#serpenthostport) | false    |              |             |
| `aggregate_agent_stats_by` | array of string                      | false    |              |             |
| `collect_agent_stats`      | boolean                              | false    |              |             |
| `collect_db_metrics`       | boolean                              | false    |              |             |
| `enable`                   | boolean                              | false    |              |             |

## codersdk.ProvisionerConfig

```json
{
  "daemon_poll_interval": 0,
  "daemon_poll_jitter": 0,
  "daemon_psk": "string",
  "daemon_types": [
    "string"
  ],
  "daemons": 0,
  "force_cancel_interval": 0
}
```

### Properties

| Name                    | Type            | Required | Restrictions | Description                                               |
|-------------------------|-----------------|----------|--------------|-----------------------------------------------------------|
| `daemon_poll_interval`  | integer         | false    |              |                                                           |
| `daemon_poll_jitter`    | integer         | false    |              |                                                           |
| `daemon_psk`            | string          | false    |              |                                                           |
| `daemon_types`          | array of string | false    |              |                                                           |
| `daemons`               | integer         | false    |              | Daemons is the number of built-in terraform provisioners. |
| `force_cancel_interval` | integer         | false    |              |                                                           |

## codersdk.ProvisionerDaemon

```json
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
```

### Properties

| Name               | Type                                                                 | Required | Restrictions | Description      |
|--------------------|----------------------------------------------------------------------|----------|--------------|------------------|
| `api_version`      | string                                                               | false    |              |                  |
| `created_at`       | string                                                               | false    |              |                  |
| `current_job`      | [codersdk.ProvisionerDaemonJob](#codersdkprovisionerdaemonjob)       | false    |              |                  |
| `id`               | string                                                               | false    |              |                  |
| `key_id`           | string                                                               | false    |              |                  |
| `key_name`         | string                                                               | false    |              | Optional fields. |
| `last_seen_at`     | string                                                               | false    |              |                  |
| `name`             | string                                                               | false    |              |                  |
| `organization_id`  | string                                                               | false    |              |                  |
| `previous_job`     | [codersdk.ProvisionerDaemonJob](#codersdkprovisionerdaemonjob)       | false    |              |                  |
| `provisioners`     | array of string                                                      | false    |              |                  |
| `status`           | [codersdk.ProvisionerDaemonStatus](#codersdkprovisionerdaemonstatus) | false    |              |                  |
| `tags`             | object                                                               | false    |              |                  |
| » `[any property]` | string                                                               | false    |              |                  |
| `version`          | string                                                               | false    |              |                  |

#### Enumerated Values

| Property | Value     |
|----------|-----------|
| `status` | `offline` |
| `status` | `idle`    |
| `status` | `busy`    |

## codersdk.ProvisionerDaemonJob

```json
{
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "status": "pending",
  "template_display_name": "string",
  "template_icon": "string",
  "template_name": "string"
}
```

### Properties

| Name                    | Type                                                           | Required | Restrictions | Description |
|-------------------------|----------------------------------------------------------------|----------|--------------|-------------|
| `id`                    | string                                                         | false    |              |             |
| `status`                | [codersdk.ProvisionerJobStatus](#codersdkprovisionerjobstatus) | false    |              |             |
| `template_display_name` | string                                                         | false    |              |             |
| `template_icon`         | string                                                         | false    |              |             |
| `template_name`         | string                                                         | false    |              |             |

#### Enumerated Values

| Property | Value       |
|----------|-------------|
| `status` | `pending`   |
| `status` | `running`   |
| `status` | `succeeded` |
| `status` | `canceling` |
| `status` | `canceled`  |
| `status` | `failed`    |

## codersdk.ProvisionerDaemonStatus

```json
"offline"
```

### Properties

#### Enumerated Values

| Value     |
|-----------|
| `offline` |
| `idle`    |
| `busy`    |

## codersdk.ProvisionerJob

```json
{
  "available_workers": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "canceled_at": "2019-08-24T14:15:22Z",
  "completed_at": "2019-08-24T14:15:22Z",
  "created_at": "2019-08-24T14:15:22Z",
  "error": "string",
  "error_code": "REQUIRED_TEMPLATE_VARIABLES",
  "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "input": {
    "error": "string",
    "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
    "workspace_build_id": "badaf2eb-96c5-4050-9f1d-db2d39ca5478"
  },
  "metadata": {
    "template_display_name": "string",
    "template_icon": "string",
    "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
    "template_name": "string",
    "template_version_name": "string",
    "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
    "workspace_name": "string"
  },
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "queue_position": 0,
  "queue_size": 0,
  "started_at": "2019-08-24T14:15:22Z",
  "status": "pending",
  "tags": {
    "property1": "string",
    "property2": "string"
  },
  "type": "template_version_import",
  "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
}
```

### Properties

| Name                | Type                                                               | Required | Restrictions | Description |
|---------------------|--------------------------------------------------------------------|----------|--------------|-------------|
| `available_workers` | array of string                                                    | false    |              |             |
| `canceled_at`       | string                                                             | false    |              |             |
| `completed_at`      | string                                                             | false    |              |             |
| `created_at`        | string                                                             | false    |              |             |
| `error`             | string                                                             | false    |              |             |
| `error_code`        | [codersdk.JobErrorCode](#codersdkjoberrorcode)                     | false    |              |             |
| `file_id`           | string                                                             | false    |              |             |
| `id`                | string                                                             | false    |              |             |
| `input`             | [codersdk.ProvisionerJobInput](#codersdkprovisionerjobinput)       | false    |              |             |
| `metadata`          | [codersdk.ProvisionerJobMetadata](#codersdkprovisionerjobmetadata) | false    |              |             |
| `organization_id`   | string                                                             | false    |              |             |
| `queue_position`    | integer                                                            | false    |              |             |
| `queue_size`        | integer                                                            | false    |              |             |
| `started_at`        | string                                                             | false    |              |             |
| `status`            | [codersdk.ProvisionerJobStatus](#codersdkprovisionerjobstatus)     | false    |              |             |
| `tags`              | object                                                             | false    |              |             |
| » `[any property]`  | string                                                             | false    |              |             |
| `type`              | [codersdk.ProvisionerJobType](#codersdkprovisionerjobtype)         | false    |              |             |
| `worker_id`         | string                                                             | false    |              |             |

#### Enumerated Values

| Property     | Value                         |
|--------------|-------------------------------|
| `error_code` | `REQUIRED_TEMPLATE_VARIABLES` |
| `status`     | `pending`                     |
| `status`     | `running`                     |
| `status`     | `succeeded`                   |
| `status`     | `canceling`                   |
| `status`     | `canceled`                    |
| `status`     | `failed`                      |

## codersdk.ProvisionerJobInput

```json
{
  "error": "string",
  "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
  "workspace_build_id": "badaf2eb-96c5-4050-9f1d-db2d39ca5478"
}
```

### Properties

| Name                  | Type   | Required | Restrictions | Description |
|-----------------------|--------|----------|--------------|-------------|
| `error`               | string | false    |              |             |
| `template_version_id` | string | false    |              |             |
| `workspace_build_id`  | string | false    |              |             |

## codersdk.ProvisionerJobLog

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "id": 0,
  "log_level": "trace",
  "log_source": "provisioner_daemon",
  "output": "string",
  "stage": "string"
}
```

### Properties

| Name         | Type                                     | Required | Restrictions | Description |
|--------------|------------------------------------------|----------|--------------|-------------|
| `created_at` | string                                   | false    |              |             |
| `id`         | integer                                  | false    |              |             |
| `log_level`  | [codersdk.LogLevel](#codersdkloglevel)   | false    |              |             |
| `log_source` | [codersdk.LogSource](#codersdklogsource) | false    |              |             |
| `output`     | string                                   | false    |              |             |
| `stage`      | string                                   | false    |              |             |

#### Enumerated Values

| Property    | Value   |
|-------------|---------|
| `log_level` | `trace` |
| `log_level` | `debug` |
| `log_level` | `info`  |
| `log_level` | `warn`  |
| `log_level` | `error` |

## codersdk.ProvisionerJobMetadata

```json
{
  "template_display_name": "string",
  "template_icon": "string",
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "template_name": "string",
  "template_version_name": "string",
  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
  "workspace_name": "string"
}
```

### Properties

| Name                    | Type   | Required | Restrictions | Description |
|-------------------------|--------|----------|--------------|-------------|
| `template_display_name` | string | false    |              |             |
| `template_icon`         | string | false    |              |             |
| `template_id`           | string | false    |              |             |
| `template_name`         | string | false    |              |             |
| `template_version_name` | string | false    |              |             |
| `workspace_id`          | string | false    |              |             |
| `workspace_name`        | string | false    |              |             |

## codersdk.ProvisionerJobStatus

```json
"pending"
```

### Properties

#### Enumerated Values

| Value       |
|-------------|
| `pending`   |
| `running`   |
| `succeeded` |
| `canceling` |
| `canceled`  |
| `failed`    |
| `unknown`   |

## codersdk.ProvisionerJobType

```json
"template_version_import"
```

### Properties

#### Enumerated Values

| Value                      |
|----------------------------|
| `template_version_import`  |
| `workspace_build`          |
| `template_version_dry_run` |

## codersdk.ProvisionerKey

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

### Properties

| Name           | Type                                                       | Required | Restrictions | Description |
|----------------|------------------------------------------------------------|----------|--------------|-------------|
| `created_at`   | string                                                     | false    |              |             |
| `id`           | string                                                     | false    |              |             |
| `name`         | string                                                     | false    |              |             |
| `organization` | string                                                     | false    |              |             |
| `tags`         | [codersdk.ProvisionerKeyTags](#codersdkprovisionerkeytags) | false    |              |             |

## codersdk.ProvisionerKeyDaemons

```json
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
```

### Properties

| Name      | Type                                                              | Required | Restrictions | Description |
|-----------|-------------------------------------------------------------------|----------|--------------|-------------|
| `daemons` | array of [codersdk.ProvisionerDaemon](#codersdkprovisionerdaemon) | false    |              |             |
| `key`     | [codersdk.ProvisionerKey](#codersdkprovisionerkey)                | false    |              |             |

## codersdk.ProvisionerKeyTags

```json
{
  "property1": "string",
  "property2": "string"
}
```

### Properties

| Name             | Type   | Required | Restrictions | Description |
|------------------|--------|----------|--------------|-------------|
| `[any property]` | string | false    |              |             |

## codersdk.ProvisionerLogLevel

```json
"debug"
```

### Properties

#### Enumerated Values

| Value   |
|---------|
| `debug` |

## codersdk.ProvisionerStorageMethod

```json
"file"
```

### Properties

#### Enumerated Values

| Value  |
|--------|
| `file` |

## codersdk.ProvisionerTiming

```json
{
  "action": "string",
  "ended_at": "2019-08-24T14:15:22Z",
  "job_id": "453bd7d7-5355-4d6d-a38e-d9e7eb218c3f",
  "resource": "string",
  "source": "string",
  "stage": "init",
  "started_at": "2019-08-24T14:15:22Z"
}
```

### Properties

| Name         | Type                                         | Required | Restrictions | Description |
|--------------|----------------------------------------------|----------|--------------|-------------|
| `action`     | string                                       | false    |              |             |
| `ended_at`   | string                                       | false    |              |             |
| `job_id`     | string                                       | false    |              |             |
| `resource`   | string                                       | false    |              |             |
| `source`     | string                                       | false    |              |             |
| `stage`      | [codersdk.TimingStage](#codersdktimingstage) | false    |              |             |
| `started_at` | string                                       | false    |              |             |

## codersdk.ProxyHealthReport

```json
{
  "errors": [
    "string"
  ],
  "warnings": [
    "string"
  ]
}
```

### Properties

| Name       | Type            | Required | Restrictions | Description                                                                              |
|------------|-----------------|----------|--------------|------------------------------------------------------------------------------------------|
| `errors`   | array of string | false    |              | Errors are problems that prevent the workspace proxy from being healthy                  |
| `warnings` | array of string | false    |              | Warnings do not prevent the workspace proxy from being healthy, but should be addressed. |

## codersdk.ProxyHealthStatus

```json
"ok"
```

### Properties

#### Enumerated Values

| Value          |
|----------------|
| `ok`           |
| `unreachable`  |
| `unhealthy`    |
| `unregistered` |

## codersdk.PutExtendWorkspaceRequest

```json
{
  "deadline": "2019-08-24T14:15:22Z"
}
```

### Properties

| Name       | Type   | Required | Restrictions | Description |
|------------|--------|----------|--------------|-------------|
| `deadline` | string | true     |              |             |

## codersdk.PutOAuth2ProviderAppRequest

```json
{
  "callback_url": "string",
  "icon": "string",
  "name": "string"
}
```

### Properties

| Name           | Type   | Required | Restrictions | Description |
|----------------|--------|----------|--------------|-------------|
| `callback_url` | string | true     |              |             |
| `icon`         | string | false    |              |             |
| `name`         | string | true     |              |             |

## codersdk.RBACAction

```json
"application_connect"
```

### Properties

#### Enumerated Values

| Value                 |
|-----------------------|
| `application_connect` |
| `assign`              |
| `create`              |
| `delete`              |
| `read`                |
| `read_personal`       |
| `ssh`                 |
| `unassign`            |
| `update`              |
| `update_personal`     |
| `use`                 |
| `view_insights`       |
| `start`               |
| `stop`                |

## codersdk.RBACResource

```json
"*"
```

### Properties

#### Enumerated Values

| Value                              |
|------------------------------------|
| `*`                                |
| `api_key`                          |
| `assign_org_role`                  |
| `assign_role`                      |
| `audit_log`                        |
| `crypto_key`                       |
| `debug_info`                       |
| `deployment_config`                |
| `deployment_stats`                 |
| `file`                             |
| `group`                            |
| `group_member`                     |
| `idpsync_settings`                 |
| `inbox_notification`               |
| `license`                          |
| `notification_message`             |
| `notification_preference`          |
| `notification_template`            |
| `oauth2_app`                       |
| `oauth2_app_code_token`            |
| `oauth2_app_secret`                |
| `organization`                     |
| `organization_member`              |
| `provisioner_daemon`               |
| `provisioner_jobs`                 |
| `replicas`                         |
| `system`                           |
| `tailnet_coordinator`              |
| `template`                         |
| `user`                             |
| `webpush_subscription`             |
| `workspace`                        |
| `workspace_agent_devcontainers`    |
| `workspace_agent_resource_monitor` |
| `workspace_dormant`                |
| `workspace_proxy`                  |

## codersdk.RateLimitConfig

```json
{
  "api": 0,
  "disable_all": true
}
```

### Properties

| Name          | Type    | Required | Restrictions | Description |
|---------------|---------|----------|--------------|-------------|
| `api`         | integer | false    |              |             |
| `disable_all` | boolean | false    |              |             |

## codersdk.ReducedUser

```json
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
```

### Properties

| Name               | Type                                       | Required | Restrictions | Description                                                                                |
|--------------------|--------------------------------------------|----------|--------------|--------------------------------------------------------------------------------------------|
| `avatar_url`       | string                                     | false    |              |                                                                                            |
| `created_at`       | string                                     | true     |              |                                                                                            |
| `email`            | string                                     | true     |              |                                                                                            |
| `id`               | string                                     | true     |              |                                                                                            |
| `last_seen_at`     | string                                     | false    |              |                                                                                            |
| `login_type`       | [codersdk.LoginType](#codersdklogintype)   | false    |              |                                                                                            |
| `name`             | string                                     | false    |              |                                                                                            |
| `status`           | [codersdk.UserStatus](#codersdkuserstatus) | false    |              |                                                                                            |
| `theme_preference` | string                                     | false    |              | Deprecated: this value should be retrieved from `codersdk.UserPreferenceSettings` instead. |
| `updated_at`       | string                                     | false    |              |                                                                                            |
| `username`         | string                                     | true     |              |                                                                                            |

#### Enumerated Values

| Property | Value       |
|----------|-------------|
| `status` | `active`    |
| `status` | `suspended` |

## codersdk.Region

```json
{
  "display_name": "string",
  "healthy": true,
  "icon_url": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "path_app_url": "string",
  "wildcard_hostname": "string"
}
```

### Properties

| Name                | Type    | Required | Restrictions | Description                                                                                                                                                                       |
|---------------------|---------|----------|--------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `display_name`      | string  | false    |              |                                                                                                                                                                                   |
| `healthy`           | boolean | false    |              |                                                                                                                                                                                   |
| `icon_url`          | string  | false    |              |                                                                                                                                                                                   |
| `id`                | string  | false    |              |                                                                                                                                                                                   |
| `name`              | string  | false    |              |                                                                                                                                                                                   |
| `path_app_url`      | string  | false    |              | Path app URL is the URL to the base path for path apps. Optional unless wildcard_hostname is set. E.g. https://us.example.com                                                     |
| `wildcard_hostname` | string  | false    |              | Wildcard hostname is the wildcard hostname for subdomain apps. E.g. *.us.example.com E.g.*--suffix.au.example.com Optional. Does not need to be on the same domain as PathAppURL. |

## codersdk.RegionsResponse-codersdk_Region

```json
{
  "regions": [
    {
      "display_name": "string",
      "healthy": true,
      "icon_url": "string",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "name": "string",
      "path_app_url": "string",
      "wildcard_hostname": "string"
    }
  ]
}
```

### Properties

| Name      | Type                                        | Required | Restrictions | Description |
|-----------|---------------------------------------------|----------|--------------|-------------|
| `regions` | array of [codersdk.Region](#codersdkregion) | false    |              |             |

## codersdk.RegionsResponse-codersdk_WorkspaceProxy

```json
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
```

### Properties

| Name      | Type                                                        | Required | Restrictions | Description |
|-----------|-------------------------------------------------------------|----------|--------------|-------------|
| `regions` | array of [codersdk.WorkspaceProxy](#codersdkworkspaceproxy) | false    |              |             |

## codersdk.Replica

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "database_latency": 0,
  "error": "string",
  "hostname": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "region_id": 0,
  "relay_address": "string"
}
```

### Properties

| Name               | Type    | Required | Restrictions | Description                                                        |
|--------------------|---------|----------|--------------|--------------------------------------------------------------------|
| `created_at`       | string  | false    |              | Created at is the timestamp when the replica was first seen.       |
| `database_latency` | integer | false    |              | Database latency is the latency in microseconds to the database.   |
| `error`            | string  | false    |              | Error is the replica error.                                        |
| `hostname`         | string  | false    |              | Hostname is the hostname of the replica.                           |
| `id`               | string  | false    |              | ID is the unique identifier for the replica.                       |
| `region_id`        | integer | false    |              | Region ID is the region of the replica.                            |
| `relay_address`    | string  | false    |              | Relay address is the accessible address to relay DERP connections. |

## codersdk.RequestOneTimePasscodeRequest

```json
{
  "email": "user@example.com"
}
```

### Properties

| Name    | Type   | Required | Restrictions | Description |
|---------|--------|----------|--------------|-------------|
| `email` | string | true     |              |             |

## codersdk.ResolveAutostartResponse

```json
{
  "parameter_mismatch": true
}
```

### Properties

| Name                 | Type    | Required | Restrictions | Description |
|----------------------|---------|----------|--------------|-------------|
| `parameter_mismatch` | boolean | false    |              |             |

## codersdk.ResourceType

```json
"template"
```

### Properties

#### Enumerated Values

| Value                            |
|----------------------------------|
| `template`                       |
| `template_version`               |
| `user`                           |
| `workspace`                      |
| `workspace_build`                |
| `git_ssh_key`                    |
| `api_key`                        |
| `group`                          |
| `license`                        |
| `convert_login`                  |
| `health_settings`                |
| `notifications_settings`         |
| `workspace_proxy`                |
| `organization`                   |
| `oauth2_provider_app`            |
| `oauth2_provider_app_secret`     |
| `custom_role`                    |
| `organization_member`            |
| `notification_template`          |
| `idp_sync_settings_organization` |
| `idp_sync_settings_group`        |
| `idp_sync_settings_role`         |
| `workspace_agent`                |
| `workspace_app`                  |

## codersdk.Response

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

| Name          | Type                                                          | Required | Restrictions | Description                                                                                                                                                                                                                        |
|---------------|---------------------------------------------------------------|----------|--------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `detail`      | string                                                        | false    |              | Detail is a debug message that provides further insight into why the action failed. This information can be technical and a regular golang err.Error() text. - "database: too many open connections" - "stat: too many open files" |
| `message`     | string                                                        | false    |              | Message is an actionable message that depicts actions the request took. These messages should be fully formed sentences with proper punctuation. Examples: - "A user has been created." - "Failed to create a user."               |
| `validations` | array of [codersdk.ValidationError](#codersdkvalidationerror) | false    |              | Validations are form field-specific friendly error messages. They will be shown on a form field in the UI. These can also be used to add additional context if there is a set of errors in the primary 'Message'.                  |

## codersdk.Role

```json
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
```

### Properties

| Name                       | Type                                                | Required | Restrictions | Description                                                                                     |
|----------------------------|-----------------------------------------------------|----------|--------------|-------------------------------------------------------------------------------------------------|
| `display_name`             | string                                              | false    |              |                                                                                                 |
| `name`                     | string                                              | false    |              |                                                                                                 |
| `organization_id`          | string                                              | false    |              |                                                                                                 |
| `organization_permissions` | array of [codersdk.Permission](#codersdkpermission) | false    |              | Organization permissions are specific for the organization in the field 'OrganizationID' above. |
| `site_permissions`         | array of [codersdk.Permission](#codersdkpermission) | false    |              |                                                                                                 |
| `user_permissions`         | array of [codersdk.Permission](#codersdkpermission) | false    |              |                                                                                                 |

## codersdk.RoleSyncSettings

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

### Properties

| Name               | Type            | Required | Restrictions | Description                                                                                                                            |
|--------------------|-----------------|----------|--------------|----------------------------------------------------------------------------------------------------------------------------------------|
| `field`            | string          | false    |              | Field is the name of the claim field that specifies what organization roles a user should be given. If empty, no roles will be synced. |
| `mapping`          | object          | false    |              | Mapping is a map from OIDC groups to Coder organization roles.                                                                         |
| » `[any property]` | array of string | false    |              |                                                                                                                                        |

## codersdk.SSHConfig

```json
{
  "deploymentName": "string",
  "sshconfigOptions": [
    "string"
  ]
}
```

### Properties

| Name               | Type            | Required | Restrictions | Description                                                                                         |
|--------------------|-----------------|----------|--------------|-----------------------------------------------------------------------------------------------------|
| `deploymentName`   | string          | false    |              | Deploymentname is the config-ssh Hostname prefix                                                    |
| `sshconfigOptions` | array of string | false    |              | Sshconfigoptions are additional options to add to the ssh config file. This will override defaults. |

## codersdk.SSHConfigResponse

```json
{
  "hostname_prefix": "string",
  "ssh_config_options": {
    "property1": "string",
    "property2": "string"
  }
}
```

### Properties

| Name                 | Type   | Required | Restrictions | Description |
|----------------------|--------|----------|--------------|-------------|
| `hostname_prefix`    | string | false    |              |             |
| `ssh_config_options` | object | false    |              |             |
| » `[any property]`   | string | false    |              |             |

## codersdk.ServerSentEvent

```json
{
  "data": null,
  "type": "ping"
}
```

### Properties

| Name   | Type                                                         | Required | Restrictions | Description |
|--------|--------------------------------------------------------------|----------|--------------|-------------|
| `data` | any                                                          | false    |              |             |
| `type` | [codersdk.ServerSentEventType](#codersdkserversenteventtype) | false    |              |             |

## codersdk.ServerSentEventType

```json
"ping"
```

### Properties

#### Enumerated Values

| Value   |
|---------|
| `ping`  |
| `data`  |
| `error` |

## codersdk.SessionCountDeploymentStats

```json
{
  "jetbrains": 0,
  "reconnecting_pty": 0,
  "ssh": 0,
  "vscode": 0
}
```

### Properties

| Name               | Type    | Required | Restrictions | Description |
|--------------------|---------|----------|--------------|-------------|
| `jetbrains`        | integer | false    |              |             |
| `reconnecting_pty` | integer | false    |              |             |
| `ssh`              | integer | false    |              |             |
| `vscode`           | integer | false    |              |             |

## codersdk.SessionLifetime

```json
{
  "default_duration": 0,
  "default_token_lifetime": 0,
  "disable_expiry_refresh": true,
  "max_token_lifetime": 0
}
```

### Properties

| Name                     | Type    | Required | Restrictions | Description                                                                                                                                                                        |
|--------------------------|---------|----------|--------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `default_duration`       | integer | false    |              | Default duration is only for browser, workspace app and oauth sessions.                                                                                                            |
| `default_token_lifetime` | integer | false    |              |                                                                                                                                                                                    |
| `disable_expiry_refresh` | boolean | false    |              | Disable expiry refresh will disable automatically refreshing api keys when they are used from the api. This means the api key lifetime at creation is the lifetime of the api key. |
| `max_token_lifetime`     | integer | false    |              |                                                                                                                                                                                    |

## codersdk.SlimRole

```json
{
  "display_name": "string",
  "name": "string",
  "organization_id": "string"
}
```

### Properties

| Name              | Type   | Required | Restrictions | Description |
|-------------------|--------|----------|--------------|-------------|
| `display_name`    | string | false    |              |             |
| `name`            | string | false    |              |             |
| `organization_id` | string | false    |              |             |

## codersdk.SupportConfig

```json
{
  "links": {
    "value": [
      {
        "icon": "bug",
        "name": "string",
        "target": "string"
      }
    ]
  }
}
```

### Properties

| Name    | Type                                                                                 | Required | Restrictions | Description |
|---------|--------------------------------------------------------------------------------------|----------|--------------|-------------|
| `links` | [serpent.Struct-array_codersdk_LinkConfig](#serpentstruct-array_codersdk_linkconfig) | false    |              |             |

## codersdk.SwaggerConfig

```json
{
  "enable": true
}
```

### Properties

| Name     | Type    | Required | Restrictions | Description |
|----------|---------|----------|--------------|-------------|
| `enable` | boolean | false    |              |             |

## codersdk.TLSConfig

```json
{
  "address": {
    "host": "string",
    "port": "string"
  },
  "allow_insecure_ciphers": true,
  "cert_file": [
    "string"
  ],
  "client_auth": "string",
  "client_ca_file": "string",
  "client_cert_file": "string",
  "client_key_file": "string",
  "enable": true,
  "key_file": [
    "string"
  ],
  "min_version": "string",
  "redirect_http": true,
  "supported_ciphers": [
    "string"
  ]
}
```

### Properties

| Name                     | Type                                 | Required | Restrictions | Description |
|--------------------------|--------------------------------------|----------|--------------|-------------|
| `address`                | [serpent.HostPort](#serpenthostport) | false    |              |             |
| `allow_insecure_ciphers` | boolean                              | false    |              |             |
| `cert_file`              | array of string                      | false    |              |             |
| `client_auth`            | string                               | false    |              |             |
| `client_ca_file`         | string                               | false    |              |             |
| `client_cert_file`       | string                               | false    |              |             |
| `client_key_file`        | string                               | false    |              |             |
| `enable`                 | boolean                              | false    |              |             |
| `key_file`               | array of string                      | false    |              |             |
| `min_version`            | string                               | false    |              |             |
| `redirect_http`          | boolean                              | false    |              |             |
| `supported_ciphers`      | array of string                      | false    |              |             |

## codersdk.TelemetryConfig

```json
{
  "enable": true,
  "trace": true,
  "url": {
    "forceQuery": true,
    "fragment": "string",
    "host": "string",
    "omitHost": true,
    "opaque": "string",
    "path": "string",
    "rawFragment": "string",
    "rawPath": "string",
    "rawQuery": "string",
    "scheme": "string",
    "user": {}
  }
}
```

### Properties

| Name     | Type                       | Required | Restrictions | Description |
|----------|----------------------------|----------|--------------|-------------|
| `enable` | boolean                    | false    |              |             |
| `trace`  | boolean                    | false    |              |             |
| `url`    | [serpent.URL](#serpenturl) | false    |              |             |

## codersdk.Template

```json
{
  "active_user_count": 0,
  "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
  "activity_bump_ms": 0,
  "allow_user_autostart": true,
  "allow_user_autostop": true,
  "allow_user_cancel_workspace_jobs": true,
  "autostart_requirement": {
    "days_of_week": [
      "monday"
    ]
  },
  "autostop_requirement": {
    "days_of_week": [
      "monday"
    ],
    "weeks": 0
  },
  "build_time_stats": {
    "property1": {
      "p50": 123,
      "p95": 146
    },
    "property2": {
      "p50": 123,
      "p95": 146
    }
  },
  "created_at": "2019-08-24T14:15:22Z",
  "created_by_id": "9377d689-01fb-4abf-8450-3368d2c1924f",
  "created_by_name": "string",
  "default_ttl_ms": 0,
  "deprecated": true,
  "deprecation_message": "string",
  "description": "string",
  "display_name": "string",
  "failure_ttl_ms": 0,
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "max_port_share_level": "owner",
  "name": "string",
  "organization_display_name": "string",
  "organization_icon": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "organization_name": "string",
  "provisioner": "terraform",
  "require_active_version": true,
  "time_til_dormant_autodelete_ms": 0,
  "time_til_dormant_ms": 0,
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Properties

| Name                               | Type                                                                           | Required | Restrictions | Description                                                                                                                                                                                     |
|------------------------------------|--------------------------------------------------------------------------------|----------|--------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `active_user_count`                | integer                                                                        | false    |              | Active user count is set to -1 when loading.                                                                                                                                                    |
| `active_version_id`                | string                                                                         | false    |              |                                                                                                                                                                                                 |
| `activity_bump_ms`                 | integer                                                                        | false    |              |                                                                                                                                                                                                 |
| `allow_user_autostart`             | boolean                                                                        | false    |              | Allow user autostart and AllowUserAutostop are enterprise-only. Their values are only used if your license is entitled to use the advanced template scheduling feature.                         |
| `allow_user_autostop`              | boolean                                                                        | false    |              |                                                                                                                                                                                                 |
| `allow_user_cancel_workspace_jobs` | boolean                                                                        | false    |              |                                                                                                                                                                                                 |
| `autostart_requirement`            | [codersdk.TemplateAutostartRequirement](#codersdktemplateautostartrequirement) | false    |              |                                                                                                                                                                                                 |
| `autostop_requirement`             | [codersdk.TemplateAutostopRequirement](#codersdktemplateautostoprequirement)   | false    |              | Autostop requirement and AutostartRequirement are enterprise features. Its value is only used if your license is entitled to use the advanced template scheduling feature.                      |
| `build_time_stats`                 | [codersdk.TemplateBuildTimeStats](#codersdktemplatebuildtimestats)             | false    |              |                                                                                                                                                                                                 |
| `created_at`                       | string                                                                         | false    |              |                                                                                                                                                                                                 |
| `created_by_id`                    | string                                                                         | false    |              |                                                                                                                                                                                                 |
| `created_by_name`                  | string                                                                         | false    |              |                                                                                                                                                                                                 |
| `default_ttl_ms`                   | integer                                                                        | false    |              |                                                                                                                                                                                                 |
| `deprecated`                       | boolean                                                                        | false    |              |                                                                                                                                                                                                 |
| `deprecation_message`              | string                                                                         | false    |              |                                                                                                                                                                                                 |
| `description`                      | string                                                                         | false    |              |                                                                                                                                                                                                 |
| `display_name`                     | string                                                                         | false    |              |                                                                                                                                                                                                 |
| `failure_ttl_ms`                   | integer                                                                        | false    |              | Failure ttl ms TimeTilDormantMillis, and TimeTilDormantAutoDeleteMillis are enterprise-only. Their values are used if your license is entitled to use the advanced template scheduling feature. |
| `icon`                             | string                                                                         | false    |              |                                                                                                                                                                                                 |
| `id`                               | string                                                                         | false    |              |                                                                                                                                                                                                 |
| `max_port_share_level`             | [codersdk.WorkspaceAgentPortShareLevel](#codersdkworkspaceagentportsharelevel) | false    |              |                                                                                                                                                                                                 |
| `name`                             | string                                                                         | false    |              |                                                                                                                                                                                                 |
| `organization_display_name`        | string                                                                         | false    |              |                                                                                                                                                                                                 |
| `organization_icon`                | string                                                                         | false    |              |                                                                                                                                                                                                 |
| `organization_id`                  | string                                                                         | false    |              |                                                                                                                                                                                                 |
| `organization_name`                | string                                                                         | false    |              |                                                                                                                                                                                                 |
| `provisioner`                      | string                                                                         | false    |              |                                                                                                                                                                                                 |
| `require_active_version`           | boolean                                                                        | false    |              | Require active version mandates that workspaces are built with the active template version.                                                                                                     |
| `time_til_dormant_autodelete_ms`   | integer                                                                        | false    |              |                                                                                                                                                                                                 |
| `time_til_dormant_ms`              | integer                                                                        | false    |              |                                                                                                                                                                                                 |
| `updated_at`                       | string                                                                         | false    |              |                                                                                                                                                                                                 |

#### Enumerated Values

| Property      | Value       |
|---------------|-------------|
| `provisioner` | `terraform` |

## codersdk.TemplateAppUsage

```json
{
  "display_name": "Visual Studio Code",
  "icon": "string",
  "seconds": 80500,
  "slug": "vscode",
  "template_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "times_used": 2,
  "type": "builtin"
}
```

### Properties

| Name           | Type                                                   | Required | Restrictions | Description |
|----------------|--------------------------------------------------------|----------|--------------|-------------|
| `display_name` | string                                                 | false    |              |             |
| `icon`         | string                                                 | false    |              |             |
| `seconds`      | integer                                                | false    |              |             |
| `slug`         | string                                                 | false    |              |             |
| `template_ids` | array of string                                        | false    |              |             |
| `times_used`   | integer                                                | false    |              |             |
| `type`         | [codersdk.TemplateAppsType](#codersdktemplateappstype) | false    |              |             |

## codersdk.TemplateAppsType

```json
"builtin"
```

### Properties

#### Enumerated Values

| Value     |
|-----------|
| `builtin` |
| `app`     |

## codersdk.TemplateAutostartRequirement

```json
{
  "days_of_week": [
    "monday"
  ]
}
```

### Properties

| Name           | Type            | Required | Restrictions | Description                                                                                                                             |
|----------------|-----------------|----------|--------------|-----------------------------------------------------------------------------------------------------------------------------------------|
| `days_of_week` | array of string | false    |              | Days of week is a list of days of the week in which autostart is allowed to happen. If no days are specified, autostart is not allowed. |

## codersdk.TemplateAutostopRequirement

```json
{
  "days_of_week": [
    "monday"
  ],
  "weeks": 0
}
```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|`days_of_week`|array of string|false||Days of week is a list of days of the week on which restarts are required. Restarts happen within the user's quiet hours (in their configured timezone). If no days are specified, restarts are not required. Weekdays cannot be specified twice.
Restarts will only happen on weekdays in this list on weeks which line up with Weeks.|
|`weeks`|integer|false||Weeks is the number of weeks between required restarts. Weeks are synced across all workspaces (and Coder deployments) using modulo math on a hardcoded epoch week of January 2nd, 2023 (the first Monday of 2023). Values of 0 or 1 indicate weekly restarts. Values of 2 indicate fortnightly restarts, etc.|

## codersdk.TemplateBuildTimeStats

```json
{
  "property1": {
    "p50": 123,
    "p95": 146
  },
  "property2": {
    "p50": 123,
    "p95": 146
  }
}
```

### Properties

| Name             | Type                                                 | Required | Restrictions | Description |
|------------------|------------------------------------------------------|----------|--------------|-------------|
| `[any property]` | [codersdk.TransitionStats](#codersdktransitionstats) | false    |              |             |

## codersdk.TemplateExample

```json
{
  "description": "string",
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "markdown": "string",
  "name": "string",
  "tags": [
    "string"
  ],
  "url": "string"
}
```

### Properties

| Name          | Type            | Required | Restrictions | Description |
|---------------|-----------------|----------|--------------|-------------|
| `description` | string          | false    |              |             |
| `icon`        | string          | false    |              |             |
| `id`          | string          | false    |              |             |
| `markdown`    | string          | false    |              |             |
| `name`        | string          | false    |              |             |
| `tags`        | array of string | false    |              |             |
| `url`         | string          | false    |              |             |

## codersdk.TemplateInsightsIntervalReport

```json
{
  "active_users": 14,
  "end_time": "2019-08-24T14:15:22Z",
  "interval": "week",
  "start_time": "2019-08-24T14:15:22Z",
  "template_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ]
}
```

### Properties

| Name           | Type                                                               | Required | Restrictions | Description |
|----------------|--------------------------------------------------------------------|----------|--------------|-------------|
| `active_users` | integer                                                            | false    |              |             |
| `end_time`     | string                                                             | false    |              |             |
| `interval`     | [codersdk.InsightsReportInterval](#codersdkinsightsreportinterval) | false    |              |             |
| `start_time`   | string                                                             | false    |              |             |
| `template_ids` | array of string                                                    | false    |              |             |

## codersdk.TemplateInsightsReport

```json
{
  "active_users": 22,
  "apps_usage": [
    {
      "display_name": "Visual Studio Code",
      "icon": "string",
      "seconds": 80500,
      "slug": "vscode",
      "template_ids": [
        "497f6eca-6276-4993-bfeb-53cbbbba6f08"
      ],
      "times_used": 2,
      "type": "builtin"
    }
  ],
  "end_time": "2019-08-24T14:15:22Z",
  "parameters_usage": [
    {
      "description": "string",
      "display_name": "string",
      "name": "string",
      "options": [
        {
          "description": "string",
          "icon": "string",
          "name": "string",
          "value": "string"
        }
      ],
      "template_ids": [
        "497f6eca-6276-4993-bfeb-53cbbbba6f08"
      ],
      "type": "string",
      "values": [
        {
          "count": 0,
          "value": "string"
        }
      ]
    }
  ],
  "start_time": "2019-08-24T14:15:22Z",
  "template_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ]
}
```

### Properties

| Name               | Type                                                                        | Required | Restrictions | Description |
|--------------------|-----------------------------------------------------------------------------|----------|--------------|-------------|
| `active_users`     | integer                                                                     | false    |              |             |
| `apps_usage`       | array of [codersdk.TemplateAppUsage](#codersdktemplateappusage)             | false    |              |             |
| `end_time`         | string                                                                      | false    |              |             |
| `parameters_usage` | array of [codersdk.TemplateParameterUsage](#codersdktemplateparameterusage) | false    |              |             |
| `start_time`       | string                                                                      | false    |              |             |
| `template_ids`     | array of string                                                             | false    |              |             |

## codersdk.TemplateInsightsResponse

```json
{
  "interval_reports": [
    {
      "active_users": 14,
      "end_time": "2019-08-24T14:15:22Z",
      "interval": "week",
      "start_time": "2019-08-24T14:15:22Z",
      "template_ids": [
        "497f6eca-6276-4993-bfeb-53cbbbba6f08"
      ]
    }
  ],
  "report": {
    "active_users": 22,
    "apps_usage": [
      {
        "display_name": "Visual Studio Code",
        "icon": "string",
        "seconds": 80500,
        "slug": "vscode",
        "template_ids": [
          "497f6eca-6276-4993-bfeb-53cbbbba6f08"
        ],
        "times_used": 2,
        "type": "builtin"
      }
    ],
    "end_time": "2019-08-24T14:15:22Z",
    "parameters_usage": [
      {
        "description": "string",
        "display_name": "string",
        "name": "string",
        "options": [
          {
            "description": "string",
            "icon": "string",
            "name": "string",
            "value": "string"
          }
        ],
        "template_ids": [
          "497f6eca-6276-4993-bfeb-53cbbbba6f08"
        ],
        "type": "string",
        "values": [
          {
            "count": 0,
            "value": "string"
          }
        ]
      }
    ],
    "start_time": "2019-08-24T14:15:22Z",
    "template_ids": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ]
  }
}
```

### Properties

| Name               | Type                                                                                        | Required | Restrictions | Description |
|--------------------|---------------------------------------------------------------------------------------------|----------|--------------|-------------|
| `interval_reports` | array of [codersdk.TemplateInsightsIntervalReport](#codersdktemplateinsightsintervalreport) | false    |              |             |
| `report`           | [codersdk.TemplateInsightsReport](#codersdktemplateinsightsreport)                          | false    |              |             |

## codersdk.TemplateParameterUsage

```json
{
  "description": "string",
  "display_name": "string",
  "name": "string",
  "options": [
    {
      "description": "string",
      "icon": "string",
      "name": "string",
      "value": "string"
    }
  ],
  "template_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "type": "string",
  "values": [
    {
      "count": 0,
      "value": "string"
    }
  ]
}
```

### Properties

| Name           | Type                                                                                        | Required | Restrictions | Description |
|----------------|---------------------------------------------------------------------------------------------|----------|--------------|-------------|
| `description`  | string                                                                                      | false    |              |             |
| `display_name` | string                                                                                      | false    |              |             |
| `name`         | string                                                                                      | false    |              |             |
| `options`      | array of [codersdk.TemplateVersionParameterOption](#codersdktemplateversionparameteroption) | false    |              |             |
| `template_ids` | array of string                                                                             | false    |              |             |
| `type`         | string                                                                                      | false    |              |             |
| `values`       | array of [codersdk.TemplateParameterValue](#codersdktemplateparametervalue)                 | false    |              |             |

## codersdk.TemplateParameterValue

```json
{
  "count": 0,
  "value": "string"
}
```

### Properties

| Name    | Type    | Required | Restrictions | Description |
|---------|---------|----------|--------------|-------------|
| `count` | integer | false    |              |             |
| `value` | string  | false    |              |             |

## codersdk.TemplateRole

```json
"admin"
```

### Properties

#### Enumerated Values

| Value   |
|---------|
| `admin` |
| `use`   |
| ``      |

## codersdk.TemplateUser

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
```

### Properties

| Name               | Type                                            | Required | Restrictions | Description                                                                                |
|--------------------|-------------------------------------------------|----------|--------------|--------------------------------------------------------------------------------------------|
| `avatar_url`       | string                                          | false    |              |                                                                                            |
| `created_at`       | string                                          | true     |              |                                                                                            |
| `email`            | string                                          | true     |              |                                                                                            |
| `id`               | string                                          | true     |              |                                                                                            |
| `last_seen_at`     | string                                          | false    |              |                                                                                            |
| `login_type`       | [codersdk.LoginType](#codersdklogintype)        | false    |              |                                                                                            |
| `name`             | string                                          | false    |              |                                                                                            |
| `organization_ids` | array of string                                 | false    |              |                                                                                            |
| `role`             | [codersdk.TemplateRole](#codersdktemplaterole)  | false    |              |                                                                                            |
| `roles`            | array of [codersdk.SlimRole](#codersdkslimrole) | false    |              |                                                                                            |
| `status`           | [codersdk.UserStatus](#codersdkuserstatus)      | false    |              |                                                                                            |
| `theme_preference` | string                                          | false    |              | Deprecated: this value should be retrieved from `codersdk.UserPreferenceSettings` instead. |
| `updated_at`       | string                                          | false    |              |                                                                                            |
| `username`         | string                                          | true     |              |                                                                                            |

#### Enumerated Values

| Property | Value       |
|----------|-------------|
| `role`   | `admin`     |
| `role`   | `use`       |
| `status` | `active`    |
| `status` | `suspended` |

## codersdk.TemplateVersion

```json
{
  "archived": true,
  "created_at": "2019-08-24T14:15:22Z",
  "created_by": {
    "avatar_url": "http://example.com",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "username": "string"
  },
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "job": {
    "available_workers": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "canceled_at": "2019-08-24T14:15:22Z",
    "completed_at": "2019-08-24T14:15:22Z",
    "created_at": "2019-08-24T14:15:22Z",
    "error": "string",
    "error_code": "REQUIRED_TEMPLATE_VARIABLES",
    "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "input": {
      "error": "string",
      "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
      "workspace_build_id": "badaf2eb-96c5-4050-9f1d-db2d39ca5478"
    },
    "metadata": {
      "template_display_name": "string",
      "template_icon": "string",
      "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
      "template_name": "string",
      "template_version_name": "string",
      "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
      "workspace_name": "string"
    },
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "queue_position": 0,
    "queue_size": 0,
    "started_at": "2019-08-24T14:15:22Z",
    "status": "pending",
    "tags": {
      "property1": "string",
      "property2": "string"
    },
    "type": "template_version_import",
    "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
  },
  "matched_provisioners": {
    "available": 0,
    "count": 0,
    "most_recently_seen": "2019-08-24T14:15:22Z"
  },
  "message": "string",
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "readme": "string",
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "updated_at": "2019-08-24T14:15:22Z",
  "warnings": [
    "UNSUPPORTED_WORKSPACES"
  ]
}
```

### Properties

| Name                   | Type                                                                        | Required | Restrictions | Description |
|------------------------|-----------------------------------------------------------------------------|----------|--------------|-------------|
| `archived`             | boolean                                                                     | false    |              |             |
| `created_at`           | string                                                                      | false    |              |             |
| `created_by`           | [codersdk.MinimalUser](#codersdkminimaluser)                                | false    |              |             |
| `id`                   | string                                                                      | false    |              |             |
| `job`                  | [codersdk.ProvisionerJob](#codersdkprovisionerjob)                          | false    |              |             |
| `matched_provisioners` | [codersdk.MatchedProvisioners](#codersdkmatchedprovisioners)                | false    |              |             |
| `message`              | string                                                                      | false    |              |             |
| `name`                 | string                                                                      | false    |              |             |
| `organization_id`      | string                                                                      | false    |              |             |
| `readme`               | string                                                                      | false    |              |             |
| `template_id`          | string                                                                      | false    |              |             |
| `updated_at`           | string                                                                      | false    |              |             |
| `warnings`             | array of [codersdk.TemplateVersionWarning](#codersdktemplateversionwarning) | false    |              |             |

## codersdk.TemplateVersionExternalAuth

```json
{
  "authenticate_url": "string",
  "authenticated": true,
  "display_icon": "string",
  "display_name": "string",
  "id": "string",
  "optional": true,
  "type": "string"
}
```

### Properties

| Name               | Type    | Required | Restrictions | Description |
|--------------------|---------|----------|--------------|-------------|
| `authenticate_url` | string  | false    |              |             |
| `authenticated`    | boolean | false    |              |             |
| `display_icon`     | string  | false    |              |             |
| `display_name`     | string  | false    |              |             |
| `id`               | string  | false    |              |             |
| `optional`         | boolean | false    |              |             |
| `type`             | string  | false    |              |             |

## codersdk.TemplateVersionParameter

```json
{
  "default_value": "string",
  "description": "string",
  "description_plaintext": "string",
  "display_name": "string",
  "ephemeral": true,
  "icon": "string",
  "mutable": true,
  "name": "string",
  "options": [
    {
      "description": "string",
      "icon": "string",
      "name": "string",
      "value": "string"
    }
  ],
  "required": true,
  "type": "string",
  "validation_error": "string",
  "validation_max": 0,
  "validation_min": 0,
  "validation_monotonic": "increasing",
  "validation_regex": "string"
}
```

### Properties

| Name                    | Type                                                                                        | Required | Restrictions | Description |
|-------------------------|---------------------------------------------------------------------------------------------|----------|--------------|-------------|
| `default_value`         | string                                                                                      | false    |              |             |
| `description`           | string                                                                                      | false    |              |             |
| `description_plaintext` | string                                                                                      | false    |              |             |
| `display_name`          | string                                                                                      | false    |              |             |
| `ephemeral`             | boolean                                                                                     | false    |              |             |
| `icon`                  | string                                                                                      | false    |              |             |
| `mutable`               | boolean                                                                                     | false    |              |             |
| `name`                  | string                                                                                      | false    |              |             |
| `options`               | array of [codersdk.TemplateVersionParameterOption](#codersdktemplateversionparameteroption) | false    |              |             |
| `required`              | boolean                                                                                     | false    |              |             |
| `type`                  | string                                                                                      | false    |              |             |
| `validation_error`      | string                                                                                      | false    |              |             |
| `validation_max`        | integer                                                                                     | false    |              |             |
| `validation_min`        | integer                                                                                     | false    |              |             |
| `validation_monotonic`  | [codersdk.ValidationMonotonicOrder](#codersdkvalidationmonotonicorder)                      | false    |              |             |
| `validation_regex`      | string                                                                                      | false    |              |             |

#### Enumerated Values

| Property               | Value          |
|------------------------|----------------|
| `type`                 | `string`       |
| `type`                 | `number`       |
| `type`                 | `bool`         |
| `type`                 | `list(string)` |
| `validation_monotonic` | `increasing`   |
| `validation_monotonic` | `decreasing`   |

## codersdk.TemplateVersionParameterOption

```json
{
  "description": "string",
  "icon": "string",
  "name": "string",
  "value": "string"
}
```

### Properties

| Name          | Type   | Required | Restrictions | Description |
|---------------|--------|----------|--------------|-------------|
| `description` | string | false    |              |             |
| `icon`        | string | false    |              |             |
| `name`        | string | false    |              |             |
| `value`       | string | false    |              |             |

## codersdk.TemplateVersionVariable

```json
{
  "default_value": "string",
  "description": "string",
  "name": "string",
  "required": true,
  "sensitive": true,
  "type": "string",
  "value": "string"
}
```

### Properties

| Name            | Type    | Required | Restrictions | Description |
|-----------------|---------|----------|--------------|-------------|
| `default_value` | string  | false    |              |             |
| `description`   | string  | false    |              |             |
| `name`          | string  | false    |              |             |
| `required`      | boolean | false    |              |             |
| `sensitive`     | boolean | false    |              |             |
| `type`          | string  | false    |              |             |
| `value`         | string  | false    |              |             |

#### Enumerated Values

| Property | Value    |
|----------|----------|
| `type`   | `string` |
| `type`   | `number` |
| `type`   | `bool`   |

## codersdk.TemplateVersionWarning

```json
"UNSUPPORTED_WORKSPACES"
```

### Properties

#### Enumerated Values

| Value                    |
|--------------------------|
| `UNSUPPORTED_WORKSPACES` |

## codersdk.TerminalFontName

```json
""
```

### Properties

#### Enumerated Values

| Value           |
|-----------------|
| ``              |
| `ibm-mono-plex` |
| `fira-code`     |

## codersdk.TimingStage

```json
"init"
```

### Properties

#### Enumerated Values

| Value     |
|-----------|
| `init`    |
| `plan`    |
| `graph`   |
| `apply`   |
| `start`   |
| `stop`    |
| `cron`    |
| `connect` |

## codersdk.TokenConfig

```json
{
  "max_token_lifetime": 0
}
```

### Properties

| Name                 | Type    | Required | Restrictions | Description |
|----------------------|---------|----------|--------------|-------------|
| `max_token_lifetime` | integer | false    |              |             |

## codersdk.TraceConfig

```json
{
  "capture_logs": true,
  "data_dog": true,
  "enable": true,
  "honeycomb_api_key": "string"
}
```

### Properties

| Name                | Type    | Required | Restrictions | Description |
|---------------------|---------|----------|--------------|-------------|
| `capture_logs`      | boolean | false    |              |             |
| `data_dog`          | boolean | false    |              |             |
| `enable`            | boolean | false    |              |             |
| `honeycomb_api_key` | string  | false    |              |             |

## codersdk.TransitionStats

```json
{
  "p50": 123,
  "p95": 146
}
```

### Properties

| Name  | Type    | Required | Restrictions | Description |
|-------|---------|----------|--------------|-------------|
| `p50` | integer | false    |              |             |
| `p95` | integer | false    |              |             |

## codersdk.UpdateActiveTemplateVersion

```json
{
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08"
}
```

### Properties

| Name | Type   | Required | Restrictions | Description |
|------|--------|----------|--------------|-------------|
| `id` | string | true     |              |             |

## codersdk.UpdateAppearanceConfig

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

### Properties

| Name                   | Type                                                    | Required | Restrictions | Description                                                         |
|------------------------|---------------------------------------------------------|----------|--------------|---------------------------------------------------------------------|
| `announcement_banners` | array of [codersdk.BannerConfig](#codersdkbannerconfig) | false    |              |                                                                     |
| `application_name`     | string                                                  | false    |              |                                                                     |
| `logo_url`             | string                                                  | false    |              |                                                                     |
| `service_banner`       | [codersdk.BannerConfig](#codersdkbannerconfig)          | false    |              | Deprecated: ServiceBanner has been replaced by AnnouncementBanners. |

## codersdk.UpdateCheckResponse

```json
{
  "current": true,
  "url": "string",
  "version": "string"
}
```

### Properties

| Name      | Type    | Required | Restrictions | Description                                                             |
|-----------|---------|----------|--------------|-------------------------------------------------------------------------|
| `current` | boolean | false    |              | Current indicates whether the server version is the same as the latest. |
| `url`     | string  | false    |              | URL to download the latest release of Coder.                            |
| `version` | string  | false    |              | Version is the semantic version for the latest release of Coder.        |

## codersdk.UpdateOrganizationRequest

```json
{
  "description": "string",
  "display_name": "string",
  "icon": "string",
  "name": "string"
}
```

### Properties

| Name           | Type   | Required | Restrictions | Description |
|----------------|--------|----------|--------------|-------------|
| `description`  | string | false    |              |             |
| `display_name` | string | false    |              |             |
| `icon`         | string | false    |              |             |
| `name`         | string | false    |              |             |

## codersdk.UpdateRoles

```json
{
  "roles": [
    "string"
  ]
}
```

### Properties

| Name    | Type            | Required | Restrictions | Description |
|---------|-----------------|----------|--------------|-------------|
| `roles` | array of string | false    |              |             |

## codersdk.UpdateTemplateACL

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

### Properties

| Name               | Type                                           | Required | Restrictions | Description                                                                                                                   |
|--------------------|------------------------------------------------|----------|--------------|-------------------------------------------------------------------------------------------------------------------------------|
| `group_perms`      | object                                         | false    |              | Group perms should be a mapping of group ID to role.                                                                          |
| » `[any property]` | [codersdk.TemplateRole](#codersdktemplaterole) | false    |              |                                                                                                                               |
| `user_perms`       | object                                         | false    |              | User perms should be a mapping of user ID to role. The user ID must be the uuid of the user, not a username or email address. |
| » `[any property]` | [codersdk.TemplateRole](#codersdktemplaterole) | false    |              |                                                                                                                               |

## codersdk.UpdateUserAppearanceSettingsRequest

```json
{
  "terminal_font": "ibm-mono-plex",
  "theme_preference": "string"
}
```

### Properties

| Name               | Type                                                   | Required | Restrictions | Description |
|--------------------|--------------------------------------------------------|----------|--------------|-------------|
| `terminal_font`    | [codersdk.TerminalFontName](#codersdkterminalfontname) | true     |              |             |
| `theme_preference` | string                                                 | true     |              |             |

#### Enumerated Values

| Property        | Value           |
|-----------------|-----------------|
| `terminal_font` | `ibm-mono-plex` |
| `terminal_font` | `fira-code`     |

## codersdk.UpdateUserNotificationPreferences

```json
{
  "template_disabled_map": {
    "property1": true,
    "property2": true
  }
}
```

### Properties

| Name                    | Type    | Required | Restrictions | Description |
|-------------------------|---------|----------|--------------|-------------|
| `template_disabled_map` | object  | false    |              |             |
| » `[any property]`      | boolean | false    |              |             |

## codersdk.UpdateUserPasswordRequest

```json
{
  "old_password": "string",
  "password": "string"
}
```

### Properties

| Name           | Type   | Required | Restrictions | Description |
|----------------|--------|----------|--------------|-------------|
| `old_password` | string | false    |              |             |
| `password`     | string | true     |              |             |

## codersdk.UpdateUserProfileRequest

```json
{
  "name": "string",
  "username": "string"
}
```

### Properties

| Name       | Type   | Required | Restrictions | Description |
|------------|--------|----------|--------------|-------------|
| `name`     | string | false    |              |             |
| `username` | string | true     |              |             |

## codersdk.UpdateUserQuietHoursScheduleRequest

```json
{
  "schedule": "string"
}
```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|`schedule`|string|true||Schedule is a cron expression that defines when the user's quiet hours window is. Schedule must not be empty. For new users, the schedule is set to 2am in their browser or computer's timezone. The schedule denotes the beginning of a 4 hour window where the workspace is allowed to automatically stop or restart due to maintenance or template schedule.
The schedule must be daily with a single time, and should have a timezone specified via a CRON_TZ prefix (otherwise UTC will be used).
If the schedule is empty, the user will be updated to use the default schedule.|

## codersdk.UpdateWorkspaceAutomaticUpdatesRequest

```json
{
  "automatic_updates": "always"
}
```

### Properties

| Name                | Type                                                   | Required | Restrictions | Description |
|---------------------|--------------------------------------------------------|----------|--------------|-------------|
| `automatic_updates` | [codersdk.AutomaticUpdates](#codersdkautomaticupdates) | false    |              |             |

## codersdk.UpdateWorkspaceAutostartRequest

```json
{
  "schedule": "string"
}
```

### Properties

| Name       | Type   | Required | Restrictions | Description                                                                                                                                                                                                                                    |
|------------|--------|----------|--------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `schedule` | string | false    |              | Schedule is expected to be of the form `CRON_TZ=<IANA Timezone> <min> <hour> * * <dow>` Example: `CRON_TZ=US/Central 30 9 * * 1-5` represents 0930 in the timezone US/Central on weekdays (Mon-Fri). `CRON_TZ` defaults to UTC if not present. |

## codersdk.UpdateWorkspaceDormancy

```json
{
  "dormant": true
}
```

### Properties

| Name      | Type    | Required | Restrictions | Description |
|-----------|---------|----------|--------------|-------------|
| `dormant` | boolean | false    |              |             |

## codersdk.UpdateWorkspaceRequest

```json
{
  "name": "string"
}
```

### Properties

| Name   | Type   | Required | Restrictions | Description |
|--------|--------|----------|--------------|-------------|
| `name` | string | false    |              |             |

## codersdk.UpdateWorkspaceTTLRequest

```json
{
  "ttl_ms": 0
}
```

### Properties

| Name     | Type    | Required | Restrictions | Description |
|----------|---------|----------|--------------|-------------|
| `ttl_ms` | integer | false    |              |             |

## codersdk.UploadResponse

```json
{
  "hash": "19686d84-b10d-4f90-b18e-84fd3fa038fd"
}
```

### Properties

| Name   | Type   | Required | Restrictions | Description |
|--------|--------|----------|--------------|-------------|
| `hash` | string | false    |              |             |

## codersdk.UpsertWorkspaceAgentPortShareRequest

```json
{
  "agent_name": "string",
  "port": 0,
  "protocol": "http",
  "share_level": "owner"
}
```

### Properties

| Name          | Type                                                                                 | Required | Restrictions | Description |
|---------------|--------------------------------------------------------------------------------------|----------|--------------|-------------|
| `agent_name`  | string                                                                               | false    |              |             |
| `port`        | integer                                                                              | false    |              |             |
| `protocol`    | [codersdk.WorkspaceAgentPortShareProtocol](#codersdkworkspaceagentportshareprotocol) | false    |              |             |
| `share_level` | [codersdk.WorkspaceAgentPortShareLevel](#codersdkworkspaceagentportsharelevel)       | false    |              |             |

#### Enumerated Values

| Property      | Value           |
|---------------|-----------------|
| `protocol`    | `http`          |
| `protocol`    | `https`         |
| `share_level` | `owner`         |
| `share_level` | `authenticated` |
| `share_level` | `public`        |

## codersdk.UsageAppName

```json
"vscode"
```

### Properties

#### Enumerated Values

| Value              |
|--------------------|
| `vscode`           |
| `jetbrains`        |
| `reconnecting-pty` |
| `ssh`              |

## codersdk.User

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

### Properties

| Name               | Type                                            | Required | Restrictions | Description                                                                                |
|--------------------|-------------------------------------------------|----------|--------------|--------------------------------------------------------------------------------------------|
| `avatar_url`       | string                                          | false    |              |                                                                                            |
| `created_at`       | string                                          | true     |              |                                                                                            |
| `email`            | string                                          | true     |              |                                                                                            |
| `id`               | string                                          | true     |              |                                                                                            |
| `last_seen_at`     | string                                          | false    |              |                                                                                            |
| `login_type`       | [codersdk.LoginType](#codersdklogintype)        | false    |              |                                                                                            |
| `name`             | string                                          | false    |              |                                                                                            |
| `organization_ids` | array of string                                 | false    |              |                                                                                            |
| `roles`            | array of [codersdk.SlimRole](#codersdkslimrole) | false    |              |                                                                                            |
| `status`           | [codersdk.UserStatus](#codersdkuserstatus)      | false    |              |                                                                                            |
| `theme_preference` | string                                          | false    |              | Deprecated: this value should be retrieved from `codersdk.UserPreferenceSettings` instead. |
| `updated_at`       | string                                          | false    |              |                                                                                            |
| `username`         | string                                          | true     |              |                                                                                            |

#### Enumerated Values

| Property | Value       |
|----------|-------------|
| `status` | `active`    |
| `status` | `suspended` |

## codersdk.UserActivity

```json
{
  "avatar_url": "http://example.com",
  "seconds": 80500,
  "template_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5",
  "username": "string"
}
```

### Properties

| Name           | Type            | Required | Restrictions | Description |
|----------------|-----------------|----------|--------------|-------------|
| `avatar_url`   | string          | false    |              |             |
| `seconds`      | integer         | false    |              |             |
| `template_ids` | array of string | false    |              |             |
| `user_id`      | string          | false    |              |             |
| `username`     | string          | false    |              |             |

## codersdk.UserActivityInsightsReport

```json
{
  "end_time": "2019-08-24T14:15:22Z",
  "start_time": "2019-08-24T14:15:22Z",
  "template_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "users": [
    {
      "avatar_url": "http://example.com",
      "seconds": 80500,
      "template_ids": [
        "497f6eca-6276-4993-bfeb-53cbbbba6f08"
      ],
      "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5",
      "username": "string"
    }
  ]
}
```

### Properties

| Name           | Type                                                    | Required | Restrictions | Description |
|----------------|---------------------------------------------------------|----------|--------------|-------------|
| `end_time`     | string                                                  | false    |              |             |
| `start_time`   | string                                                  | false    |              |             |
| `template_ids` | array of string                                         | false    |              |             |
| `users`        | array of [codersdk.UserActivity](#codersdkuseractivity) | false    |              |             |

## codersdk.UserActivityInsightsResponse

```json
{
  "report": {
    "end_time": "2019-08-24T14:15:22Z",
    "start_time": "2019-08-24T14:15:22Z",
    "template_ids": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "users": [
      {
        "avatar_url": "http://example.com",
        "seconds": 80500,
        "template_ids": [
          "497f6eca-6276-4993-bfeb-53cbbbba6f08"
        ],
        "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5",
        "username": "string"
      }
    ]
  }
}
```

### Properties

| Name     | Type                                                                       | Required | Restrictions | Description |
|----------|----------------------------------------------------------------------------|----------|--------------|-------------|
| `report` | [codersdk.UserActivityInsightsReport](#codersdkuseractivityinsightsreport) | false    |              |             |

## codersdk.UserAppearanceSettings

```json
{
  "terminal_font": "ibm-mono-plex",
  "theme_preference": "string"
}
```

### Properties

| Name               | Type                                                   | Required | Restrictions | Description |
|--------------------|--------------------------------------------------------|----------|--------------|-------------|
| `terminal_font`    | [codersdk.TerminalFontName](#codersdkterminalfontname) | false    |              |             |
| `theme_preference` | string                                                 | false    |              |             |

#### Enumerated Values

| Property        | Value           |
|-----------------|-----------------|
| `terminal_font` | `ibm-mono-plex` |
| `terminal_font` | `fira-code`     |

## codersdk.UserLatency

```json
{
  "avatar_url": "http://example.com",
  "latency_ms": {
    "p50": 31.312,
    "p95": 119.832
  },
  "template_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5",
  "username": "string"
}
```

### Properties

| Name           | Type                                                     | Required | Restrictions | Description |
|----------------|----------------------------------------------------------|----------|--------------|-------------|
| `avatar_url`   | string                                                   | false    |              |             |
| `latency_ms`   | [codersdk.ConnectionLatency](#codersdkconnectionlatency) | false    |              |             |
| `template_ids` | array of string                                          | false    |              |             |
| `user_id`      | string                                                   | false    |              |             |
| `username`     | string                                                   | false    |              |             |

## codersdk.UserLatencyInsightsReport

```json
{
  "end_time": "2019-08-24T14:15:22Z",
  "start_time": "2019-08-24T14:15:22Z",
  "template_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "users": [
    {
      "avatar_url": "http://example.com",
      "latency_ms": {
        "p50": 31.312,
        "p95": 119.832
      },
      "template_ids": [
        "497f6eca-6276-4993-bfeb-53cbbbba6f08"
      ],
      "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5",
      "username": "string"
    }
  ]
}
```

### Properties

| Name           | Type                                                  | Required | Restrictions | Description |
|----------------|-------------------------------------------------------|----------|--------------|-------------|
| `end_time`     | string                                                | false    |              |             |
| `start_time`   | string                                                | false    |              |             |
| `template_ids` | array of string                                       | false    |              |             |
| `users`        | array of [codersdk.UserLatency](#codersdkuserlatency) | false    |              |             |

## codersdk.UserLatencyInsightsResponse

```json
{
  "report": {
    "end_time": "2019-08-24T14:15:22Z",
    "start_time": "2019-08-24T14:15:22Z",
    "template_ids": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "users": [
      {
        "avatar_url": "http://example.com",
        "latency_ms": {
          "p50": 31.312,
          "p95": 119.832
        },
        "template_ids": [
          "497f6eca-6276-4993-bfeb-53cbbbba6f08"
        ],
        "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5",
        "username": "string"
      }
    ]
  }
}
```

### Properties

| Name     | Type                                                                     | Required | Restrictions | Description |
|----------|--------------------------------------------------------------------------|----------|--------------|-------------|
| `report` | [codersdk.UserLatencyInsightsReport](#codersdkuserlatencyinsightsreport) | false    |              |             |

## codersdk.UserLoginType

```json
{
  "login_type": ""
}
```

### Properties

| Name         | Type                                     | Required | Restrictions | Description |
|--------------|------------------------------------------|----------|--------------|-------------|
| `login_type` | [codersdk.LoginType](#codersdklogintype) | false    |              |             |

## codersdk.UserParameter

```json
{
  "name": "string",
  "value": "string"
}
```

### Properties

| Name    | Type   | Required | Restrictions | Description |
|---------|--------|----------|--------------|-------------|
| `name`  | string | false    |              |             |
| `value` | string | false    |              |             |

## codersdk.UserQuietHoursScheduleConfig

```json
{
  "allow_user_custom": true,
  "default_schedule": "string"
}
```

### Properties

| Name                | Type    | Required | Restrictions | Description |
|---------------------|---------|----------|--------------|-------------|
| `allow_user_custom` | boolean | false    |              |             |
| `default_schedule`  | string  | false    |              |             |

## codersdk.UserQuietHoursScheduleResponse

```json
{
  "next": "2019-08-24T14:15:22Z",
  "raw_schedule": "string",
  "time": "string",
  "timezone": "string",
  "user_can_set": true,
  "user_set": true
}
```

### Properties

| Name           | Type    | Required | Restrictions | Description                                                                                                                                                                      |
|----------------|---------|----------|--------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `next`         | string  | false    |              | Next is the next time that the quiet hours window will start.                                                                                                                    |
| `raw_schedule` | string  | false    |              |                                                                                                                                                                                  |
| `time`         | string  | false    |              | Time is the time of day that the quiet hours window starts in the given Timezone each day.                                                                                       |
| `timezone`     | string  | false    |              | raw format from the cron expression, UTC if unspecified                                                                                                                          |
| `user_can_set` | boolean | false    |              | User can set is true if the user is allowed to set their own quiet hours schedule. If false, the user cannot set a custom schedule and the default schedule will always be used. |
| `user_set`     | boolean | false    |              | User set is true if the user has set their own quiet hours schedule. If false, the user is using the default schedule.                                                           |

## codersdk.UserStatus

```json
"active"
```

### Properties

#### Enumerated Values

| Value       |
|-------------|
| `active`    |
| `dormant`   |
| `suspended` |

## codersdk.UserStatusChangeCount

```json
{
  "count": 10,
  "date": "2019-08-24T14:15:22Z"
}
```

### Properties

| Name    | Type    | Required | Restrictions | Description |
|---------|---------|----------|--------------|-------------|
| `count` | integer | false    |              |             |
| `date`  | string  | false    |              |             |

## codersdk.ValidateUserPasswordRequest

```json
{
  "password": "string"
}
```

### Properties

| Name       | Type   | Required | Restrictions | Description |
|------------|--------|----------|--------------|-------------|
| `password` | string | true     |              |             |

## codersdk.ValidateUserPasswordResponse

```json
{
  "details": "string",
  "valid": true
}
```

### Properties

| Name      | Type    | Required | Restrictions | Description |
|-----------|---------|----------|--------------|-------------|
| `details` | string  | false    |              |             |
| `valid`   | boolean | false    |              |             |

## codersdk.ValidationError

```json
{
  "detail": "string",
  "field": "string"
}
```

### Properties

| Name     | Type   | Required | Restrictions | Description |
|----------|--------|----------|--------------|-------------|
| `detail` | string | true     |              |             |
| `field`  | string | true     |              |             |

## codersdk.ValidationMonotonicOrder

```json
"increasing"
```

### Properties

#### Enumerated Values

| Value        |
|--------------|
| `increasing` |
| `decreasing` |

## codersdk.VariableValue

```json
{
  "name": "string",
  "value": "string"
}
```

### Properties

| Name    | Type   | Required | Restrictions | Description |
|---------|--------|----------|--------------|-------------|
| `name`  | string | false    |              |             |
| `value` | string | false    |              |             |

## codersdk.WebpushSubscription

```json
{
  "auth_key": "string",
  "endpoint": "string",
  "p256dh_key": "string"
}
```

### Properties

| Name         | Type   | Required | Restrictions | Description |
|--------------|--------|----------|--------------|-------------|
| `auth_key`   | string | false    |              |             |
| `endpoint`   | string | false    |              |             |
| `p256dh_key` | string | false    |              |             |

## codersdk.Workspace

```json
{
  "allow_renames": true,
  "automatic_updates": "always",
  "autostart_schedule": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "deleting_at": "2019-08-24T14:15:22Z",
  "dormant_at": "2019-08-24T14:15:22Z",
  "favorite": true,
  "health": {
    "failing_agents": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "healthy": false
  },
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "last_used_at": "2019-08-24T14:15:22Z",
  "latest_app_status": {
    "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
    "app_id": "affd1d10-9538-4fc8-9e0b-4594a28c1335",
    "created_at": "2019-08-24T14:15:22Z",
    "icon": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "message": "string",
    "needs_user_attention": true,
    "state": "working",
    "uri": "string",
    "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
  },
  "latest_build": {
    "build_number": 0,
    "created_at": "2019-08-24T14:15:22Z",
    "daily_cost": 0,
    "deadline": "2019-08-24T14:15:22Z",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "initiator_id": "06588898-9a84-4b35-ba8f-f9cbd64946f3",
    "initiator_name": "string",
    "job": {
      "available_workers": [
        "497f6eca-6276-4993-bfeb-53cbbbba6f08"
      ],
      "canceled_at": "2019-08-24T14:15:22Z",
      "completed_at": "2019-08-24T14:15:22Z",
      "created_at": "2019-08-24T14:15:22Z",
      "error": "string",
      "error_code": "REQUIRED_TEMPLATE_VARIABLES",
      "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "input": {
        "error": "string",
        "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
        "workspace_build_id": "badaf2eb-96c5-4050-9f1d-db2d39ca5478"
      },
      "metadata": {
        "template_display_name": "string",
        "template_icon": "string",
        "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
        "template_name": "string",
        "template_version_name": "string",
        "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
        "workspace_name": "string"
      },
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "queue_position": 0,
      "queue_size": 0,
      "started_at": "2019-08-24T14:15:22Z",
      "status": "pending",
      "tags": {
        "property1": "string",
        "property2": "string"
      },
      "type": "template_version_import",
      "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
    },
    "matched_provisioners": {
      "available": 0,
      "count": 0,
      "most_recently_seen": "2019-08-24T14:15:22Z"
    },
    "max_deadline": "2019-08-24T14:15:22Z",
    "reason": "initiator",
    "resources": [
      {
        "agents": [
          {
            "api_version": "string",
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
                "hidden": true,
                "icon": "string",
                "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
                "open_in": "slim-window",
                "sharing_level": "owner",
                "slug": "string",
                "statuses": [
                  {
                    "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
                    "app_id": "affd1d10-9538-4fc8-9e0b-4594a28c1335",
                    "created_at": "2019-08-24T14:15:22Z",
                    "icon": "string",
                    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
                    "message": "string",
                    "needs_user_attention": true,
                    "state": "working",
                    "uri": "string",
                    "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
                  }
                ],
                "subdomain": true,
                "subdomain_name": "string",
                "url": "string"
              }
            ],
            "architecture": "string",
            "connection_timeout_seconds": 0,
            "created_at": "2019-08-24T14:15:22Z",
            "directory": "string",
            "disconnected_at": "2019-08-24T14:15:22Z",
            "display_apps": [
              "vscode"
            ],
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
            "log_sources": [
              {
                "created_at": "2019-08-24T14:15:22Z",
                "display_name": "string",
                "icon": "string",
                "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
                "workspace_agent_id": "7ad2e618-fea7-4c1a-b70a-f501566a72f1"
              }
            ],
            "logs_length": 0,
            "logs_overflowed": true,
            "name": "string",
            "operating_system": "string",
            "ready_at": "2019-08-24T14:15:22Z",
            "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
            "scripts": [
              {
                "cron": "string",
                "display_name": "string",
                "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
                "log_path": "string",
                "log_source_id": "4197ab25-95cf-4b91-9c78-f7f2af5d353a",
                "run_on_start": true,
                "run_on_stop": true,
                "script": "string",
                "start_blocks_login": true,
                "timeout": 0
              }
            ],
            "started_at": "2019-08-24T14:15:22Z",
            "startup_script_behavior": "blocking",
            "status": "connecting",
            "subsystems": [
              "envbox"
            ],
            "troubleshooting_url": "string",
            "updated_at": "2019-08-24T14:15:22Z",
            "version": "string"
          }
        ],
        "created_at": "2019-08-24T14:15:22Z",
        "daily_cost": 0,
        "hide": true,
        "icon": "string",
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "job_id": "453bd7d7-5355-4d6d-a38e-d9e7eb218c3f",
        "metadata": [
          {
            "key": "string",
            "sensitive": true,
            "value": "string"
          }
        ],
        "name": "string",
        "type": "string",
        "workspace_transition": "start"
      }
    ],
    "status": "pending",
    "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
    "template_version_name": "string",
    "transition": "start",
    "updated_at": "2019-08-24T14:15:22Z",
    "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
    "workspace_name": "string",
    "workspace_owner_avatar_url": "string",
    "workspace_owner_id": "e7078695-5279-4c86-8774-3ac2367a2fc7",
    "workspace_owner_name": "string"
  },
  "name": "string",
  "next_start_at": "2019-08-24T14:15:22Z",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "organization_name": "string",
  "outdated": true,
  "owner_avatar_url": "string",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "owner_name": "string",
  "template_active_version_id": "b0da9c29-67d8-4c87-888c-bafe356f7f3c",
  "template_allow_user_cancel_workspace_jobs": true,
  "template_display_name": "string",
  "template_icon": "string",
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "template_name": "string",
  "template_require_active_version": true,
  "ttl_ms": 0,
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Properties

| Name                                        | Type                                                       | Required | Restrictions | Description                                                                                                                                                                                                                                           |
|---------------------------------------------|------------------------------------------------------------|----------|--------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `allow_renames`                             | boolean                                                    | false    |              |                                                                                                                                                                                                                                                       |
| `automatic_updates`                         | [codersdk.AutomaticUpdates](#codersdkautomaticupdates)     | false    |              |                                                                                                                                                                                                                                                       |
| `autostart_schedule`                        | string                                                     | false    |              |                                                                                                                                                                                                                                                       |
| `created_at`                                | string                                                     | false    |              |                                                                                                                                                                                                                                                       |
| `deleting_at`                               | string                                                     | false    |              | Deleting at indicates the time at which the workspace will be permanently deleted. A workspace is eligible for deletion if it is dormant (a non-nil dormant_at value) and a value has been specified for time_til_dormant_autodelete on its template. |
| `dormant_at`                                | string                                                     | false    |              | Dormant at being non-nil indicates a workspace that is dormant. A dormant workspace is no longer accessible must be activated. It is subject to deletion if it breaches the duration of the time_til_ field on its template.                          |
| `favorite`                                  | boolean                                                    | false    |              |                                                                                                                                                                                                                                                       |
| `health`                                    | [codersdk.WorkspaceHealth](#codersdkworkspacehealth)       | false    |              | Health shows the health of the workspace and information about what is causing an unhealthy status.                                                                                                                                                   |
| `id`                                        | string                                                     | false    |              |                                                                                                                                                                                                                                                       |
| `last_used_at`                              | string                                                     | false    |              |                                                                                                                                                                                                                                                       |
| `latest_app_status`                         | [codersdk.WorkspaceAppStatus](#codersdkworkspaceappstatus) | false    |              |                                                                                                                                                                                                                                                       |
| `latest_build`                              | [codersdk.WorkspaceBuild](#codersdkworkspacebuild)         | false    |              |                                                                                                                                                                                                                                                       |
| `name`                                      | string                                                     | false    |              |                                                                                                                                                                                                                                                       |
| `next_start_at`                             | string                                                     | false    |              |                                                                                                                                                                                                                                                       |
| `organization_id`                           | string                                                     | false    |              |                                                                                                                                                                                                                                                       |
| `organization_name`                         | string                                                     | false    |              |                                                                                                                                                                                                                                                       |
| `outdated`                                  | boolean                                                    | false    |              |                                                                                                                                                                                                                                                       |
| `owner_avatar_url`                          | string                                                     | false    |              |                                                                                                                                                                                                                                                       |
| `owner_id`                                  | string                                                     | false    |              |                                                                                                                                                                                                                                                       |
| `owner_name`                                | string                                                     | false    |              |                                                                                                                                                                                                                                                       |
| `template_active_version_id`                | string                                                     | false    |              |                                                                                                                                                                                                                                                       |
| `template_allow_user_cancel_workspace_jobs` | boolean                                                    | false    |              |                                                                                                                                                                                                                                                       |
| `template_display_name`                     | string                                                     | false    |              |                                                                                                                                                                                                                                                       |
| `template_icon`                             | string                                                     | false    |              |                                                                                                                                                                                                                                                       |
| `template_id`                               | string                                                     | false    |              |                                                                                                                                                                                                                                                       |
| `template_name`                             | string                                                     | false    |              |                                                                                                                                                                                                                                                       |
| `template_require_active_version`           | boolean                                                    | false    |              |                                                                                                                                                                                                                                                       |
| `ttl_ms`                                    | integer                                                    | false    |              |                                                                                                                                                                                                                                                       |
| `updated_at`                                | string                                                     | false    |              |                                                                                                                                                                                                                                                       |

#### Enumerated Values

| Property            | Value    |
|---------------------|----------|
| `automatic_updates` | `always` |
| `automatic_updates` | `never`  |

## codersdk.WorkspaceAgent

```json
{
  "api_version": "string",
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
      "hidden": true,
      "icon": "string",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "open_in": "slim-window",
      "sharing_level": "owner",
      "slug": "string",
      "statuses": [
        {
          "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
          "app_id": "affd1d10-9538-4fc8-9e0b-4594a28c1335",
          "created_at": "2019-08-24T14:15:22Z",
          "icon": "string",
          "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
          "message": "string",
          "needs_user_attention": true,
          "state": "working",
          "uri": "string",
          "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
        }
      ],
      "subdomain": true,
      "subdomain_name": "string",
      "url": "string"
    }
  ],
  "architecture": "string",
  "connection_timeout_seconds": 0,
  "created_at": "2019-08-24T14:15:22Z",
  "directory": "string",
  "disconnected_at": "2019-08-24T14:15:22Z",
  "display_apps": [
    "vscode"
  ],
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
  "log_sources": [
    {
      "created_at": "2019-08-24T14:15:22Z",
      "display_name": "string",
      "icon": "string",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "workspace_agent_id": "7ad2e618-fea7-4c1a-b70a-f501566a72f1"
    }
  ],
  "logs_length": 0,
  "logs_overflowed": true,
  "name": "string",
  "operating_system": "string",
  "ready_at": "2019-08-24T14:15:22Z",
  "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
  "scripts": [
    {
      "cron": "string",
      "display_name": "string",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "log_path": "string",
      "log_source_id": "4197ab25-95cf-4b91-9c78-f7f2af5d353a",
      "run_on_start": true,
      "run_on_stop": true,
      "script": "string",
      "start_blocks_login": true,
      "timeout": 0
    }
  ],
  "started_at": "2019-08-24T14:15:22Z",
  "startup_script_behavior": "blocking",
  "status": "connecting",
  "subsystems": [
    "envbox"
  ],
  "troubleshooting_url": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "version": "string"
}
```

### Properties

| Name                         | Type                                                                                         | Required | Restrictions | Description                                                                                                                                                                  |
|------------------------------|----------------------------------------------------------------------------------------------|----------|--------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `api_version`                | string                                                                                       | false    |              |                                                                                                                                                                              |
| `apps`                       | array of [codersdk.WorkspaceApp](#codersdkworkspaceapp)                                      | false    |              |                                                                                                                                                                              |
| `architecture`               | string                                                                                       | false    |              |                                                                                                                                                                              |
| `connection_timeout_seconds` | integer                                                                                      | false    |              |                                                                                                                                                                              |
| `created_at`                 | string                                                                                       | false    |              |                                                                                                                                                                              |
| `directory`                  | string                                                                                       | false    |              |                                                                                                                                                                              |
| `disconnected_at`            | string                                                                                       | false    |              |                                                                                                                                                                              |
| `display_apps`               | array of [codersdk.DisplayApp](#codersdkdisplayapp)                                          | false    |              |                                                                                                                                                                              |
| `environment_variables`      | object                                                                                       | false    |              |                                                                                                                                                                              |
| » `[any property]`           | string                                                                                       | false    |              |                                                                                                                                                                              |
| `expanded_directory`         | string                                                                                       | false    |              |                                                                                                                                                                              |
| `first_connected_at`         | string                                                                                       | false    |              |                                                                                                                                                                              |
| `health`                     | [codersdk.WorkspaceAgentHealth](#codersdkworkspaceagenthealth)                               | false    |              | Health reports the health of the agent.                                                                                                                                      |
| `id`                         | string                                                                                       | false    |              |                                                                                                                                                                              |
| `instance_id`                | string                                                                                       | false    |              |                                                                                                                                                                              |
| `last_connected_at`          | string                                                                                       | false    |              |                                                                                                                                                                              |
| `latency`                    | object                                                                                       | false    |              | Latency is mapped by region name (e.g. "New York City", "Seattle").                                                                                                          |
| » `[any property]`           | [codersdk.DERPRegion](#codersdkderpregion)                                                   | false    |              |                                                                                                                                                                              |
| `lifecycle_state`            | [codersdk.WorkspaceAgentLifecycle](#codersdkworkspaceagentlifecycle)                         | false    |              |                                                                                                                                                                              |
| `log_sources`                | array of [codersdk.WorkspaceAgentLogSource](#codersdkworkspaceagentlogsource)                | false    |              |                                                                                                                                                                              |
| `logs_length`                | integer                                                                                      | false    |              |                                                                                                                                                                              |
| `logs_overflowed`            | boolean                                                                                      | false    |              |                                                                                                                                                                              |
| `name`                       | string                                                                                       | false    |              |                                                                                                                                                                              |
| `operating_system`           | string                                                                                       | false    |              |                                                                                                                                                                              |
| `ready_at`                   | string                                                                                       | false    |              |                                                                                                                                                                              |
| `resource_id`                | string                                                                                       | false    |              |                                                                                                                                                                              |
| `scripts`                    | array of [codersdk.WorkspaceAgentScript](#codersdkworkspaceagentscript)                      | false    |              |                                                                                                                                                                              |
| `started_at`                 | string                                                                                       | false    |              |                                                                                                                                                                              |
| `startup_script_behavior`    | [codersdk.WorkspaceAgentStartupScriptBehavior](#codersdkworkspaceagentstartupscriptbehavior) | false    |              | Startup script behavior is a legacy field that is deprecated in favor of the `coder_script` resource. It's only referenced by old clients. Deprecated: Remove in the future! |
| `status`                     | [codersdk.WorkspaceAgentStatus](#codersdkworkspaceagentstatus)                               | false    |              |                                                                                                                                                                              |
| `subsystems`                 | array of [codersdk.AgentSubsystem](#codersdkagentsubsystem)                                  | false    |              |                                                                                                                                                                              |
| `troubleshooting_url`        | string                                                                                       | false    |              |                                                                                                                                                                              |
| `updated_at`                 | string                                                                                       | false    |              |                                                                                                                                                                              |
| `version`                    | string                                                                                       | false    |              |                                                                                                                                                                              |

## codersdk.WorkspaceAgentContainer

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "id": "string",
  "image": "string",
  "labels": {
    "property1": "string",
    "property2": "string"
  },
  "name": "string",
  "ports": [
    {
      "host_ip": "string",
      "host_port": 0,
      "network": "string",
      "port": 0
    }
  ],
  "running": true,
  "status": "string",
  "volumes": {
    "property1": "string",
    "property2": "string"
  }
}
```

### Properties

| Name               | Type                                                                                  | Required | Restrictions | Description                                                                                                                                |
|--------------------|---------------------------------------------------------------------------------------|----------|--------------|--------------------------------------------------------------------------------------------------------------------------------------------|
| `created_at`       | string                                                                                | false    |              | Created at is the time the container was created.                                                                                          |
| `id`               | string                                                                                | false    |              | ID is the unique identifier of the container.                                                                                              |
| `image`            | string                                                                                | false    |              | Image is the name of the container image.                                                                                                  |
| `labels`           | object                                                                                | false    |              | Labels is a map of key-value pairs of container labels.                                                                                    |
| » `[any property]` | string                                                                                | false    |              |                                                                                                                                            |
| `name`             | string                                                                                | false    |              | Name is the human-readable name of the container.                                                                                          |
| `ports`            | array of [codersdk.WorkspaceAgentContainerPort](#codersdkworkspaceagentcontainerport) | false    |              | Ports includes ports exposed by the container.                                                                                             |
| `running`          | boolean                                                                               | false    |              | Running is true if the container is currently running.                                                                                     |
| `status`           | string                                                                                | false    |              | Status is the current status of the container. This is somewhat implementation-dependent, but should generally be a human-readable string. |
| `volumes`          | object                                                                                | false    |              | Volumes is a map of "things" mounted into the container. Again, this is somewhat implementation-dependent.                                 |
| » `[any property]` | string                                                                                | false    |              |                                                                                                                                            |

## codersdk.WorkspaceAgentContainerPort

```json
{
  "host_ip": "string",
  "host_port": 0,
  "network": "string",
  "port": 0
}
```

### Properties

| Name        | Type    | Required | Restrictions | Description                                                                                                                |
|-------------|---------|----------|--------------|----------------------------------------------------------------------------------------------------------------------------|
| `host_ip`   | string  | false    |              | Host ip is the IP address of the host interface to which the port is bound. Note that this can be an IPv4 or IPv6 address. |
| `host_port` | integer | false    |              | Host port is the port number *outside* the container.                                                                      |
| `network`   | string  | false    |              | Network is the network protocol used by the port (tcp, udp, etc).                                                          |
| `port`      | integer | false    |              | Port is the port number *inside* the container.                                                                            |

## codersdk.WorkspaceAgentHealth

```json
{
  "healthy": false,
  "reason": "agent has lost connection"
}
```

### Properties

| Name      | Type    | Required | Restrictions | Description                                                                                   |
|-----------|---------|----------|--------------|-----------------------------------------------------------------------------------------------|
| `healthy` | boolean | false    |              | Healthy is true if the agent is healthy.                                                      |
| `reason`  | string  | false    |              | Reason is a human-readable explanation of the agent's health. It is empty if Healthy is true. |

## codersdk.WorkspaceAgentLifecycle

```json
"created"
```

### Properties

#### Enumerated Values

| Value              |
|--------------------|
| `created`          |
| `starting`         |
| `start_timeout`    |
| `start_error`      |
| `ready`            |
| `shutting_down`    |
| `shutdown_timeout` |
| `shutdown_error`   |
| `off`              |

## codersdk.WorkspaceAgentListContainersResponse

```json
{
  "containers": [
    {
      "created_at": "2019-08-24T14:15:22Z",
      "id": "string",
      "image": "string",
      "labels": {
        "property1": "string",
        "property2": "string"
      },
      "name": "string",
      "ports": [
        {
          "host_ip": "string",
          "host_port": 0,
          "network": "string",
          "port": 0
        }
      ],
      "running": true,
      "status": "string",
      "volumes": {
        "property1": "string",
        "property2": "string"
      }
    }
  ],
  "warnings": [
    "string"
  ]
}
```

### Properties

| Name         | Type                                                                          | Required | Restrictions | Description                                                                                                                           |
|--------------|-------------------------------------------------------------------------------|----------|--------------|---------------------------------------------------------------------------------------------------------------------------------------|
| `containers` | array of [codersdk.WorkspaceAgentContainer](#codersdkworkspaceagentcontainer) | false    |              | Containers is a list of containers visible to the workspace agent.                                                                    |
| `warnings`   | array of string                                                               | false    |              | Warnings is a list of warnings that may have occurred during the process of listing containers. This should not include fatal errors. |

## codersdk.WorkspaceAgentListeningPort

```json
{
  "network": "string",
  "port": 0,
  "process_name": "string"
}
```

### Properties

| Name           | Type    | Required | Restrictions | Description              |
|----------------|---------|----------|--------------|--------------------------|
| `network`      | string  | false    |              | only "tcp" at the moment |
| `port`         | integer | false    |              |                          |
| `process_name` | string  | false    |              | may be empty             |

## codersdk.WorkspaceAgentListeningPortsResponse

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

### Properties

| Name    | Type                                                                                  | Required | Restrictions | Description                                                                                                                                                                                                                                            |
|---------|---------------------------------------------------------------------------------------|----------|--------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `ports` | array of [codersdk.WorkspaceAgentListeningPort](#codersdkworkspaceagentlisteningport) | false    |              | If there are no ports in the list, nothing should be displayed in the UI. There must not be a "no ports available" message or anything similar, as there will always be no ports displayed on platforms where our port detection logic is unsupported. |

## codersdk.WorkspaceAgentLog

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "id": 0,
  "level": "trace",
  "output": "string",
  "source_id": "ae50a35c-df42-4eff-ba26-f8bc28d2af81"
}
```

### Properties

| Name         | Type                                   | Required | Restrictions | Description |
|--------------|----------------------------------------|----------|--------------|-------------|
| `created_at` | string                                 | false    |              |             |
| `id`         | integer                                | false    |              |             |
| `level`      | [codersdk.LogLevel](#codersdkloglevel) | false    |              |             |
| `output`     | string                                 | false    |              |             |
| `source_id`  | string                                 | false    |              |             |

## codersdk.WorkspaceAgentLogSource

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "display_name": "string",
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "workspace_agent_id": "7ad2e618-fea7-4c1a-b70a-f501566a72f1"
}
```

### Properties

| Name                 | Type   | Required | Restrictions | Description |
|----------------------|--------|----------|--------------|-------------|
| `created_at`         | string | false    |              |             |
| `display_name`       | string | false    |              |             |
| `icon`               | string | false    |              |             |
| `id`                 | string | false    |              |             |
| `workspace_agent_id` | string | false    |              |             |

## codersdk.WorkspaceAgentPortShare

```json
{
  "agent_name": "string",
  "port": 0,
  "protocol": "http",
  "share_level": "owner",
  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
}
```

### Properties

| Name           | Type                                                                                 | Required | Restrictions | Description |
|----------------|--------------------------------------------------------------------------------------|----------|--------------|-------------|
| `agent_name`   | string                                                                               | false    |              |             |
| `port`         | integer                                                                              | false    |              |             |
| `protocol`     | [codersdk.WorkspaceAgentPortShareProtocol](#codersdkworkspaceagentportshareprotocol) | false    |              |             |
| `share_level`  | [codersdk.WorkspaceAgentPortShareLevel](#codersdkworkspaceagentportsharelevel)       | false    |              |             |
| `workspace_id` | string                                                                               | false    |              |             |

#### Enumerated Values

| Property      | Value           |
|---------------|-----------------|
| `protocol`    | `http`          |
| `protocol`    | `https`         |
| `share_level` | `owner`         |
| `share_level` | `authenticated` |
| `share_level` | `public`        |

## codersdk.WorkspaceAgentPortShareLevel

```json
"owner"
```

### Properties

#### Enumerated Values

| Value           |
|-----------------|
| `owner`         |
| `authenticated` |
| `public`        |

## codersdk.WorkspaceAgentPortShareProtocol

```json
"http"
```

### Properties

#### Enumerated Values

| Value   |
|---------|
| `http`  |
| `https` |

## codersdk.WorkspaceAgentPortShares

```json
{
  "shares": [
    {
      "agent_name": "string",
      "port": 0,
      "protocol": "http",
      "share_level": "owner",
      "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
    }
  ]
}
```

### Properties

| Name     | Type                                                                          | Required | Restrictions | Description |
|----------|-------------------------------------------------------------------------------|----------|--------------|-------------|
| `shares` | array of [codersdk.WorkspaceAgentPortShare](#codersdkworkspaceagentportshare) | false    |              |             |

## codersdk.WorkspaceAgentScript

```json
{
  "cron": "string",
  "display_name": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "log_path": "string",
  "log_source_id": "4197ab25-95cf-4b91-9c78-f7f2af5d353a",
  "run_on_start": true,
  "run_on_stop": true,
  "script": "string",
  "start_blocks_login": true,
  "timeout": 0
}
```

### Properties

| Name                 | Type    | Required | Restrictions | Description |
|----------------------|---------|----------|--------------|-------------|
| `cron`               | string  | false    |              |             |
| `display_name`       | string  | false    |              |             |
| `id`                 | string  | false    |              |             |
| `log_path`           | string  | false    |              |             |
| `log_source_id`      | string  | false    |              |             |
| `run_on_start`       | boolean | false    |              |             |
| `run_on_stop`        | boolean | false    |              |             |
| `script`             | string  | false    |              |             |
| `start_blocks_login` | boolean | false    |              |             |
| `timeout`            | integer | false    |              |             |

## codersdk.WorkspaceAgentStartupScriptBehavior

```json
"blocking"
```

### Properties

#### Enumerated Values

| Value          |
|----------------|
| `blocking`     |
| `non-blocking` |

## codersdk.WorkspaceAgentStatus

```json
"connecting"
```

### Properties

#### Enumerated Values

| Value          |
|----------------|
| `connecting`   |
| `connected`    |
| `disconnected` |
| `timeout`      |

## codersdk.WorkspaceApp

```json
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
  "hidden": true,
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "open_in": "slim-window",
  "sharing_level": "owner",
  "slug": "string",
  "statuses": [
    {
      "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
      "app_id": "affd1d10-9538-4fc8-9e0b-4594a28c1335",
      "created_at": "2019-08-24T14:15:22Z",
      "icon": "string",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "message": "string",
      "needs_user_attention": true,
      "state": "working",
      "uri": "string",
      "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
    }
  ],
  "subdomain": true,
  "subdomain_name": "string",
  "url": "string"
}
```

### Properties

| Name             | Type                                                                   | Required | Restrictions | Description                                                                                                                                                                                                                                    |
|------------------|------------------------------------------------------------------------|----------|--------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `command`        | string                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `display_name`   | string                                                                 | false    |              | Display name is a friendly name for the app.                                                                                                                                                                                                   |
| `external`       | boolean                                                                | false    |              | External specifies whether the URL should be opened externally on the client or not.                                                                                                                                                           |
| `health`         | [codersdk.WorkspaceAppHealth](#codersdkworkspaceapphealth)             | false    |              |                                                                                                                                                                                                                                                |
| `healthcheck`    | [codersdk.Healthcheck](#codersdkhealthcheck)                           | false    |              | Healthcheck specifies the configuration for checking app health.                                                                                                                                                                               |
| `hidden`         | boolean                                                                | false    |              |                                                                                                                                                                                                                                                |
| `icon`           | string                                                                 | false    |              | Icon is a relative path or external URL that specifies an icon to be displayed in the dashboard.                                                                                                                                               |
| `id`             | string                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `open_in`        | [codersdk.WorkspaceAppOpenIn](#codersdkworkspaceappopenin)             | false    |              |                                                                                                                                                                                                                                                |
| `sharing_level`  | [codersdk.WorkspaceAppSharingLevel](#codersdkworkspaceappsharinglevel) | false    |              |                                                                                                                                                                                                                                                |
| `slug`           | string                                                                 | false    |              | Slug is a unique identifier within the agent.                                                                                                                                                                                                  |
| `statuses`       | array of [codersdk.WorkspaceAppStatus](#codersdkworkspaceappstatus)    | false    |              | Statuses is a list of statuses for the app.                                                                                                                                                                                                    |
| `subdomain`      | boolean                                                                | false    |              | Subdomain denotes whether the app should be accessed via a path on the `coder server` or via a hostname-based dev URL. If this is set to true and there is no app wildcard configured on the server, the app will not be accessible in the UI. |
| `subdomain_name` | string                                                                 | false    |              | Subdomain name is the application domain exposed on the `coder server`.                                                                                                                                                                        |
| `url`            | string                                                                 | false    |              | URL is the address being proxied to inside the workspace. If external is specified, this will be opened on the client.                                                                                                                         |

#### Enumerated Values

| Property        | Value           |
|-----------------|-----------------|
| `sharing_level` | `owner`         |
| `sharing_level` | `authenticated` |
| `sharing_level` | `public`        |

## codersdk.WorkspaceAppHealth

```json
"disabled"
```

### Properties

#### Enumerated Values

| Value          |
|----------------|
| `disabled`     |
| `initializing` |
| `healthy`      |
| `unhealthy`    |

## codersdk.WorkspaceAppOpenIn

```json
"slim-window"
```

### Properties

#### Enumerated Values

| Value         |
|---------------|
| `slim-window` |
| `tab`         |

## codersdk.WorkspaceAppSharingLevel

```json
"owner"
```

### Properties

#### Enumerated Values

| Value           |
|-----------------|
| `owner`         |
| `authenticated` |
| `public`        |

## codersdk.WorkspaceAppStatus

```json
{
  "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
  "app_id": "affd1d10-9538-4fc8-9e0b-4594a28c1335",
  "created_at": "2019-08-24T14:15:22Z",
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "message": "string",
  "needs_user_attention": true,
  "state": "working",
  "uri": "string",
  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
}
```

### Properties

| Name                   | Type                                                                 | Required | Restrictions | Description                                                                                                                |
|------------------------|----------------------------------------------------------------------|----------|--------------|----------------------------------------------------------------------------------------------------------------------------|
| `agent_id`             | string                                                               | false    |              |                                                                                                                            |
| `app_id`               | string                                                               | false    |              |                                                                                                                            |
| `created_at`           | string                                                               | false    |              |                                                                                                                            |
| `icon`                 | string                                                               | false    |              | Icon is an external URL to an icon that will be rendered in the UI.                                                        |
| `id`                   | string                                                               | false    |              |                                                                                                                            |
| `message`              | string                                                               | false    |              |                                                                                                                            |
| `needs_user_attention` | boolean                                                              | false    |              |                                                                                                                            |
| `state`                | [codersdk.WorkspaceAppStatusState](#codersdkworkspaceappstatusstate) | false    |              |                                                                                                                            |
| `uri`                  | string                                                               | false    |              | Uri is the URI of the resource that the status is for. e.g. https://github.com/org/repo/pull/123 e.g. file:///path/to/file |
| `workspace_id`         | string                                                               | false    |              |                                                                                                                            |

## codersdk.WorkspaceAppStatusState

```json
"working"
```

### Properties

#### Enumerated Values

| Value      |
|------------|
| `working`  |
| `complete` |
| `failure`  |

## codersdk.WorkspaceBuild

```json
{
  "build_number": 0,
  "created_at": "2019-08-24T14:15:22Z",
  "daily_cost": 0,
  "deadline": "2019-08-24T14:15:22Z",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "initiator_id": "06588898-9a84-4b35-ba8f-f9cbd64946f3",
  "initiator_name": "string",
  "job": {
    "available_workers": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "canceled_at": "2019-08-24T14:15:22Z",
    "completed_at": "2019-08-24T14:15:22Z",
    "created_at": "2019-08-24T14:15:22Z",
    "error": "string",
    "error_code": "REQUIRED_TEMPLATE_VARIABLES",
    "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "input": {
      "error": "string",
      "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
      "workspace_build_id": "badaf2eb-96c5-4050-9f1d-db2d39ca5478"
    },
    "metadata": {
      "template_display_name": "string",
      "template_icon": "string",
      "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
      "template_name": "string",
      "template_version_name": "string",
      "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
      "workspace_name": "string"
    },
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "queue_position": 0,
    "queue_size": 0,
    "started_at": "2019-08-24T14:15:22Z",
    "status": "pending",
    "tags": {
      "property1": "string",
      "property2": "string"
    },
    "type": "template_version_import",
    "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
  },
  "matched_provisioners": {
    "available": 0,
    "count": 0,
    "most_recently_seen": "2019-08-24T14:15:22Z"
  },
  "max_deadline": "2019-08-24T14:15:22Z",
  "reason": "initiator",
  "resources": [
    {
      "agents": [
        {
          "api_version": "string",
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
              "hidden": true,
              "icon": "string",
              "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
              "open_in": "slim-window",
              "sharing_level": "owner",
              "slug": "string",
              "statuses": [
                {
                  "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
                  "app_id": "affd1d10-9538-4fc8-9e0b-4594a28c1335",
                  "created_at": "2019-08-24T14:15:22Z",
                  "icon": "string",
                  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
                  "message": "string",
                  "needs_user_attention": true,
                  "state": "working",
                  "uri": "string",
                  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
                }
              ],
              "subdomain": true,
              "subdomain_name": "string",
              "url": "string"
            }
          ],
          "architecture": "string",
          "connection_timeout_seconds": 0,
          "created_at": "2019-08-24T14:15:22Z",
          "directory": "string",
          "disconnected_at": "2019-08-24T14:15:22Z",
          "display_apps": [
            "vscode"
          ],
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
          "log_sources": [
            {
              "created_at": "2019-08-24T14:15:22Z",
              "display_name": "string",
              "icon": "string",
              "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
              "workspace_agent_id": "7ad2e618-fea7-4c1a-b70a-f501566a72f1"
            }
          ],
          "logs_length": 0,
          "logs_overflowed": true,
          "name": "string",
          "operating_system": "string",
          "ready_at": "2019-08-24T14:15:22Z",
          "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
          "scripts": [
            {
              "cron": "string",
              "display_name": "string",
              "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
              "log_path": "string",
              "log_source_id": "4197ab25-95cf-4b91-9c78-f7f2af5d353a",
              "run_on_start": true,
              "run_on_stop": true,
              "script": "string",
              "start_blocks_login": true,
              "timeout": 0
            }
          ],
          "started_at": "2019-08-24T14:15:22Z",
          "startup_script_behavior": "blocking",
          "status": "connecting",
          "subsystems": [
            "envbox"
          ],
          "troubleshooting_url": "string",
          "updated_at": "2019-08-24T14:15:22Z",
          "version": "string"
        }
      ],
      "created_at": "2019-08-24T14:15:22Z",
      "daily_cost": 0,
      "hide": true,
      "icon": "string",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "job_id": "453bd7d7-5355-4d6d-a38e-d9e7eb218c3f",
      "metadata": [
        {
          "key": "string",
          "sensitive": true,
          "value": "string"
        }
      ],
      "name": "string",
      "type": "string",
      "workspace_transition": "start"
    }
  ],
  "status": "pending",
  "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
  "template_version_name": "string",
  "transition": "start",
  "updated_at": "2019-08-24T14:15:22Z",
  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
  "workspace_name": "string",
  "workspace_owner_avatar_url": "string",
  "workspace_owner_id": "e7078695-5279-4c86-8774-3ac2367a2fc7",
  "workspace_owner_name": "string"
}
```

### Properties

| Name                         | Type                                                              | Required | Restrictions | Description |
|------------------------------|-------------------------------------------------------------------|----------|--------------|-------------|
| `build_number`               | integer                                                           | false    |              |             |
| `created_at`                 | string                                                            | false    |              |             |
| `daily_cost`                 | integer                                                           | false    |              |             |
| `deadline`                   | string                                                            | false    |              |             |
| `id`                         | string                                                            | false    |              |             |
| `initiator_id`               | string                                                            | false    |              |             |
| `initiator_name`             | string                                                            | false    |              |             |
| `job`                        | [codersdk.ProvisionerJob](#codersdkprovisionerjob)                | false    |              |             |
| `matched_provisioners`       | [codersdk.MatchedProvisioners](#codersdkmatchedprovisioners)      | false    |              |             |
| `max_deadline`               | string                                                            | false    |              |             |
| `reason`                     | [codersdk.BuildReason](#codersdkbuildreason)                      | false    |              |             |
| `resources`                  | array of [codersdk.WorkspaceResource](#codersdkworkspaceresource) | false    |              |             |
| `status`                     | [codersdk.WorkspaceStatus](#codersdkworkspacestatus)              | false    |              |             |
| `template_version_id`        | string                                                            | false    |              |             |
| `template_version_name`      | string                                                            | false    |              |             |
| `transition`                 | [codersdk.WorkspaceTransition](#codersdkworkspacetransition)      | false    |              |             |
| `updated_at`                 | string                                                            | false    |              |             |
| `workspace_id`               | string                                                            | false    |              |             |
| `workspace_name`             | string                                                            | false    |              |             |
| `workspace_owner_avatar_url` | string                                                            | false    |              |             |
| `workspace_owner_id`         | string                                                            | false    |              |             |
| `workspace_owner_name`       | string                                                            | false    |              |             |

#### Enumerated Values

| Property     | Value       |
|--------------|-------------|
| `reason`     | `initiator` |
| `reason`     | `autostart` |
| `reason`     | `autostop`  |
| `status`     | `pending`   |
| `status`     | `starting`  |
| `status`     | `running`   |
| `status`     | `stopping`  |
| `status`     | `stopped`   |
| `status`     | `failed`    |
| `status`     | `canceling` |
| `status`     | `canceled`  |
| `status`     | `deleting`  |
| `status`     | `deleted`   |
| `transition` | `start`     |
| `transition` | `stop`      |
| `transition` | `delete`    |

## codersdk.WorkspaceBuildParameter

```json
{
  "name": "string",
  "value": "string"
}
```

### Properties

| Name    | Type   | Required | Restrictions | Description |
|---------|--------|----------|--------------|-------------|
| `name`  | string | false    |              |             |
| `value` | string | false    |              |             |

## codersdk.WorkspaceBuildTimings

```json
{
  "agent_connection_timings": [
    {
      "ended_at": "2019-08-24T14:15:22Z",
      "stage": "init",
      "started_at": "2019-08-24T14:15:22Z",
      "workspace_agent_id": "string",
      "workspace_agent_name": "string"
    }
  ],
  "agent_script_timings": [
    {
      "display_name": "string",
      "ended_at": "2019-08-24T14:15:22Z",
      "exit_code": 0,
      "stage": "init",
      "started_at": "2019-08-24T14:15:22Z",
      "status": "string",
      "workspace_agent_id": "string",
      "workspace_agent_name": "string"
    }
  ],
  "provisioner_timings": [
    {
      "action": "string",
      "ended_at": "2019-08-24T14:15:22Z",
      "job_id": "453bd7d7-5355-4d6d-a38e-d9e7eb218c3f",
      "resource": "string",
      "source": "string",
      "stage": "init",
      "started_at": "2019-08-24T14:15:22Z"
    }
  ]
}
```

### Properties

| Name                       | Type                                                                      | Required | Restrictions | Description                                                                                                      |
|----------------------------|---------------------------------------------------------------------------|----------|--------------|------------------------------------------------------------------------------------------------------------------|
| `agent_connection_timings` | array of [codersdk.AgentConnectionTiming](#codersdkagentconnectiontiming) | false    |              |                                                                                                                  |
| `agent_script_timings`     | array of [codersdk.AgentScriptTiming](#codersdkagentscripttiming)         | false    |              | Agent script timings Consolidate agent-related timing metrics into a single struct when updating the API version |
| `provisioner_timings`      | array of [codersdk.ProvisionerTiming](#codersdkprovisionertiming)         | false    |              |                                                                                                                  |

## codersdk.WorkspaceConnectionLatencyMS

```json
{
  "p50": 0,
  "p95": 0
}
```

### Properties

| Name  | Type   | Required | Restrictions | Description |
|-------|--------|----------|--------------|-------------|
| `p50` | number | false    |              |             |
| `p95` | number | false    |              |             |

## codersdk.WorkspaceDeploymentStats

```json
{
  "building": 0,
  "connection_latency_ms": {
    "p50": 0,
    "p95": 0
  },
  "failed": 0,
  "pending": 0,
  "running": 0,
  "rx_bytes": 0,
  "stopped": 0,
  "tx_bytes": 0
}
```

### Properties

| Name                    | Type                                                                           | Required | Restrictions | Description |
|-------------------------|--------------------------------------------------------------------------------|----------|--------------|-------------|
| `building`              | integer                                                                        | false    |              |             |
| `connection_latency_ms` | [codersdk.WorkspaceConnectionLatencyMS](#codersdkworkspaceconnectionlatencyms) | false    |              |             |
| `failed`                | integer                                                                        | false    |              |             |
| `pending`               | integer                                                                        | false    |              |             |
| `running`               | integer                                                                        | false    |              |             |
| `rx_bytes`              | integer                                                                        | false    |              |             |
| `stopped`               | integer                                                                        | false    |              |             |
| `tx_bytes`              | integer                                                                        | false    |              |             |

## codersdk.WorkspaceHealth

```json
{
  "failing_agents": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "healthy": false
}
```

### Properties

| Name             | Type            | Required | Restrictions | Description                                                          |
|------------------|-----------------|----------|--------------|----------------------------------------------------------------------|
| `failing_agents` | array of string | false    |              | Failing agents lists the IDs of the agents that are failing, if any. |
| `healthy`        | boolean         | false    |              | Healthy is true if the workspace is healthy.                         |

## codersdk.WorkspaceProxy

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

### Properties

| Name                | Type                                                           | Required | Restrictions | Description                                                                                                                                                                       |
|---------------------|----------------------------------------------------------------|----------|--------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `created_at`        | string                                                         | false    |              |                                                                                                                                                                                   |
| `deleted`           | boolean                                                        | false    |              |                                                                                                                                                                                   |
| `derp_enabled`      | boolean                                                        | false    |              |                                                                                                                                                                                   |
| `derp_only`         | boolean                                                        | false    |              |                                                                                                                                                                                   |
| `display_name`      | string                                                         | false    |              |                                                                                                                                                                                   |
| `healthy`           | boolean                                                        | false    |              |                                                                                                                                                                                   |
| `icon_url`          | string                                                         | false    |              |                                                                                                                                                                                   |
| `id`                | string                                                         | false    |              |                                                                                                                                                                                   |
| `name`              | string                                                         | false    |              |                                                                                                                                                                                   |
| `path_app_url`      | string                                                         | false    |              | Path app URL is the URL to the base path for path apps. Optional unless wildcard_hostname is set. E.g. https://us.example.com                                                     |
| `status`            | [codersdk.WorkspaceProxyStatus](#codersdkworkspaceproxystatus) | false    |              | Status is the latest status check of the proxy. This will be empty for deleted proxies. This value can be used to determine if a workspace proxy is healthy and ready to use.     |
| `updated_at`        | string                                                         | false    |              |                                                                                                                                                                                   |
| `version`           | string                                                         | false    |              |                                                                                                                                                                                   |
| `wildcard_hostname` | string                                                         | false    |              | Wildcard hostname is the wildcard hostname for subdomain apps. E.g. *.us.example.com E.g.*--suffix.au.example.com Optional. Does not need to be on the same domain as PathAppURL. |

## codersdk.WorkspaceProxyStatus

```json
{
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
}
```

### Properties

| Name         | Type                                                     | Required | Restrictions | Description                                                               |
|--------------|----------------------------------------------------------|----------|--------------|---------------------------------------------------------------------------|
| `checked_at` | string                                                   | false    |              |                                                                           |
| `report`     | [codersdk.ProxyHealthReport](#codersdkproxyhealthreport) | false    |              | Report provides more information about the health of the workspace proxy. |
| `status`     | [codersdk.ProxyHealthStatus](#codersdkproxyhealthstatus) | false    |              |                                                                           |

## codersdk.WorkspaceQuota

```json
{
  "budget": 0,
  "credits_consumed": 0
}
```

### Properties

| Name               | Type    | Required | Restrictions | Description |
|--------------------|---------|----------|--------------|-------------|
| `budget`           | integer | false    |              |             |
| `credits_consumed` | integer | false    |              |             |

## codersdk.WorkspaceResource

```json
{
  "agents": [
    {
      "api_version": "string",
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
          "hidden": true,
          "icon": "string",
          "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
          "open_in": "slim-window",
          "sharing_level": "owner",
          "slug": "string",
          "statuses": [
            {
              "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
              "app_id": "affd1d10-9538-4fc8-9e0b-4594a28c1335",
              "created_at": "2019-08-24T14:15:22Z",
              "icon": "string",
              "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
              "message": "string",
              "needs_user_attention": true,
              "state": "working",
              "uri": "string",
              "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
            }
          ],
          "subdomain": true,
          "subdomain_name": "string",
          "url": "string"
        }
      ],
      "architecture": "string",
      "connection_timeout_seconds": 0,
      "created_at": "2019-08-24T14:15:22Z",
      "directory": "string",
      "disconnected_at": "2019-08-24T14:15:22Z",
      "display_apps": [
        "vscode"
      ],
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
      "log_sources": [
        {
          "created_at": "2019-08-24T14:15:22Z",
          "display_name": "string",
          "icon": "string",
          "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
          "workspace_agent_id": "7ad2e618-fea7-4c1a-b70a-f501566a72f1"
        }
      ],
      "logs_length": 0,
      "logs_overflowed": true,
      "name": "string",
      "operating_system": "string",
      "ready_at": "2019-08-24T14:15:22Z",
      "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
      "scripts": [
        {
          "cron": "string",
          "display_name": "string",
          "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
          "log_path": "string",
          "log_source_id": "4197ab25-95cf-4b91-9c78-f7f2af5d353a",
          "run_on_start": true,
          "run_on_stop": true,
          "script": "string",
          "start_blocks_login": true,
          "timeout": 0
        }
      ],
      "started_at": "2019-08-24T14:15:22Z",
      "startup_script_behavior": "blocking",
      "status": "connecting",
      "subsystems": [
        "envbox"
      ],
      "troubleshooting_url": "string",
      "updated_at": "2019-08-24T14:15:22Z",
      "version": "string"
    }
  ],
  "created_at": "2019-08-24T14:15:22Z",
  "daily_cost": 0,
  "hide": true,
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "job_id": "453bd7d7-5355-4d6d-a38e-d9e7eb218c3f",
  "metadata": [
    {
      "key": "string",
      "sensitive": true,
      "value": "string"
    }
  ],
  "name": "string",
  "type": "string",
  "workspace_transition": "start"
}
```

### Properties

| Name                   | Type                                                                              | Required | Restrictions | Description |
|------------------------|-----------------------------------------------------------------------------------|----------|--------------|-------------|
| `agents`               | array of [codersdk.WorkspaceAgent](#codersdkworkspaceagent)                       | false    |              |             |
| `created_at`           | string                                                                            | false    |              |             |
| `daily_cost`           | integer                                                                           | false    |              |             |
| `hide`                 | boolean                                                                           | false    |              |             |
| `icon`                 | string                                                                            | false    |              |             |
| `id`                   | string                                                                            | false    |              |             |
| `job_id`               | string                                                                            | false    |              |             |
| `metadata`             | array of [codersdk.WorkspaceResourceMetadata](#codersdkworkspaceresourcemetadata) | false    |              |             |
| `name`                 | string                                                                            | false    |              |             |
| `type`                 | string                                                                            | false    |              |             |
| `workspace_transition` | [codersdk.WorkspaceTransition](#codersdkworkspacetransition)                      | false    |              |             |

#### Enumerated Values

| Property               | Value    |
|------------------------|----------|
| `workspace_transition` | `start`  |
| `workspace_transition` | `stop`   |
| `workspace_transition` | `delete` |

## codersdk.WorkspaceResourceMetadata

```json
{
  "key": "string",
  "sensitive": true,
  "value": "string"
}
```

### Properties

| Name        | Type    | Required | Restrictions | Description |
|-------------|---------|----------|--------------|-------------|
| `key`       | string  | false    |              |             |
| `sensitive` | boolean | false    |              |             |
| `value`     | string  | false    |              |             |

## codersdk.WorkspaceStatus

```json
"pending"
```

### Properties

#### Enumerated Values

| Value       |
|-------------|
| `pending`   |
| `starting`  |
| `running`   |
| `stopping`  |
| `stopped`   |
| `failed`    |
| `canceling` |
| `canceled`  |
| `deleting`  |
| `deleted`   |

## codersdk.WorkspaceTransition

```json
"start"
```

### Properties

#### Enumerated Values

| Value    |
|----------|
| `start`  |
| `stop`   |
| `delete` |

## codersdk.WorkspacesResponse

```json
{
  "count": 0,
  "workspaces": [
    {
      "allow_renames": true,
      "automatic_updates": "always",
      "autostart_schedule": "string",
      "created_at": "2019-08-24T14:15:22Z",
      "deleting_at": "2019-08-24T14:15:22Z",
      "dormant_at": "2019-08-24T14:15:22Z",
      "favorite": true,
      "health": {
        "failing_agents": [
          "497f6eca-6276-4993-bfeb-53cbbbba6f08"
        ],
        "healthy": false
      },
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "last_used_at": "2019-08-24T14:15:22Z",
      "latest_app_status": {
        "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
        "app_id": "affd1d10-9538-4fc8-9e0b-4594a28c1335",
        "created_at": "2019-08-24T14:15:22Z",
        "icon": "string",
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "message": "string",
        "needs_user_attention": true,
        "state": "working",
        "uri": "string",
        "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
      },
      "latest_build": {
        "build_number": 0,
        "created_at": "2019-08-24T14:15:22Z",
        "daily_cost": 0,
        "deadline": "2019-08-24T14:15:22Z",
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "initiator_id": "06588898-9a84-4b35-ba8f-f9cbd64946f3",
        "initiator_name": "string",
        "job": {
          "available_workers": [
            "497f6eca-6276-4993-bfeb-53cbbbba6f08"
          ],
          "canceled_at": "2019-08-24T14:15:22Z",
          "completed_at": "2019-08-24T14:15:22Z",
          "created_at": "2019-08-24T14:15:22Z",
          "error": "string",
          "error_code": "REQUIRED_TEMPLATE_VARIABLES",
          "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
          "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
          "input": {
            "error": "string",
            "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
            "workspace_build_id": "badaf2eb-96c5-4050-9f1d-db2d39ca5478"
          },
          "metadata": {
            "template_display_name": "string",
            "template_icon": "string",
            "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
            "template_name": "string",
            "template_version_name": "string",
            "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
            "workspace_name": "string"
          },
          "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
          "queue_position": 0,
          "queue_size": 0,
          "started_at": "2019-08-24T14:15:22Z",
          "status": "pending",
          "tags": {
            "property1": "string",
            "property2": "string"
          },
          "type": "template_version_import",
          "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
        },
        "matched_provisioners": {
          "available": 0,
          "count": 0,
          "most_recently_seen": "2019-08-24T14:15:22Z"
        },
        "max_deadline": "2019-08-24T14:15:22Z",
        "reason": "initiator",
        "resources": [
          {
            "agents": [
              {
                "api_version": "string",
                "apps": [
                  {
                    "command": "string",
                    "display_name": "string",
                    "external": true,
                    "health": "disabled",
                    "healthcheck": {},
                    "hidden": true,
                    "icon": "string",
                    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
                    "open_in": "slim-window",
                    "sharing_level": "owner",
                    "slug": "string",
                    "statuses": [],
                    "subdomain": true,
                    "subdomain_name": "string",
                    "url": "string"
                  }
                ],
                "architecture": "string",
                "connection_timeout_seconds": 0,
                "created_at": "2019-08-24T14:15:22Z",
                "directory": "string",
                "disconnected_at": "2019-08-24T14:15:22Z",
                "display_apps": [
                  "vscode"
                ],
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
                "log_sources": [
                  {
                    "created_at": "2019-08-24T14:15:22Z",
                    "display_name": "string",
                    "icon": "string",
                    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
                    "workspace_agent_id": "7ad2e618-fea7-4c1a-b70a-f501566a72f1"
                  }
                ],
                "logs_length": 0,
                "logs_overflowed": true,
                "name": "string",
                "operating_system": "string",
                "ready_at": "2019-08-24T14:15:22Z",
                "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
                "scripts": [
                  {
                    "cron": "string",
                    "display_name": "string",
                    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
                    "log_path": "string",
                    "log_source_id": "4197ab25-95cf-4b91-9c78-f7f2af5d353a",
                    "run_on_start": true,
                    "run_on_stop": true,
                    "script": "string",
                    "start_blocks_login": true,
                    "timeout": 0
                  }
                ],
                "started_at": "2019-08-24T14:15:22Z",
                "startup_script_behavior": "blocking",
                "status": "connecting",
                "subsystems": [
                  "envbox"
                ],
                "troubleshooting_url": "string",
                "updated_at": "2019-08-24T14:15:22Z",
                "version": "string"
              }
            ],
            "created_at": "2019-08-24T14:15:22Z",
            "daily_cost": 0,
            "hide": true,
            "icon": "string",
            "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
            "job_id": "453bd7d7-5355-4d6d-a38e-d9e7eb218c3f",
            "metadata": [
              {
                "key": "string",
                "sensitive": true,
                "value": "string"
              }
            ],
            "name": "string",
            "type": "string",
            "workspace_transition": "start"
          }
        ],
        "status": "pending",
        "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
        "template_version_name": "string",
        "transition": "start",
        "updated_at": "2019-08-24T14:15:22Z",
        "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
        "workspace_name": "string",
        "workspace_owner_avatar_url": "string",
        "workspace_owner_id": "e7078695-5279-4c86-8774-3ac2367a2fc7",
        "workspace_owner_name": "string"
      },
      "name": "string",
      "next_start_at": "2019-08-24T14:15:22Z",
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "organization_name": "string",
      "outdated": true,
      "owner_avatar_url": "string",
      "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
      "owner_name": "string",
      "template_active_version_id": "b0da9c29-67d8-4c87-888c-bafe356f7f3c",
      "template_allow_user_cancel_workspace_jobs": true,
      "template_display_name": "string",
      "template_icon": "string",
      "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
      "template_name": "string",
      "template_require_active_version": true,
      "ttl_ms": 0,
      "updated_at": "2019-08-24T14:15:22Z"
    }
  ]
}
```

### Properties

| Name         | Type                                              | Required | Restrictions | Description |
|--------------|---------------------------------------------------|----------|--------------|-------------|
| `count`      | integer                                           | false    |              |             |
| `workspaces` | array of [codersdk.Workspace](#codersdkworkspace) | false    |              |             |

## derp.BytesSentRecv

```json
{
  "key": {},
  "recv": 0,
  "sent": 0
}
```

### Properties

| Name   | Type                             | Required | Restrictions | Description                                                          |
|--------|----------------------------------|----------|--------------|----------------------------------------------------------------------|
| `key`  | [key.NodePublic](#keynodepublic) | false    |              | Key is the public key of the client which sent/received these bytes. |
| `recv` | integer                          | false    |              |                                                                      |
| `sent` | integer                          | false    |              |                                                                      |

## derp.ServerInfoMessage

```json
{
  "tokenBucketBytesBurst": 0,
  "tokenBucketBytesPerSecond": 0
}
```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|`tokenBucketBytesBurst`|integer|false||Tokenbucketbytesburst is how many bytes the server will allow to burst, temporarily violating TokenBucketBytesPerSecond.
Zero means unspecified. There might be a limit, but the client need not try to respect it.|
|`tokenBucketBytesPerSecond`|integer|false||Tokenbucketbytespersecond is how many bytes per second the server says it will accept, including all framing bytes.
Zero means unspecified. There might be a limit, but the client need not try to respect it.|

## health.Code

```json
"EUNKNOWN"
```

### Properties

#### Enumerated Values

| Value      |
|------------|
| `EUNKNOWN` |
| `EWP01`    |
| `EWP02`    |
| `EWP04`    |
| `EDB01`    |
| `EDB02`    |
| `EWS01`    |
| `EWS02`    |
| `EWS03`    |
| `EACS01`   |
| `EACS02`   |
| `EACS03`   |
| `EACS04`   |
| `EDERP01`  |
| `EDERP02`  |
| `EPD01`    |
| `EPD02`    |
| `EPD03`    |

## health.Message

```json
{
  "code": "EUNKNOWN",
  "message": "string"
}
```

### Properties

| Name      | Type                       | Required | Restrictions | Description |
|-----------|----------------------------|----------|--------------|-------------|
| `code`    | [health.Code](#healthcode) | false    |              |             |
| `message` | string                     | false    |              |             |

## health.Severity

```json
"ok"
```

### Properties

#### Enumerated Values

| Value     |
|-----------|
| `ok`      |
| `warning` |
| `error`   |

## healthsdk.AccessURLReport

```json
{
  "access_url": "string",
  "dismissed": true,
  "error": "string",
  "healthy": true,
  "healthz_response": "string",
  "reachable": true,
  "severity": "ok",
  "status_code": 0,
  "warnings": [
    {
      "code": "EUNKNOWN",
      "message": "string"
    }
  ]
}
```

### Properties

| Name               | Type                                      | Required | Restrictions | Description                                                                                 |
|--------------------|-------------------------------------------|----------|--------------|---------------------------------------------------------------------------------------------|
| `access_url`       | string                                    | false    |              |                                                                                             |
| `dismissed`        | boolean                                   | false    |              |                                                                                             |
| `error`            | string                                    | false    |              |                                                                                             |
| `healthy`          | boolean                                   | false    |              | Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead. |
| `healthz_response` | string                                    | false    |              |                                                                                             |
| `reachable`        | boolean                                   | false    |              |                                                                                             |
| `severity`         | [health.Severity](#healthseverity)        | false    |              |                                                                                             |
| `status_code`      | integer                                   | false    |              |                                                                                             |
| `warnings`         | array of [health.Message](#healthmessage) | false    |              |                                                                                             |

#### Enumerated Values

| Property   | Value     |
|------------|-----------|
| `severity` | `ok`      |
| `severity` | `warning` |
| `severity` | `error`   |

## healthsdk.DERPHealthReport

```json
{
  "dismissed": true,
  "error": "string",
  "healthy": true,
  "netcheck": {
    "captivePortal": "string",
    "globalV4": "string",
    "globalV6": "string",
    "hairPinning": "string",
    "icmpv4": true,
    "ipv4": true,
    "ipv4CanSend": true,
    "ipv6": true,
    "ipv6CanSend": true,
    "mappingVariesByDestIP": "string",
    "oshasIPv6": true,
    "pcp": "string",
    "pmp": "string",
    "preferredDERP": 0,
    "regionLatency": {
      "property1": 0,
      "property2": 0
    },
    "regionV4Latency": {
      "property1": 0,
      "property2": 0
    },
    "regionV6Latency": {
      "property1": 0,
      "property2": 0
    },
    "udp": true,
    "upnP": "string"
  },
  "netcheck_err": "string",
  "netcheck_logs": [
    "string"
  ],
  "regions": {
    "property1": {
      "error": "string",
      "healthy": true,
      "node_reports": [
        {
          "can_exchange_messages": true,
          "client_errs": [
            [
              "string"
            ]
          ],
          "client_logs": [
            [
              "string"
            ]
          ],
          "error": "string",
          "healthy": true,
          "node": {
            "canPort80": true,
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
          },
          "node_info": {
            "tokenBucketBytesBurst": 0,
            "tokenBucketBytesPerSecond": 0
          },
          "round_trip_ping": "string",
          "round_trip_ping_ms": 0,
          "severity": "ok",
          "stun": {
            "canSTUN": true,
            "enabled": true,
            "error": "string"
          },
          "uses_websocket": true,
          "warnings": [
            {
              "code": "EUNKNOWN",
              "message": "string"
            }
          ]
        }
      ],
      "region": {
        "avoid": true,
        "embeddedRelay": true,
        "nodes": [
          {
            "canPort80": true,
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
      "severity": "ok",
      "warnings": [
        {
          "code": "EUNKNOWN",
          "message": "string"
        }
      ]
    },
    "property2": {
      "error": "string",
      "healthy": true,
      "node_reports": [
        {
          "can_exchange_messages": true,
          "client_errs": [
            [
              "string"
            ]
          ],
          "client_logs": [
            [
              "string"
            ]
          ],
          "error": "string",
          "healthy": true,
          "node": {
            "canPort80": true,
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
          },
          "node_info": {
            "tokenBucketBytesBurst": 0,
            "tokenBucketBytesPerSecond": 0
          },
          "round_trip_ping": "string",
          "round_trip_ping_ms": 0,
          "severity": "ok",
          "stun": {
            "canSTUN": true,
            "enabled": true,
            "error": "string"
          },
          "uses_websocket": true,
          "warnings": [
            {
              "code": "EUNKNOWN",
              "message": "string"
            }
          ]
        }
      ],
      "region": {
        "avoid": true,
        "embeddedRelay": true,
        "nodes": [
          {
            "canPort80": true,
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
      "severity": "ok",
      "warnings": [
        {
          "code": "EUNKNOWN",
          "message": "string"
        }
      ]
    }
  },
  "severity": "ok",
  "warnings": [
    {
      "code": "EUNKNOWN",
      "message": "string"
    }
  ]
}
```

### Properties

| Name               | Type                                                     | Required | Restrictions | Description                                                                                 |
|--------------------|----------------------------------------------------------|----------|--------------|---------------------------------------------------------------------------------------------|
| `dismissed`        | boolean                                                  | false    |              |                                                                                             |
| `error`            | string                                                   | false    |              |                                                                                             |
| `healthy`          | boolean                                                  | false    |              | Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead. |
| `netcheck`         | [netcheck.Report](#netcheckreport)                       | false    |              |                                                                                             |
| `netcheck_err`     | string                                                   | false    |              |                                                                                             |
| `netcheck_logs`    | array of string                                          | false    |              |                                                                                             |
| `regions`          | object                                                   | false    |              |                                                                                             |
| » `[any property]` | [healthsdk.DERPRegionReport](#healthsdkderpregionreport) | false    |              |                                                                                             |
| `severity`         | [health.Severity](#healthseverity)                       | false    |              |                                                                                             |
| `warnings`         | array of [health.Message](#healthmessage)                | false    |              |                                                                                             |

#### Enumerated Values

| Property   | Value     |
|------------|-----------|
| `severity` | `ok`      |
| `severity` | `warning` |
| `severity` | `error`   |

## healthsdk.DERPNodeReport

```json
{
  "can_exchange_messages": true,
  "client_errs": [
    [
      "string"
    ]
  ],
  "client_logs": [
    [
      "string"
    ]
  ],
  "error": "string",
  "healthy": true,
  "node": {
    "canPort80": true,
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
  },
  "node_info": {
    "tokenBucketBytesBurst": 0,
    "tokenBucketBytesPerSecond": 0
  },
  "round_trip_ping": "string",
  "round_trip_ping_ms": 0,
  "severity": "ok",
  "stun": {
    "canSTUN": true,
    "enabled": true,
    "error": "string"
  },
  "uses_websocket": true,
  "warnings": [
    {
      "code": "EUNKNOWN",
      "message": "string"
    }
  ]
}
```

### Properties

| Name                    | Type                                             | Required | Restrictions | Description                                                                                 |
|-------------------------|--------------------------------------------------|----------|--------------|---------------------------------------------------------------------------------------------|
| `can_exchange_messages` | boolean                                          | false    |              |                                                                                             |
| `client_errs`           | array of array                                   | false    |              |                                                                                             |
| `client_logs`           | array of array                                   | false    |              |                                                                                             |
| `error`                 | string                                           | false    |              |                                                                                             |
| `healthy`               | boolean                                          | false    |              | Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead. |
| `node`                  | [tailcfg.DERPNode](#tailcfgderpnode)             | false    |              |                                                                                             |
| `node_info`             | [derp.ServerInfoMessage](#derpserverinfomessage) | false    |              |                                                                                             |
| `round_trip_ping`       | string                                           | false    |              |                                                                                             |
| `round_trip_ping_ms`    | integer                                          | false    |              |                                                                                             |
| `severity`              | [health.Severity](#healthseverity)               | false    |              |                                                                                             |
| `stun`                  | [healthsdk.STUNReport](#healthsdkstunreport)     | false    |              |                                                                                             |
| `uses_websocket`        | boolean                                          | false    |              |                                                                                             |
| `warnings`              | array of [health.Message](#healthmessage)        | false    |              |                                                                                             |

#### Enumerated Values

| Property   | Value     |
|------------|-----------|
| `severity` | `ok`      |
| `severity` | `warning` |
| `severity` | `error`   |

## healthsdk.DERPRegionReport

```json
{
  "error": "string",
  "healthy": true,
  "node_reports": [
    {
      "can_exchange_messages": true,
      "client_errs": [
        [
          "string"
        ]
      ],
      "client_logs": [
        [
          "string"
        ]
      ],
      "error": "string",
      "healthy": true,
      "node": {
        "canPort80": true,
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
      },
      "node_info": {
        "tokenBucketBytesBurst": 0,
        "tokenBucketBytesPerSecond": 0
      },
      "round_trip_ping": "string",
      "round_trip_ping_ms": 0,
      "severity": "ok",
      "stun": {
        "canSTUN": true,
        "enabled": true,
        "error": "string"
      },
      "uses_websocket": true,
      "warnings": [
        {
          "code": "EUNKNOWN",
          "message": "string"
        }
      ]
    }
  ],
  "region": {
    "avoid": true,
    "embeddedRelay": true,
    "nodes": [
      {
        "canPort80": true,
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
  "severity": "ok",
  "warnings": [
    {
      "code": "EUNKNOWN",
      "message": "string"
    }
  ]
}
```

### Properties

| Name           | Type                                                          | Required | Restrictions | Description                                                                                 |
|----------------|---------------------------------------------------------------|----------|--------------|---------------------------------------------------------------------------------------------|
| `error`        | string                                                        | false    |              |                                                                                             |
| `healthy`      | boolean                                                       | false    |              | Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead. |
| `node_reports` | array of [healthsdk.DERPNodeReport](#healthsdkderpnodereport) | false    |              |                                                                                             |
| `region`       | [tailcfg.DERPRegion](#tailcfgderpregion)                      | false    |              |                                                                                             |
| `severity`     | [health.Severity](#healthseverity)                            | false    |              |                                                                                             |
| `warnings`     | array of [health.Message](#healthmessage)                     | false    |              |                                                                                             |

#### Enumerated Values

| Property   | Value     |
|------------|-----------|
| `severity` | `ok`      |
| `severity` | `warning` |
| `severity` | `error`   |

## healthsdk.DatabaseReport

```json
{
  "dismissed": true,
  "error": "string",
  "healthy": true,
  "latency": "string",
  "latency_ms": 0,
  "reachable": true,
  "severity": "ok",
  "threshold_ms": 0,
  "warnings": [
    {
      "code": "EUNKNOWN",
      "message": "string"
    }
  ]
}
```

### Properties

| Name           | Type                                      | Required | Restrictions | Description                                                                                 |
|----------------|-------------------------------------------|----------|--------------|---------------------------------------------------------------------------------------------|
| `dismissed`    | boolean                                   | false    |              |                                                                                             |
| `error`        | string                                    | false    |              |                                                                                             |
| `healthy`      | boolean                                   | false    |              | Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead. |
| `latency`      | string                                    | false    |              |                                                                                             |
| `latency_ms`   | integer                                   | false    |              |                                                                                             |
| `reachable`    | boolean                                   | false    |              |                                                                                             |
| `severity`     | [health.Severity](#healthseverity)        | false    |              |                                                                                             |
| `threshold_ms` | integer                                   | false    |              |                                                                                             |
| `warnings`     | array of [health.Message](#healthmessage) | false    |              |                                                                                             |

#### Enumerated Values

| Property   | Value     |
|------------|-----------|
| `severity` | `ok`      |
| `severity` | `warning` |
| `severity` | `error`   |

## healthsdk.HealthSection

```json
"DERP"
```

### Properties

#### Enumerated Values

| Value                |
|----------------------|
| `DERP`               |
| `AccessURL`          |
| `Websocket`          |
| `Database`           |
| `WorkspaceProxy`     |
| `ProvisionerDaemons` |

## healthsdk.HealthSettings

```json
{
  "dismissed_healthchecks": [
    "DERP"
  ]
}
```

### Properties

| Name                     | Type                                                        | Required | Restrictions | Description |
|--------------------------|-------------------------------------------------------------|----------|--------------|-------------|
| `dismissed_healthchecks` | array of [healthsdk.HealthSection](#healthsdkhealthsection) | false    |              |             |

## healthsdk.HealthcheckReport

```json
{
  "access_url": {
    "access_url": "string",
    "dismissed": true,
    "error": "string",
    "healthy": true,
    "healthz_response": "string",
    "reachable": true,
    "severity": "ok",
    "status_code": 0,
    "warnings": [
      {
        "code": "EUNKNOWN",
        "message": "string"
      }
    ]
  },
  "coder_version": "string",
  "database": {
    "dismissed": true,
    "error": "string",
    "healthy": true,
    "latency": "string",
    "latency_ms": 0,
    "reachable": true,
    "severity": "ok",
    "threshold_ms": 0,
    "warnings": [
      {
        "code": "EUNKNOWN",
        "message": "string"
      }
    ]
  },
  "derp": {
    "dismissed": true,
    "error": "string",
    "healthy": true,
    "netcheck": {
      "captivePortal": "string",
      "globalV4": "string",
      "globalV6": "string",
      "hairPinning": "string",
      "icmpv4": true,
      "ipv4": true,
      "ipv4CanSend": true,
      "ipv6": true,
      "ipv6CanSend": true,
      "mappingVariesByDestIP": "string",
      "oshasIPv6": true,
      "pcp": "string",
      "pmp": "string",
      "preferredDERP": 0,
      "regionLatency": {
        "property1": 0,
        "property2": 0
      },
      "regionV4Latency": {
        "property1": 0,
        "property2": 0
      },
      "regionV6Latency": {
        "property1": 0,
        "property2": 0
      },
      "udp": true,
      "upnP": "string"
    },
    "netcheck_err": "string",
    "netcheck_logs": [
      "string"
    ],
    "regions": {
      "property1": {
        "error": "string",
        "healthy": true,
        "node_reports": [
          {
            "can_exchange_messages": true,
            "client_errs": [
              [
                "string"
              ]
            ],
            "client_logs": [
              [
                "string"
              ]
            ],
            "error": "string",
            "healthy": true,
            "node": {
              "canPort80": true,
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
            },
            "node_info": {
              "tokenBucketBytesBurst": 0,
              "tokenBucketBytesPerSecond": 0
            },
            "round_trip_ping": "string",
            "round_trip_ping_ms": 0,
            "severity": "ok",
            "stun": {
              "canSTUN": true,
              "enabled": true,
              "error": "string"
            },
            "uses_websocket": true,
            "warnings": [
              {
                "code": "EUNKNOWN",
                "message": "string"
              }
            ]
          }
        ],
        "region": {
          "avoid": true,
          "embeddedRelay": true,
          "nodes": [
            {
              "canPort80": true,
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
        "severity": "ok",
        "warnings": [
          {
            "code": "EUNKNOWN",
            "message": "string"
          }
        ]
      },
      "property2": {
        "error": "string",
        "healthy": true,
        "node_reports": [
          {
            "can_exchange_messages": true,
            "client_errs": [
              [
                "string"
              ]
            ],
            "client_logs": [
              [
                "string"
              ]
            ],
            "error": "string",
            "healthy": true,
            "node": {
              "canPort80": true,
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
            },
            "node_info": {
              "tokenBucketBytesBurst": 0,
              "tokenBucketBytesPerSecond": 0
            },
            "round_trip_ping": "string",
            "round_trip_ping_ms": 0,
            "severity": "ok",
            "stun": {
              "canSTUN": true,
              "enabled": true,
              "error": "string"
            },
            "uses_websocket": true,
            "warnings": [
              {
                "code": "EUNKNOWN",
                "message": "string"
              }
            ]
          }
        ],
        "region": {
          "avoid": true,
          "embeddedRelay": true,
          "nodes": [
            {
              "canPort80": true,
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
        "severity": "ok",
        "warnings": [
          {
            "code": "EUNKNOWN",
            "message": "string"
          }
        ]
      }
    },
    "severity": "ok",
    "warnings": [
      {
        "code": "EUNKNOWN",
        "message": "string"
      }
    ]
  },
  "healthy": true,
  "provisioner_daemons": {
    "dismissed": true,
    "error": "string",
    "items": [
      {
        "provisioner_daemon": {
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
        },
        "warnings": [
          {
            "code": "EUNKNOWN",
            "message": "string"
          }
        ]
      }
    ],
    "severity": "ok",
    "warnings": [
      {
        "code": "EUNKNOWN",
        "message": "string"
      }
    ]
  },
  "severity": "ok",
  "time": "2019-08-24T14:15:22Z",
  "websocket": {
    "body": "string",
    "code": 0,
    "dismissed": true,
    "error": "string",
    "healthy": true,
    "severity": "ok",
    "warnings": [
      {
        "code": "EUNKNOWN",
        "message": "string"
      }
    ]
  },
  "workspace_proxy": {
    "dismissed": true,
    "error": "string",
    "healthy": true,
    "severity": "ok",
    "warnings": [
      {
        "code": "EUNKNOWN",
        "message": "string"
      }
    ],
    "workspace_proxies": {
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
  }
}
```

### Properties

| Name                  | Type                                                                     | Required | Restrictions | Description                                                                         |
|-----------------------|--------------------------------------------------------------------------|----------|--------------|-------------------------------------------------------------------------------------|
| `access_url`          | [healthsdk.AccessURLReport](#healthsdkaccessurlreport)                   | false    |              |                                                                                     |
| `coder_version`       | string                                                                   | false    |              | The Coder version of the server that the report was generated on.                   |
| `database`            | [healthsdk.DatabaseReport](#healthsdkdatabasereport)                     | false    |              |                                                                                     |
| `derp`                | [healthsdk.DERPHealthReport](#healthsdkderphealthreport)                 | false    |              |                                                                                     |
| `healthy`             | boolean                                                                  | false    |              | Healthy is true if the report returns no errors. Deprecated: use `Severity` instead |
| `provisioner_daemons` | [healthsdk.ProvisionerDaemonsReport](#healthsdkprovisionerdaemonsreport) | false    |              |                                                                                     |
| `severity`            | [health.Severity](#healthseverity)                                       | false    |              | Severity indicates the status of Coder health.                                      |
| `time`                | string                                                                   | false    |              | Time is the time the report was generated at.                                       |
| `websocket`           | [healthsdk.WebsocketReport](#healthsdkwebsocketreport)                   | false    |              |                                                                                     |
| `workspace_proxy`     | [healthsdk.WorkspaceProxyReport](#healthsdkworkspaceproxyreport)         | false    |              |                                                                                     |

#### Enumerated Values

| Property   | Value     |
|------------|-----------|
| `severity` | `ok`      |
| `severity` | `warning` |
| `severity` | `error`   |

## healthsdk.ProvisionerDaemonsReport

```json
{
  "dismissed": true,
  "error": "string",
  "items": [
    {
      "provisioner_daemon": {
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
      },
      "warnings": [
        {
          "code": "EUNKNOWN",
          "message": "string"
        }
      ]
    }
  ],
  "severity": "ok",
  "warnings": [
    {
      "code": "EUNKNOWN",
      "message": "string"
    }
  ]
}
```

### Properties

| Name        | Type                                                                                      | Required | Restrictions | Description |
|-------------|-------------------------------------------------------------------------------------------|----------|--------------|-------------|
| `dismissed` | boolean                                                                                   | false    |              |             |
| `error`     | string                                                                                    | false    |              |             |
| `items`     | array of [healthsdk.ProvisionerDaemonsReportItem](#healthsdkprovisionerdaemonsreportitem) | false    |              |             |
| `severity`  | [health.Severity](#healthseverity)                                                        | false    |              |             |
| `warnings`  | array of [health.Message](#healthmessage)                                                 | false    |              |             |

#### Enumerated Values

| Property   | Value     |
|------------|-----------|
| `severity` | `ok`      |
| `severity` | `warning` |
| `severity` | `error`   |

## healthsdk.ProvisionerDaemonsReportItem

```json
{
  "provisioner_daemon": {
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
  },
  "warnings": [
    {
      "code": "EUNKNOWN",
      "message": "string"
    }
  ]
}
```

### Properties

| Name                 | Type                                                     | Required | Restrictions | Description |
|----------------------|----------------------------------------------------------|----------|--------------|-------------|
| `provisioner_daemon` | [codersdk.ProvisionerDaemon](#codersdkprovisionerdaemon) | false    |              |             |
| `warnings`           | array of [health.Message](#healthmessage)                | false    |              |             |

## healthsdk.STUNReport

```json
{
  "canSTUN": true,
  "enabled": true,
  "error": "string"
}
```

### Properties

| Name      | Type    | Required | Restrictions | Description |
|-----------|---------|----------|--------------|-------------|
| `canSTUN` | boolean | false    |              |             |
| `enabled` | boolean | false    |              |             |
| `error`   | string  | false    |              |             |

## healthsdk.UpdateHealthSettings

```json
{
  "dismissed_healthchecks": [
    "DERP"
  ]
}
```

### Properties

| Name                     | Type                                                        | Required | Restrictions | Description |
|--------------------------|-------------------------------------------------------------|----------|--------------|-------------|
| `dismissed_healthchecks` | array of [healthsdk.HealthSection](#healthsdkhealthsection) | false    |              |             |

## healthsdk.WebsocketReport

```json
{
  "body": "string",
  "code": 0,
  "dismissed": true,
  "error": "string",
  "healthy": true,
  "severity": "ok",
  "warnings": [
    {
      "code": "EUNKNOWN",
      "message": "string"
    }
  ]
}
```

### Properties

| Name        | Type                                      | Required | Restrictions | Description                                                                                 |
|-------------|-------------------------------------------|----------|--------------|---------------------------------------------------------------------------------------------|
| `body`      | string                                    | false    |              |                                                                                             |
| `code`      | integer                                   | false    |              |                                                                                             |
| `dismissed` | boolean                                   | false    |              |                                                                                             |
| `error`     | string                                    | false    |              |                                                                                             |
| `healthy`   | boolean                                   | false    |              | Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead. |
| `severity`  | [health.Severity](#healthseverity)        | false    |              |                                                                                             |
| `warnings`  | array of [health.Message](#healthmessage) | false    |              |                                                                                             |

#### Enumerated Values

| Property   | Value     |
|------------|-----------|
| `severity` | `ok`      |
| `severity` | `warning` |
| `severity` | `error`   |

## healthsdk.WorkspaceProxyReport

```json
{
  "dismissed": true,
  "error": "string",
  "healthy": true,
  "severity": "ok",
  "warnings": [
    {
      "code": "EUNKNOWN",
      "message": "string"
    }
  ],
  "workspace_proxies": {
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
}
```

### Properties

| Name                | Type                                                                                                 | Required | Restrictions | Description                                                                                 |
|---------------------|------------------------------------------------------------------------------------------------------|----------|--------------|---------------------------------------------------------------------------------------------|
| `dismissed`         | boolean                                                                                              | false    |              |                                                                                             |
| `error`             | string                                                                                               | false    |              |                                                                                             |
| `healthy`           | boolean                                                                                              | false    |              | Healthy is deprecated and left for backward compatibility purposes, use `Severity` instead. |
| `severity`          | [health.Severity](#healthseverity)                                                                   | false    |              |                                                                                             |
| `warnings`          | array of [health.Message](#healthmessage)                                                            | false    |              |                                                                                             |
| `workspace_proxies` | [codersdk.RegionsResponse-codersdk_WorkspaceProxy](#codersdkregionsresponse-codersdk_workspaceproxy) | false    |              |                                                                                             |

#### Enumerated Values

| Property   | Value     |
|------------|-----------|
| `severity` | `ok`      |
| `severity` | `warning` |
| `severity` | `error`   |

## key.NodePublic

```json
{}
```

### Properties

None

## netcheck.Report

```json
{
  "captivePortal": "string",
  "globalV4": "string",
  "globalV6": "string",
  "hairPinning": "string",
  "icmpv4": true,
  "ipv4": true,
  "ipv4CanSend": true,
  "ipv6": true,
  "ipv6CanSend": true,
  "mappingVariesByDestIP": "string",
  "oshasIPv6": true,
  "pcp": "string",
  "pmp": "string",
  "preferredDERP": 0,
  "regionLatency": {
    "property1": 0,
    "property2": 0
  },
  "regionV4Latency": {
    "property1": 0,
    "property2": 0
  },
  "regionV6Latency": {
    "property1": 0,
    "property2": 0
  },
  "udp": true,
  "upnP": "string"
}
```

### Properties

| Name                    | Type    | Required | Restrictions | Description                                                                                                                        |
|-------------------------|---------|----------|--------------|------------------------------------------------------------------------------------------------------------------------------------|
| `captivePortal`         | string  | false    |              | Captiveportal is set when we think there's a captive portal that is intercepting HTTP traffic.                                     |
| `globalV4`              | string  | false    |              | ip:port of global IPv4                                                                                                             |
| `globalV6`              | string  | false    |              | [ip]:port of global IPv6                                                                                                           |
| `hairPinning`           | string  | false    |              | Hairpinning is whether the router supports communicating between two local devices through the NATted public IP address (on IPv4). |
| `icmpv4`                | boolean | false    |              | an ICMPv4 round trip completed                                                                                                     |
| `ipv4`                  | boolean | false    |              | an IPv4 STUN round trip completed                                                                                                  |
| `ipv4CanSend`           | boolean | false    |              | an IPv4 packet was able to be sent                                                                                                 |
| `ipv6`                  | boolean | false    |              | an IPv6 STUN round trip completed                                                                                                  |
| `ipv6CanSend`           | boolean | false    |              | an IPv6 packet was able to be sent                                                                                                 |
| `mappingVariesByDestIP` | string  | false    |              | Mappingvariesbydestip is whether STUN results depend which STUN server you're talking to (on IPv4).                                |
| `oshasIPv6`             | boolean | false    |              | could bind a socket to ::1                                                                                                         |
| `pcp`                   | string  | false    |              | Pcp is whether PCP appears present on the LAN. Empty means not checked.                                                            |
| `pmp`                   | string  | false    |              | Pmp is whether NAT-PMP appears present on the LAN. Empty means not checked.                                                        |
| `preferredDERP`         | integer | false    |              | or 0 for unknown                                                                                                                   |
| `regionLatency`         | object  | false    |              | keyed by DERP Region ID                                                                                                            |
| » `[any property]`      | integer | false    |              |                                                                                                                                    |
| `regionV4Latency`       | object  | false    |              | keyed by DERP Region ID                                                                                                            |
| » `[any property]`      | integer | false    |              |                                                                                                                                    |
| `regionV6Latency`       | object  | false    |              | keyed by DERP Region ID                                                                                                            |
| » `[any property]`      | integer | false    |              |                                                                                                                                    |
| `udp`                   | boolean | false    |              | a UDP STUN round trip completed                                                                                                    |
| `upnP`                  | string  | false    |              | Upnp is whether UPnP appears present on the LAN. Empty means not checked.                                                          |

## oauth2.Token

```json
{
  "access_token": "string",
  "expires_in": 0,
  "expiry": "string",
  "refresh_token": "string",
  "token_type": "string"
}
```

### Properties

| Name           | Type    | Required | Restrictions | Description                                                                                                                                                                                                                                                                 |
|----------------|---------|----------|--------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `access_token` | string  | false    |              | Access token is the token that authorizes and authenticates the requests.                                                                                                                                                                                                   |
| `expires_in`   | integer | false    |              | Expires in is the OAuth2 wire format "expires_in" field, which specifies how many seconds later the token expires, relative to an unknown time base approximately around "now". It is the application's responsibility to populate `Expiry` from `ExpiresIn` when required. |
|`expiry`|string|false||Expiry is the optional expiration time of the access token.
If zero, TokenSource implementations will reuse the same token forever and RefreshToken or equivalent mechanisms for that TokenSource will not be used.|
|`refresh_token`|string|false||Refresh token is a token that's used by the application (as opposed to the user) to refresh the access token if it expires.|
|`token_type`|string|false||Token type is the type of token. The Type method returns either this or "Bearer", the default.|

## regexp.Regexp

```json
{}
```

### Properties

None

## serpent.Annotations

```json
{
  "property1": "string",
  "property2": "string"
}
```

### Properties

| Name             | Type   | Required | Restrictions | Description |
|------------------|--------|----------|--------------|-------------|
| `[any property]` | string | false    |              |             |

## serpent.Group

```json
{
  "description": "string",
  "name": "string",
  "parent": {
    "description": "string",
    "name": "string",
    "parent": {},
    "yaml": "string"
  },
  "yaml": "string"
}
```

### Properties

| Name          | Type                           | Required | Restrictions | Description |
|---------------|--------------------------------|----------|--------------|-------------|
| `description` | string                         | false    |              |             |
| `name`        | string                         | false    |              |             |
| `parent`      | [serpent.Group](#serpentgroup) | false    |              |             |
| `yaml`        | string                         | false    |              |             |

## serpent.HostPort

```json
{
  "host": "string",
  "port": "string"
}
```

### Properties

| Name   | Type   | Required | Restrictions | Description |
|--------|--------|----------|--------------|-------------|
| `host` | string | false    |              |             |
| `port` | string | false    |              |             |

## serpent.Option

```json
{
  "annotations": {
    "property1": "string",
    "property2": "string"
  },
  "default": "string",
  "description": "string",
  "env": "string",
  "flag": "string",
  "flag_shorthand": "string",
  "group": {
    "description": "string",
    "name": "string",
    "parent": {
      "description": "string",
      "name": "string",
      "parent": {},
      "yaml": "string"
    },
    "yaml": "string"
  },
  "hidden": true,
  "name": "string",
  "required": true,
  "use_instead": [
    {
      "annotations": {
        "property1": "string",
        "property2": "string"
      },
      "default": "string",
      "description": "string",
      "env": "string",
      "flag": "string",
      "flag_shorthand": "string",
      "group": {
        "description": "string",
        "name": "string",
        "parent": {
          "description": "string",
          "name": "string",
          "parent": {},
          "yaml": "string"
        },
        "yaml": "string"
      },
      "hidden": true,
      "name": "string",
      "required": true,
      "use_instead": [],
      "value": null,
      "value_source": "",
      "yaml": "string"
    }
  ],
  "value": null,
  "value_source": "",
  "yaml": "string"
}
```

### Properties

| Name             | Type                                       | Required | Restrictions | Description                                                                                                                                        |
|------------------|--------------------------------------------|----------|--------------|----------------------------------------------------------------------------------------------------------------------------------------------------|
| `annotations`    | [serpent.Annotations](#serpentannotations) | false    |              | Annotations enable extensions to serpent higher up in the stack. It's useful for help formatting and documentation generation.                     |
| `default`        | string                                     | false    |              | Default is parsed into Value if set.                                                                                                               |
| `description`    | string                                     | false    |              |                                                                                                                                                    |
| `env`            | string                                     | false    |              | Env is the environment variable used to configure this option. If unset, environment configuring is disabled.                                      |
| `flag`           | string                                     | false    |              | Flag is the long name of the flag used to configure this option. If unset, flag configuring is disabled.                                           |
| `flag_shorthand` | string                                     | false    |              | Flag shorthand is the one-character shorthand for the flag. If unset, no shorthand is used.                                                        |
| `group`          | [serpent.Group](#serpentgroup)             | false    |              | Group is a group hierarchy that helps organize this option in help, configs and other documentation.                                               |
| `hidden`         | boolean                                    | false    |              |                                                                                                                                                    |
| `name`           | string                                     | false    |              |                                                                                                                                                    |
| `required`       | boolean                                    | false    |              | Required means this value must be set by some means. It requires `ValueSource != ValueSourceNone` If `Default` is set, then `Required` is ignored. |
| `use_instead`    | array of [serpent.Option](#serpentoption)  | false    |              | Use instead is a list of options that should be used instead of this one. The field is used to generate a deprecation warning.                     |
| `value`          | any                                        | false    |              | Value includes the types listed in values.go.                                                                                                      |
| `value_source`   | [serpent.ValueSource](#serpentvaluesource) | false    |              |                                                                                                                                                    |
| `yaml`           | string                                     | false    |              | Yaml is the YAML key used to configure this option. If unset, YAML configuring is disabled.                                                        |

## serpent.Regexp

```json
{}
```

### Properties

None

## serpent.Struct-array_codersdk_ExternalAuthConfig

```json
{
  "value": [
    {
      "app_install_url": "string",
      "app_installations_url": "string",
      "auth_url": "string",
      "client_id": "string",
      "device_code_url": "string",
      "device_flow": true,
      "display_icon": "string",
      "display_name": "string",
      "id": "string",
      "no_refresh": true,
      "regex": "string",
      "scopes": [
        "string"
      ],
      "token_url": "string",
      "type": "string",
      "validate_url": "string"
    }
  ]
}
```

### Properties

| Name    | Type                                                                | Required | Restrictions | Description |
|---------|---------------------------------------------------------------------|----------|--------------|-------------|
| `value` | array of [codersdk.ExternalAuthConfig](#codersdkexternalauthconfig) | false    |              |             |

## serpent.Struct-array_codersdk_LinkConfig

```json
{
  "value": [
    {
      "icon": "bug",
      "name": "string",
      "target": "string"
    }
  ]
}
```

### Properties

| Name    | Type                                                | Required | Restrictions | Description |
|---------|-----------------------------------------------------|----------|--------------|-------------|
| `value` | array of [codersdk.LinkConfig](#codersdklinkconfig) | false    |              |             |

## serpent.URL

```json
{
  "forceQuery": true,
  "fragment": "string",
  "host": "string",
  "omitHost": true,
  "opaque": "string",
  "path": "string",
  "rawFragment": "string",
  "rawPath": "string",
  "rawQuery": "string",
  "scheme": "string",
  "user": {}
}
```

### Properties

| Name          | Type                         | Required | Restrictions | Description                                        |
|---------------|------------------------------|----------|--------------|----------------------------------------------------|
| `forceQuery`  | boolean                      | false    |              | append a query ('?') even if RawQuery is empty     |
| `fragment`    | string                       | false    |              | fragment for references, without '#'               |
| `host`        | string                       | false    |              | host or host:port (see Hostname and Port methods)  |
| `omitHost`    | boolean                      | false    |              | do not emit empty host (authority)                 |
| `opaque`      | string                       | false    |              | encoded opaque data                                |
| `path`        | string                       | false    |              | path (relative paths may omit leading slash)       |
| `rawFragment` | string                       | false    |              | encoded fragment hint (see EscapedFragment method) |
| `rawPath`     | string                       | false    |              | encoded path hint (see EscapedPath method)         |
| `rawQuery`    | string                       | false    |              | encoded query values, without '?'                  |
| `scheme`      | string                       | false    |              |                                                    |
| `user`        | [url.Userinfo](#urluserinfo) | false    |              | username and password information                  |

## serpent.ValueSource

```json
""
```

### Properties

#### Enumerated Values

| Value     |
|-----------|
| ``        |
| `flag`    |
| `env`     |
| `yaml`    |
| `default` |

## tailcfg.DERPHomeParams

```json
{
  "regionScore": {
    "property1": 0,
    "property2": 0
  }
}
```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|`regionScore`|object|false||Regionscore scales latencies of DERP regions by a given scaling factor when determining which region to use as the home ("preferred") DERP. Scores in the range (0, 1) will cause this region to be proportionally more preferred, and scores in the range (1, ∞) will penalize a region.
If a region is not present in this map, it is treated as having a score of 1.0.
Scores should not be 0 or negative; such scores will be ignored.
A nil map means no change from the previous value (if any); an empty non-nil map can be sent to reset all scores back to 1.0.|
|» `[any property]`|number|false|||

## tailcfg.DERPMap

```json
{
  "homeParams": {
    "regionScore": {
      "property1": 0,
      "property2": 0
    }
  },
  "omitDefaultRegions": true,
  "regions": {
    "property1": {
      "avoid": true,
      "embeddedRelay": true,
      "nodes": [
        {
          "canPort80": true,
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
          "canPort80": true,
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
```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|`homeParams`|[tailcfg.DERPHomeParams](#tailcfgderphomeparams)|false||Homeparams if non-nil, is a change in home parameters.
The rest of the DEPRMap fields, if zero, means unchanged.|
|`omitDefaultRegions`|boolean|false||Omitdefaultregions specifies to not use Tailscale's DERP servers, and only use those specified in this DERPMap. If there are none set outside of the defaults, this is a noop.
This field is only meaningful if the Regions map is non-nil (indicating a change).|
|`regions`|object|false||Regions is the set of geographic regions running DERP node(s).
It's keyed by the DERPRegion.RegionID.
The numbers are not necessarily contiguous.|
|» `[any property]`|[tailcfg.DERPRegion](#tailcfgderpregion)|false|||

## tailcfg.DERPNode

```json
{
  "canPort80": true,
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
```

### Properties

| Name        | Type    | Required | Restrictions | Description                                                                                                                                                                                                     |
|-------------|---------|----------|--------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `canPort80` | boolean | false    |              | Canport80 specifies whether this DERP node is accessible over HTTP on port 80 specifically. This is used for captive portal checks.                                                                             |
| `certName`  | string  | false    |              | Certname optionally specifies the expected TLS cert common name. If empty, HostName is used. If CertName is non-empty, HostName is only used for the TCP dial (if IPv4/IPv6 are not present) + TLS ClientHello. |
|`derpport`|integer|false||Derpport optionally provides an alternate TLS port number for the DERP HTTPS server.
If zero, 443 is used.|
|`forceHTTP`|boolean|false||Forcehttp is used by unit tests to force HTTP. It should not be set by users.|
|`hostName`|string|false||Hostname is the DERP node's hostname.
It is required but need not be unique; multiple nodes may have the same HostName but vary in configuration otherwise.|
|`insecureForTests`|boolean|false||Insecurefortests is used by unit tests to disable TLS verification. It should not be set by users.|
|`ipv4`|string|false||Ipv4 optionally forces an IPv4 address to use, instead of using DNS. If empty, A record(s) from DNS lookups of HostName are used. If the string is not an IPv4 address, IPv4 is not used; the conventional string to disable IPv4 (and not use DNS) is "none".|
|`ipv6`|string|false||Ipv6 optionally forces an IPv6 address to use, instead of using DNS. If empty, AAAA record(s) from DNS lookups of HostName are used. If the string is not an IPv6 address, IPv6 is not used; the conventional string to disable IPv6 (and not use DNS) is "none".|
|`name`|string|false||Name is a unique node name (across all regions). It is not a host name. It's typically of the form "1b", "2a", "3b", etc. (region ID + suffix within that region)|
|`regionID`|integer|false||Regionid is the RegionID of the DERPRegion that this node is running in.|
|`stunonly`|boolean|false||Stunonly marks a node as only a STUN server and not a DERP server.|
|`stunport`|integer|false||Port optionally specifies a STUN port to use. Zero means 3478. To disable STUN on this node, use -1.|
|`stuntestIP`|string|false||Stuntestip is used in tests to override the STUN server's IP. If empty, it's assumed to be the same as the DERP server.|

## tailcfg.DERPRegion

```json
{
  "avoid": true,
  "embeddedRelay": true,
  "nodes": [
    {
      "canPort80": true,
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
```

### Properties

| Name            | Type    | Required | Restrictions | Description                                                                                                                                                                                                                         |
|-----------------|---------|----------|--------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `avoid`         | boolean | false    |              | Avoid is whether the client should avoid picking this as its home region. The region should only be used if a peer is there. Clients already using this region as their home should migrate away to a new region without Avoid set. |
| `embeddedRelay` | boolean | false    |              | Embeddedrelay is true when the region is bundled with the Coder control plane.                                                                                                                                                      |
|`nodes`|array of [tailcfg.DERPNode](#tailcfgderpnode)|false||Nodes are the DERP nodes running in this region, in priority order for the current client. Client TLS connections should ideally only go to the first entry (falling back to the second if necessary). STUN packets should go to the first 1 or 2.
If nodes within a region route packets amongst themselves, but not to other regions. That said, each user/domain should get a the same preferred node order, so if all nodes for a user/network pick the first one (as they should, when things are healthy), the inter-cluster routing is minimal to zero.|
|`regionCode`|string|false||Regioncode is a short name for the region. It's usually a popular city or airport code in the region: "nyc", "sf", "sin", "fra", etc.|
|`regionID`|integer|false||Regionid is a unique integer for a geographic region.
It corresponds to the legacy derpN.tailscale.com hostnames used by older clients. (Older clients will continue to resolve derpN.tailscale.com when contacting peers, rather than use the server-provided DERPMap)
RegionIDs must be non-zero, positive, and guaranteed to fit in a JavaScript number.
RegionIDs in range 900-999 are reserved for end users to run their own DERP nodes.|
|`regionName`|string|false||Regionname is a long English name for the region: "New York City", "San Francisco", "Singapore", "Frankfurt", etc.|

## url.Userinfo

```json
{}
```

### Properties

None

## workspaceapps.AccessMethod

```json
"path"
```

### Properties

#### Enumerated Values

| Value       |
|-------------|
| `path`      |
| `subdomain` |
| `terminal`  |

## workspaceapps.IssueTokenRequest

```json
{
  "app_hostname": "string",
  "app_path": "string",
  "app_query": "string",
  "app_request": {
    "access_method": "path",
    "agent_name_or_id": "string",
    "app_prefix": "string",
    "app_slug_or_port": "string",
    "base_path": "string",
    "username_or_id": "string",
    "workspace_name_or_id": "string"
  },
  "path_app_base_url": "string",
  "session_token": "string"
}
```

### Properties

| Name                | Type                                           | Required | Restrictions | Description                                                                                                     |
|---------------------|------------------------------------------------|----------|--------------|-----------------------------------------------------------------------------------------------------------------|
| `app_hostname`      | string                                         | false    |              | App hostname is the optional hostname for subdomain apps on the external proxy. It must start with an asterisk. |
| `app_path`          | string                                         | false    |              | App path is the path of the user underneath the app base path.                                                  |
| `app_query`         | string                                         | false    |              | App query is the query parameters the user provided in the app request.                                         |
| `app_request`       | [workspaceapps.Request](#workspaceappsrequest) | false    |              |                                                                                                                 |
| `path_app_base_url` | string                                         | false    |              | Path app base URL is required.                                                                                  |
| `session_token`     | string                                         | false    |              | Session token is the session token provided by the user.                                                        |

## workspaceapps.Request

```json
{
  "access_method": "path",
  "agent_name_or_id": "string",
  "app_prefix": "string",
  "app_slug_or_port": "string",
  "base_path": "string",
  "username_or_id": "string",
  "workspace_name_or_id": "string"
}
```

### Properties

| Name                   | Type                                                     | Required | Restrictions | Description                                                                                                                                                                           |
|------------------------|----------------------------------------------------------|----------|--------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `access_method`        | [workspaceapps.AccessMethod](#workspaceappsaccessmethod) | false    |              |                                                                                                                                                                                       |
| `agent_name_or_id`     | string                                                   | false    |              | Agent name or ID is not required if the workspace has only one agent.                                                                                                                 |
| `app_prefix`           | string                                                   | false    |              | Prefix is the prefix of the subdomain app URL. Prefix should have a trailing "---" if set.                                                                                            |
| `app_slug_or_port`     | string                                                   | false    |              |                                                                                                                                                                                       |
| `base_path`            | string                                                   | false    |              | Base path of the app. For path apps, this is the path prefix in the router for this particular app. For subdomain apps, this should be "/". This is used for setting the cookie path. |
| `username_or_id`       | string                                                   | false    |              | For the following fields, if the AccessMethod is AccessMethodTerminal, then only AgentNameOrID may be set and it must be a UUID. The other fields must be left blank.                 |
| `workspace_name_or_id` | string                                                   | false    |              |                                                                                                                                                                                       |

## workspaceapps.StatsReport

```json
{
  "access_method": "path",
  "agent_id": "string",
  "requests": 0,
  "session_ended_at": "string",
  "session_id": "string",
  "session_started_at": "string",
  "slug_or_port": "string",
  "user_id": "string",
  "workspace_id": "string"
}
```

### Properties

| Name                 | Type                                                     | Required | Restrictions | Description                                                                             |
|----------------------|----------------------------------------------------------|----------|--------------|-----------------------------------------------------------------------------------------|
| `access_method`      | [workspaceapps.AccessMethod](#workspaceappsaccessmethod) | false    |              |                                                                                         |
| `agent_id`           | string                                                   | false    |              |                                                                                         |
| `requests`           | integer                                                  | false    |              |                                                                                         |
| `session_ended_at`   | string                                                   | false    |              | Updated periodically while app is in use active and when the last connection is closed. |
| `session_id`         | string                                                   | false    |              |                                                                                         |
| `session_started_at` | string                                                   | false    |              |                                                                                         |
| `slug_or_port`       | string                                                   | false    |              |                                                                                         |
| `user_id`            | string                                                   | false    |              |                                                                                         |
| `workspace_id`       | string                                                   | false    |              |                                                                                         |

## workspacesdk.AgentConnectionInfo

```json
{
  "derp_force_websockets": true,
  "derp_map": {
    "homeParams": {
      "regionScore": {
        "property1": 0,
        "property2": 0
      }
    },
    "omitDefaultRegions": true,
    "regions": {
      "property1": {
        "avoid": true,
        "embeddedRelay": true,
        "nodes": [
          {
            "canPort80": true,
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
            "canPort80": true,
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

### Properties

| Name                         | Type                               | Required | Restrictions | Description |
|------------------------------|------------------------------------|----------|--------------|-------------|
| `derp_force_websockets`      | boolean                            | false    |              |             |
| `derp_map`                   | [tailcfg.DERPMap](#tailcfgderpmap) | false    |              |             |
| `disable_direct_connections` | boolean                            | false    |              |             |

## wsproxysdk.CryptoKeysResponse

```json
{
  "crypto_keys": [
    {
      "deletes_at": "2019-08-24T14:15:22Z",
      "feature": "workspace_apps_api_key",
      "secret": "string",
      "sequence": 0,
      "starts_at": "2019-08-24T14:15:22Z"
    }
  ]
}
```

### Properties

| Name          | Type                                              | Required | Restrictions | Description |
|---------------|---------------------------------------------------|----------|--------------|-------------|
| `crypto_keys` | array of [codersdk.CryptoKey](#codersdkcryptokey) | false    |              |             |

## wsproxysdk.DeregisterWorkspaceProxyRequest

```json
{
  "replica_id": "string"
}
```

### Properties

| Name         | Type   | Required | Restrictions | Description                                                                                                                                                                                       |
|--------------|--------|----------|--------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `replica_id` | string | false    |              | Replica ID is a unique identifier for the replica of the proxy that is deregistering. It should be generated by the client on startup and should've already been passed to the register endpoint. |

## wsproxysdk.IssueSignedAppTokenResponse

```json
{
  "signed_token_str": "string"
}
```

### Properties

| Name               | Type   | Required | Restrictions | Description                                                 |
|--------------------|--------|----------|--------------|-------------------------------------------------------------|
| `signed_token_str` | string | false    |              | Signed token str should be set as a cookie on the response. |

## wsproxysdk.RegisterWorkspaceProxyRequest

```json
{
  "access_url": "string",
  "derp_enabled": true,
  "derp_only": true,
  "hostname": "string",
  "replica_error": "string",
  "replica_id": "string",
  "replica_relay_address": "string",
  "version": "string",
  "wildcard_hostname": "string"
}
```

### Properties

| Name           | Type    | Required | Restrictions | Description                                                                                                                              |
|----------------|---------|----------|--------------|------------------------------------------------------------------------------------------------------------------------------------------|
| `access_url`   | string  | false    |              | Access URL that hits the workspace proxy api.                                                                                            |
| `derp_enabled` | boolean | false    |              | Derp enabled indicates whether the proxy should be included in the DERP map or not.                                                      |
| `derp_only`    | boolean | false    |              | Derp only indicates whether the proxy should only be included in the DERP map and should not be used for serving apps.                   |
| `hostname`     | string  | false    |              | Hostname is the OS hostname of the machine that the proxy is running on.  This is only used for tracking purposes in the replicas table. |
|`replica_error`|string|false||Replica error is the error that the replica encountered when trying to dial it's peers. This is stored in the replicas table for debugging purposes but does not affect the proxy's ability to register.
This value is only stored on subsequent requests to the register endpoint, not the first request.|
|`replica_id`|string|false||Replica ID is a unique identifier for the replica of the proxy that is registering. It should be generated by the client on startup and persisted (in memory only) until the process is restarted.|
|`replica_relay_address`|string|false||Replica relay address is the DERP address of the replica that other replicas may use to connect internally for DERP meshing.|
|`version`|string|false||Version is the Coder version of the proxy.|
|`wildcard_hostname`|string|false||Wildcard hostname that the workspace proxy api is serving for subdomain apps.|

## wsproxysdk.RegisterWorkspaceProxyResponse

```json
{
  "derp_force_websockets": true,
  "derp_map": {
    "homeParams": {
      "regionScore": {
        "property1": 0,
        "property2": 0
      }
    },
    "omitDefaultRegions": true,
    "regions": {
      "property1": {
        "avoid": true,
        "embeddedRelay": true,
        "nodes": [
          {
            "canPort80": true,
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
            "canPort80": true,
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
  "derp_mesh_key": "string",
  "derp_region_id": 0,
  "sibling_replicas": [
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
}
```

### Properties

| Name                    | Type                                          | Required | Restrictions | Description                                                                            |
|-------------------------|-----------------------------------------------|----------|--------------|----------------------------------------------------------------------------------------|
| `derp_force_websockets` | boolean                                       | false    |              |                                                                                        |
| `derp_map`              | [tailcfg.DERPMap](#tailcfgderpmap)            | false    |              |                                                                                        |
| `derp_mesh_key`         | string                                        | false    |              |                                                                                        |
| `derp_region_id`        | integer                                       | false    |              |                                                                                        |
| `sibling_replicas`      | array of [codersdk.Replica](#codersdkreplica) | false    |              | Sibling replicas is a list of all other replicas of the proxy that have not timed out. |

## wsproxysdk.ReportAppStatsRequest

```json
{
  "stats": [
    {
      "access_method": "path",
      "agent_id": "string",
      "requests": 0,
      "session_ended_at": "string",
      "session_id": "string",
      "session_started_at": "string",
      "slug_or_port": "string",
      "user_id": "string",
      "workspace_id": "string"
    }
  ]
}
```

### Properties

| Name    | Type                                                            | Required | Restrictions | Description |
|---------|-----------------------------------------------------------------|----------|--------------|-------------|
| `stats` | array of [workspaceapps.StatsReport](#workspaceappsstatsreport) | false    |              |             |
