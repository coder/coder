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
| ----------- | ------ | -------- | ------------ | ----------- |
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
| --------------- | ------ | -------- | ------------ | ----------- |
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
| ----------- | ------ | -------- | ------------ | ----------- |
| `encoding`  | string | true     |              |             |
| `signature` | string | true     |              |             |

## agentsdk.GitAuthResponse

```json
{
  "password": "string",
  "url": "string",
  "username": "string"
}
```

### Properties

| Name       | Type   | Required | Restrictions | Description |
| ---------- | ------ | -------- | ------------ | ----------- |
| `password` | string | false    |              |             |
| `url`      | string | false    |              |             |
| `username` | string | false    |              |             |

## agentsdk.GitSSHKey

```json
{
  "private_key": "string",
  "public_key": "string"
}
```

### Properties

| Name          | Type   | Required | Restrictions | Description |
| ------------- | ------ | -------- | ------------ | ----------- |
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
| ---------------- | ------ | -------- | ------------ | ----------- |
| `json_web_token` | string | true     |              |             |

## agentsdk.Metadata

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

### Properties

| Name                     | Type                                                    | Required | Restrictions | Description                                                                                                                                                |
| ------------------------ | ------------------------------------------------------- | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `apps`                   | array of [codersdk.WorkspaceApp](#codersdkworkspaceapp) | false    |              |                                                                                                                                                            |
| `derpmap`                | [tailcfg.DERPMap](#tailcfgderpmap)                      | false    |              |                                                                                                                                                            |
| `directory`              | string                                                  | false    |              |                                                                                                                                                            |
| `environment_variables`  | object                                                  | false    |              |                                                                                                                                                            |
| » `[any property]`       | string                                                  | false    |              |                                                                                                                                                            |
| `git_auth_configs`       | integer                                                 | false    |              | Git auth configs stores the number of Git configurations the Coder deployment has. If this number is >0, we set up special configuration in the workspace. |
| `motd_file`              | string                                                  | false    |              |                                                                                                                                                            |
| `startup_script`         | string                                                  | false    |              |                                                                                                                                                            |
| `startup_script_timeout` | integer                                                 | false    |              |                                                                                                                                                            |
| `vscode_port_proxy_uri`  | string                                                  | false    |              |                                                                                                                                                            |

## agentsdk.PostAppHealthsRequest

```json
{
  "healths": {
    "property1": "disabled",
    "property2": "disabled"
  }
}
```

### Properties

| Name               | Type                                                       | Required | Restrictions | Description                                                           |
| ------------------ | ---------------------------------------------------------- | -------- | ------------ | --------------------------------------------------------------------- |
| `healths`          | object                                                     | false    |              | Healths is a map of the workspace app name and the health of the app. |
| » `[any property]` | [codersdk.WorkspaceAppHealth](#codersdkworkspaceapphealth) | false    |              |                                                                       |

## agentsdk.PostLifecycleRequest

```json
{
  "state": "created"
}
```

### Properties

| Name    | Type                                                                 | Required | Restrictions | Description |
| ------- | -------------------------------------------------------------------- | -------- | ------------ | ----------- |
| `state` | [codersdk.WorkspaceAgentLifecycle](#codersdkworkspaceagentlifecycle) | false    |              |             |

## agentsdk.PostStartupRequest

```json
{
  "expanded_directory": "string",
  "version": "string"
}
```

### Properties

| Name                 | Type   | Required | Restrictions | Description |
| -------------------- | ------ | -------- | ------------ | ----------- |
| `expanded_directory` | string | false    |              |             |
| `version`            | string | false    |              |             |

## agentsdk.Stats

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

### Properties

| Name               | Type    | Required | Restrictions | Description                                                  |
| ------------------ | ------- | -------- | ------------ | ------------------------------------------------------------ |
| `conns_by_proto`   | object  | false    |              | Conns by proto is a count of connections by protocol.        |
| » `[any property]` | integer | false    |              |                                                              |
| `num_comms`        | integer | false    |              | Num comms is the number of connections received by an agent. |
| `rx_bytes`         | integer | false    |              | Rx bytes is the number of received bytes.                    |
| `rx_packets`       | integer | false    |              | Rx packets is the number of received packets.                |
| `tx_bytes`         | integer | false    |              | Tx bytes is the number of transmitted bytes.                 |
| `tx_packets`       | integer | false    |              | Tx packets is the number of transmitted bytes.               |

## agentsdk.StatsResponse

```json
{
  "report_interval": 0
}
```

### Properties

| Name              | Type    | Required | Restrictions | Description                                                                    |
| ----------------- | ------- | -------- | ------------ | ------------------------------------------------------------------------------ |
| `report_interval` | integer | false    |              | Report interval is the duration after which the agent should send stats again. |

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

### Properties

| Name             | Type               | Required | Restrictions | Description |
| ---------------- | ------------------ | -------- | ------------ | ----------- |
| `active`         | boolean            | false    |              |             |
| `emails`         | array of object    | false    |              |             |
| `» display`      | string             | false    |              |             |
| `» primary`      | boolean            | false    |              |             |
| `» type`         | string             | false    |              |             |
| `» value`        | string             | false    |              |             |
| `groups`         | array of undefined | false    |              |             |
| `id`             | string             | false    |              |             |
| `meta`           | object             | false    |              |             |
| `» resourceType` | string             | false    |              |             |
| `name`           | object             | false    |              |             |
| `» familyName`   | string             | false    |              |             |
| `» givenName`    | string             | false    |              |             |
| `schemas`        | array of string    | false    |              |             |
| `userName`       | string             | false    |              |             |

## coderd.cspViolation

```json
{
  "csp-report": {}
}
```

### Properties

| Name         | Type   | Required | Restrictions | Description |
| ------------ | ------ | -------- | ------------ | ----------- |
| `csp-report` | object | false    |              |             |

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
  "updated_at": "2019-08-24T14:15:22Z",
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
}
```

### Properties

| Name               | Type                                         | Required | Restrictions | Description |
| ------------------ | -------------------------------------------- | -------- | ------------ | ----------- |
| `created_at`       | string                                       | true     |              |             |
| `expires_at`       | string                                       | true     |              |             |
| `id`               | string                                       | true     |              |             |
| `last_used`        | string                                       | true     |              |             |
| `lifetime_seconds` | integer                                      | true     |              |             |
| `login_type`       | [codersdk.LoginType](#codersdklogintype)     | true     |              |             |
| `scope`            | [codersdk.APIKeyScope](#codersdkapikeyscope) | true     |              |             |
| `updated_at`       | string                                       | true     |              |             |
| `user_id`          | string                                       | true     |              |             |

#### Enumerated Values

| Property     | Value                 |
| ------------ | --------------------- |
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
| --------------------- |
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
| --------- | ------ | -------- | ------------ | ----------- |
| `license` | string | true     |              |             |

## codersdk.AppHostResponse

```json
{
  "host": "string"
}
```

### Properties

| Name   | Type   | Required | Restrictions | Description                                                   |
| ------ | ------ | -------- | ------------ | ------------------------------------------------------------- |
| `host` | string | false    |              | Host is the externally accessible URL for the Coder instance. |

## codersdk.AppearanceConfig

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

### Properties

| Name             | Type                                                         | Required | Restrictions | Description |
| ---------------- | ------------------------------------------------------------ | -------- | ------------ | ----------- |
| `logo_url`       | string                                                       | false    |              |             |
| `service_banner` | [codersdk.ServiceBannerConfig](#codersdkservicebannerconfig) | false    |              |             |

## codersdk.AssignableRoles

```json
{
  "assignable": true,
  "display_name": "string",
  "name": "string"
}
```

### Properties

| Name           | Type    | Required | Restrictions | Description |
| -------------- | ------- | -------- | ------------ | ----------- |
| `assignable`   | boolean | false    |              |             |
| `display_name` | string  | false    |              |             |
| `name`         | string  | false    |              |             |

## codersdk.AuditAction

```json
"create"
```

### Properties

#### Enumerated Values

| Value    |
| -------- |
| `create` |
| `write`  |
| `delete` |
| `start`  |
| `stop`   |
| `login`  |
| `logout` |

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
| ---------------- | -------------------------------------------------- | -------- | ------------ | ----------- |
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
| -------- | ------- | -------- | ------------ | ----------- |
| `new`    | any     | false    |              |             |
| `old`    | any     | false    |              |             |
| `secret` | boolean | false    |              |             |

## codersdk.AuditLog

```json
{
  "action": "create",
  "additional_fields": [0],
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
    "organization_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
    "roles": [
      {
        "display_name": "string",
        "name": "string"
      }
    ],
    "status": "active",
    "username": "string"
  },
  "user_agent": "string"
}
```

### Properties

| Name                | Type                                           | Required | Restrictions | Description                                  |
| ------------------- | ---------------------------------------------- | -------- | ------------ | -------------------------------------------- |
| `action`            | [codersdk.AuditAction](#codersdkauditaction)   | false    |              |                                              |
| `additional_fields` | array of integer                               | false    |              |                                              |
| `description`       | string                                         | false    |              |                                              |
| `diff`              | [codersdk.AuditDiff](#codersdkauditdiff)       | false    |              |                                              |
| `id`                | string                                         | false    |              |                                              |
| `ip`                | string                                         | false    |              |                                              |
| `is_deleted`        | boolean                                        | false    |              |                                              |
| `organization_id`   | string                                         | false    |              |                                              |
| `request_id`        | string                                         | false    |              |                                              |
| `resource_icon`     | string                                         | false    |              |                                              |
| `resource_id`       | string                                         | false    |              |                                              |
| `resource_link`     | string                                         | false    |              |                                              |
| `resource_target`   | string                                         | false    |              | Resource target is the name of the resource. |
| `resource_type`     | [codersdk.ResourceType](#codersdkresourcetype) | false    |              |                                              |
| `status_code`       | integer                                        | false    |              |                                              |
| `time`              | string                                         | false    |              |                                              |
| `user`              | [codersdk.User](#codersdkuser)                 | false    |              |                                              |
| `user_agent`        | string                                         | false    |              |                                              |

## codersdk.AuditLogResponse

```json
{
  "audit_logs": [
    {
      "action": "create",
      "additional_fields": [0],
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
        "organization_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
        "roles": [
          {
            "display_name": "string",
            "name": "string"
          }
        ],
        "status": "active",
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
| ------------ | ----------------------------------------------- | -------- | ------------ | ----------- |
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
| --------- | ------- | -------- | ------------ | ----------- |
| `enabled` | boolean | false    |              |             |

## codersdk.AuthMethods

```json
{
  "github": {
    "enabled": true
  },
  "oidc": {
    "enabled": true,
    "iconUrl": "string",
    "signInText": "string"
  },
  "password": {
    "enabled": true
  }
}
```

### Properties

| Name       | Type                                               | Required | Restrictions | Description |
| ---------- | -------------------------------------------------- | -------- | ------------ | ----------- |
| `github`   | [codersdk.AuthMethod](#codersdkauthmethod)         | false    |              |             |
| `oidc`     | [codersdk.OIDCAuthMethod](#codersdkoidcauthmethod) | false    |              |             |
| `password` | [codersdk.AuthMethod](#codersdkauthmethod)         | false    |              |             |

## codersdk.AuthorizationCheck

```json
{
  "action": "create",
  "object": {
    "organization_id": "string",
    "owner_id": "string",
    "resource_id": "string",
    "resource_type": "string"
  }
}
```

AuthorizationCheck is used to check if the currently authenticated user (or the specified user) can do a given action to a given set of objects.

### Properties

| Name     | Type                                                         | Required | Restrictions | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| -------- | ------------------------------------------------------------ | -------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `action` | string                                                       | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `object` | [codersdk.AuthorizationObject](#codersdkauthorizationobject) | false    |              | Object can represent a "set" of objects, such as: all workspaces in an organization, all workspaces owned by me, and all workspaces across the entire product. When defining an object, use the most specific language when possible to produce the smallest set. Meaning to set as many fields on 'Object' as you can. Example, if you want to check if you can update all workspaces owned by 'me', try to also add an 'OrganizationID' to the settings. Omitting the 'OrganizationID' could produce the incorrect value, as workspaces have both `user` and `organization` owners. |

#### Enumerated Values

| Property | Value    |
| -------- | -------- |
| `action` | `create` |
| `action` | `read`   |
| `action` | `update` |
| `action` | `delete` |

## codersdk.AuthorizationObject

```json
{
  "organization_id": "string",
  "owner_id": "string",
  "resource_id": "string",
  "resource_type": "string"
}
```

AuthorizationObject can represent a "set" of objects, such as: all workspaces in an organization, all workspaces owned by me, all workspaces across the entire product.

### Properties

| Name              | Type   | Required | Restrictions | Description                                                                                                                                                                                                                                                                                                                                                          |
| ----------------- | ------ | -------- | ------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `organization_id` | string | false    |              | Organization ID (optional) adds the set constraint to all resources owned by a given organization.                                                                                                                                                                                                                                                                   |
| `owner_id`        | string | false    |              | Owner ID (optional) adds the set constraint to all resources owned by a given user.                                                                                                                                                                                                                                                                                  |
| `resource_id`     | string | false    |              | Resource ID (optional) reduces the set to a singular resource. This assigns a resource ID to the resource type, eg: a single workspace. The rbac library will not fetch the resource from the database, so if you are using this option, you should also set the owner ID and organization ID if possible. Be as specific as possible using all the fields relevant. |
| `resource_type`   | string | false    |              | Resource type is the name of the resource. `./coderd/rbac/object.go` has the list of valid resource types.                                                                                                                                                                                                                                                           |

## codersdk.AuthorizationRequest

```json
{
  "checks": {
    "property1": {
      "action": "create",
      "object": {
        "organization_id": "string",
        "owner_id": "string",
        "resource_id": "string",
        "resource_type": "string"
      }
    },
    "property2": {
      "action": "create",
      "object": {
        "organization_id": "string",
        "owner_id": "string",
        "resource_id": "string",
        "resource_type": "string"
      }
    }
  }
}
```

### Properties

| Name               | Type                                                       | Required | Restrictions | Description                                                                                                                                                                                                                                                                      |
| ------------------ | ---------------------------------------------------------- | -------- | ------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
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
| ---------------- | ------- | -------- | ------------ | ----------- |
| `[any property]` | boolean | false    |              |             |

## codersdk.BuildInfoResponse

```json
{
  "external_url": "string",
  "version": "string"
}
```

### Properties

| Name           | Type   | Required | Restrictions | Description                                                                                                                                                         |
| -------------- | ------ | -------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `external_url` | string | false    |              | External URL references the current Coder version. For production builds, this will link directly to a release. For development builds, this will link to a commit. |
| `version`      | string | false    |              | Version returns the semantic version of the build.                                                                                                                  |

## codersdk.BuildReason

```json
"initiator"
```

### Properties

#### Enumerated Values

| Value       |
| ----------- |
| `initiator` |
| `autostart` |
| `autostop`  |

## codersdk.CreateFirstUserRequest

```json
{
  "email": "string",
  "password": "string",
  "trial": true,
  "username": "string"
}
```

### Properties

| Name       | Type    | Required | Restrictions | Description |
| ---------- | ------- | -------- | ------------ | ----------- |
| `email`    | string  | true     |              |             |
| `password` | string  | true     |              |             |
| `trial`    | boolean | false    |              |             |
| `username` | string  | true     |              |             |

## codersdk.CreateFirstUserResponse

```json
{
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
}
```

### Properties

| Name              | Type   | Required | Restrictions | Description |
| ----------------- | ------ | -------- | ------------ | ----------- |
| `organization_id` | string | false    |              |             |
| `user_id`         | string | false    |              |             |

## codersdk.CreateGroupRequest

```json
{
  "avatar_url": "string",
  "name": "string",
  "quota_allowance": 0
}
```

### Properties

| Name              | Type    | Required | Restrictions | Description |
| ----------------- | ------- | -------- | ------------ | ----------- |
| `avatar_url`      | string  | false    |              |             |
| `name`            | string  | false    |              |             |
| `quota_allowance` | integer | false    |              |             |

## codersdk.CreateOrganizationRequest

```json
{
  "name": "string"
}
```

### Properties

| Name   | Type   | Required | Restrictions | Description |
| ------ | ------ | -------- | ------------ | ----------- |
| `name` | string | true     |              |             |

## codersdk.CreateParameterRequest

```json
{
  "copy_from_parameter": "000e07d6-021d-446c-be14-48a9c20bca0b",
  "destination_scheme": "none",
  "name": "string",
  "source_scheme": "none",
  "source_value": "string"
}
```

CreateParameterRequest is a structure used to create a new parameter value for a scope.

### Properties

| Name                  | Type                                                                       | Required | Restrictions | Description                                                                                                                                                                                                                                        |
| --------------------- | -------------------------------------------------------------------------- | -------- | ------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `copy_from_parameter` | string                                                                     | false    |              | Copy from parameter allows copying the value of another parameter. The other param must be related to the same template_id for this to succeed. No other fields are required if using this, as all fields will be copied from the other parameter. |
| `destination_scheme`  | [codersdk.ParameterDestinationScheme](#codersdkparameterdestinationscheme) | true     |              |                                                                                                                                                                                                                                                    |
| `name`                | string                                                                     | true     |              |                                                                                                                                                                                                                                                    |
| `source_scheme`       | [codersdk.ParameterSourceScheme](#codersdkparametersourcescheme)           | true     |              |                                                                                                                                                                                                                                                    |
| `source_value`        | string                                                                     | true     |              |                                                                                                                                                                                                                                                    |

#### Enumerated Values

| Property             | Value                  |
| -------------------- | ---------------------- |
| `destination_scheme` | `none`                 |
| `destination_scheme` | `environment_variable` |
| `destination_scheme` | `provisioner_variable` |
| `source_scheme`      | `none`                 |
| `source_scheme`      | `data`                 |

## codersdk.CreateTemplateRequest

```json
{
  "allow_user_cancel_workspace_jobs": true,
  "default_ttl_ms": 0,
  "description": "string",
  "display_name": "string",
  "icon": "string",
  "name": "string",
  "parameter_values": [
    {
      "copy_from_parameter": "000e07d6-021d-446c-be14-48a9c20bca0b",
      "destination_scheme": "none",
      "name": "string",
      "source_scheme": "none",
      "source_value": "string"
    }
  ],
  "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1"
}
```

### Properties

| Name                                                                                                                                                                                      | Type                                                                        | Required | Restrictions | Description                                                                                                |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------- | -------- | ------------ | ---------------------------------------------------------------------------------------------------------- |
| `allow_user_cancel_workspace_jobs`                                                                                                                                                        | boolean                                                                     | false    |              | Allow users to cancel in-progress workspace jobs. \*bool as the default value is "true".                   |
| `default_ttl_ms`                                                                                                                                                                          | integer                                                                     | false    |              | Default ttl ms allows optionally specifying the default TTL for all workspaces created from this template. |
| `description`                                                                                                                                                                             | string                                                                      | false    |              | Description is a description of what the template contains. It must be less than 128 bytes.                |
| `display_name`                                                                                                                                                                            | string                                                                      | false    |              | Display name is the displayed name of the template.                                                        |
| `icon`                                                                                                                                                                                    | string                                                                      | false    |              | Icon is a relative path or external URL that specifies an icon to be displayed in the dashboard.           |
| `name`                                                                                                                                                                                    | string                                                                      | true     |              | Name is the name of the template.                                                                          |
| `parameter_values`                                                                                                                                                                        | array of [codersdk.CreateParameterRequest](#codersdkcreateparameterrequest) | false    |              | Parameter values is a structure used to create a new parameter value for a scope.]                         |
| `template_version_id`                                                                                                                                                                     | string                                                                      | true     |              | Template version ID is an in-progress or completed job to use as an initial version of the template.       |
| This is required on creation to enable a user-flow of validating a template works. There is no reason the data-model cannot support empty templates, but it doesn't make sense for users. |

## codersdk.CreateTemplateVersionDryRunRequest

```json
{
  "parameter_values": [
    {
      "copy_from_parameter": "000e07d6-021d-446c-be14-48a9c20bca0b",
      "destination_scheme": "none",
      "name": "string",
      "source_scheme": "none",
      "source_value": "string"
    }
  ],
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

| Name                    | Type                                                                          | Required | Restrictions | Description                                                                        |
| ----------------------- | ----------------------------------------------------------------------------- | -------- | ------------ | ---------------------------------------------------------------------------------- |
| `parameter_values`      | array of [codersdk.CreateParameterRequest](#codersdkcreateparameterrequest)   | false    |              | Parameter values is a structure used to create a new parameter value for a scope.] |
| `rich_parameter_values` | array of [codersdk.WorkspaceBuildParameter](#codersdkworkspacebuildparameter) | false    |              |                                                                                    |
| `user_variable_values`  | array of [codersdk.VariableValue](#codersdkvariablevalue)                     | false    |              |                                                                                    |
| `workspace_name`        | string                                                                        | false    |              |                                                                                    |

## codersdk.CreateTestAuditLogRequest

```json
{
  "action": "create",
  "additional_fields": [0],
  "build_reason": "autostart",
  "resource_id": "4d5215ed-38bb-48ed-879a-fdb9ca58522f",
  "resource_type": "template",
  "time": "2019-08-24T14:15:22Z"
}
```

### Properties

| Name                | Type                                           | Required | Restrictions | Description |
| ------------------- | ---------------------------------------------- | -------- | ------------ | ----------- |
| `action`            | [codersdk.AuditAction](#codersdkauditaction)   | false    |              |             |
| `additional_fields` | array of integer                               | false    |              |             |
| `build_reason`      | [codersdk.BuildReason](#codersdkbuildreason)   | false    |              |             |
| `resource_id`       | string                                         | false    |              |             |
| `resource_type`     | [codersdk.ResourceType](#codersdkresourcetype) | false    |              |             |
| `time`              | string                                         | false    |              |             |

#### Enumerated Values

| Property        | Value              |
| --------------- | ------------------ |
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
  "scope": "all"
}
```

### Properties

| Name       | Type                                         | Required | Restrictions | Description |
| ---------- | -------------------------------------------- | -------- | ------------ | ----------- |
| `lifetime` | integer                                      | false    |              |             |
| `scope`    | [codersdk.APIKeyScope](#codersdkapikeyscope) | false    |              |             |

#### Enumerated Values

| Property | Value                 |
| -------- | --------------------- |
| `scope`  | `all`                 |
| `scope`  | `application_connect` |

## codersdk.CreateUserRequest

```json
{
  "email": "user@example.com",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "password": "string",
  "username": "string"
}
```

### Properties

| Name              | Type   | Required | Restrictions | Description |
| ----------------- | ------ | -------- | ------------ | ----------- |
| `email`           | string | true     |              |             |
| `organization_id` | string | true     |              |             |
| `password`        | string | true     |              |             |
| `username`        | string | true     |              |             |

## codersdk.CreateWorkspaceBuildRequest

```json
{
  "dry_run": true,
  "orphan": true,
  "parameter_values": [
    {
      "copy_from_parameter": "000e07d6-021d-446c-be14-48a9c20bca0b",
      "destination_scheme": "none",
      "name": "string",
      "source_scheme": "none",
      "source_value": "string"
    }
  ],
  "rich_parameter_values": [
    {
      "name": "string",
      "value": "string"
    }
  ],
  "state": [0],
  "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
  "transition": "create"
}
```

### Properties

| Name                    | Type                                                                          | Required | Restrictions | Description                                                                                                                                                                                              |
| ----------------------- | ----------------------------------------------------------------------------- | -------- | ------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `dry_run`               | boolean                                                                       | false    |              |                                                                                                                                                                                                          |
| `orphan`                | boolean                                                                       | false    |              | Orphan may be set for the Destroy transition.                                                                                                                                                            |
| `parameter_values`      | array of [codersdk.CreateParameterRequest](#codersdkcreateparameterrequest)   | false    |              | Parameter values are optional. It will write params to the 'workspace' scope. This will overwrite any existing parameters with the same name. This will not delete old params not included in this list. |
| `rich_parameter_values` | array of [codersdk.WorkspaceBuildParameter](#codersdkworkspacebuildparameter) | false    |              |                                                                                                                                                                                                          |
| `state`                 | array of integer                                                              | false    |              |                                                                                                                                                                                                          |
| `template_version_id`   | string                                                                        | false    |              |                                                                                                                                                                                                          |
| `transition`            | [codersdk.WorkspaceTransition](#codersdkworkspacetransition)                  | true     |              |                                                                                                                                                                                                          |

#### Enumerated Values

| Property     | Value    |
| ------------ | -------- |
| `transition` | `create` |
| `transition` | `start`  |
| `transition` | `stop`   |
| `transition` | `delete` |

## codersdk.CreateWorkspaceRequest

```json
{
  "autostart_schedule": "string",
  "name": "string",
  "parameter_values": [
    {
      "copy_from_parameter": "000e07d6-021d-446c-be14-48a9c20bca0b",
      "destination_scheme": "none",
      "name": "string",
      "source_scheme": "none",
      "source_value": "string"
    }
  ],
  "rich_parameter_values": [
    {
      "name": "string",
      "value": "string"
    }
  ],
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "ttl_ms": 0
}
```

### Properties

| Name                    | Type                                                                          | Required | Restrictions | Description                                                                                    |
| ----------------------- | ----------------------------------------------------------------------------- | -------- | ------------ | ---------------------------------------------------------------------------------------------- |
| `autostart_schedule`    | string                                                                        | false    |              |                                                                                                |
| `name`                  | string                                                                        | true     |              |                                                                                                |
| `parameter_values`      | array of [codersdk.CreateParameterRequest](#codersdkcreateparameterrequest)   | false    |              | Parameter values allows for additional parameters to be provided during the initial provision. |
| `rich_parameter_values` | array of [codersdk.WorkspaceBuildParameter](#codersdkworkspacebuildparameter) | false    |              |                                                                                                |
| `template_id`           | string                                                                        | true     |              |                                                                                                |
| `ttl_ms`                | integer                                                                       | false    |              |                                                                                                |

## codersdk.DAUEntry

```json
{
  "amount": 0,
  "date": "2019-08-24T14:15:22Z"
}
```

### Properties

| Name     | Type    | Required | Restrictions | Description |
| -------- | ------- | -------- | ------------ | ----------- |
| `amount` | integer | false    |              |             |
| `date`   | string  | false    |              |             |

## codersdk.DERP

```json
{
  "config": {
    "path": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "url": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    }
  },
  "server": {
    "enable": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    },
    "region_code": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "region_id": {
      "default": 0,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": 0
    },
    "region_name": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "relay_url": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "stun_addresses": {
      "default": ["string"],
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": ["string"]
    }
  }
}
```

### Properties

| Name     | Type                                                   | Required | Restrictions | Description |
| -------- | ------------------------------------------------------ | -------- | ------------ | ----------- |
| `config` | [codersdk.DERPConfig](#codersdkderpconfig)             | false    |              |             |
| `server` | [codersdk.DERPServerConfig](#codersdkderpserverconfig) | false    |              |             |

## codersdk.DERPConfig

```json
{
  "path": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "url": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  }
}
```

### Properties

| Name   | Type                                                                           | Required | Restrictions | Description |
| ------ | ------------------------------------------------------------------------------ | -------- | ------------ | ----------- |
| `path` | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string) | false    |              |             |
| `url`  | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string) | false    |              |             |

## codersdk.DERPRegion

```json
{
  "latency_ms": 0,
  "preferred": true
}
```

### Properties

| Name         | Type    | Required | Restrictions | Description |
| ------------ | ------- | -------- | ------------ | ----------- |
| `latency_ms` | number  | false    |              |             |
| `preferred`  | boolean | false    |              |             |

## codersdk.DERPServerConfig

```json
{
  "enable": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "region_code": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "region_id": {
    "default": 0,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": 0
  },
  "region_name": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "relay_url": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "stun_addresses": {
    "default": ["string"],
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": ["string"]
  }
}
```

### Properties

| Name             | Type                                                                                       | Required | Restrictions | Description |
| ---------------- | ------------------------------------------------------------------------------------------ | -------- | ------------ | ----------- |
| `enable`         | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)                 | false    |              |             |
| `region_code`    | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)             | false    |              |             |
| `region_id`      | [codersdk.DeploymentConfigField-int](#codersdkdeploymentconfigfield-int)                   | false    |              |             |
| `region_name`    | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)             | false    |              |             |
| `relay_url`      | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)             | false    |              |             |
| `stun_addresses` | [codersdk.DeploymentConfigField-array_string](#codersdkdeploymentconfigfield-array_string) | false    |              |             |

## codersdk.DangerousConfig

```json
{
  "allow_path_app_sharing": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "allow_path_app_site_owner_access": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  }
}
```

### Properties

| Name                               | Type                                                                       | Required | Restrictions | Description |
| ---------------------------------- | -------------------------------------------------------------------------- | -------- | ------------ | ----------- |
| `allow_path_app_sharing`           | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool) | false    |              |             |
| `allow_path_app_site_owner_access` | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool) | false    |              |             |

## codersdk.DeploymentConfig

```json
{
  "access_url": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "address": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "agent_fallback_troubleshooting_url": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "agent_stat_refresh_interval": {
    "default": 0,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": 0
  },
  "audit_logging": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "autobuild_poll_interval": {
    "default": 0,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": 0
  },
  "browser_only": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "cache_directory": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "dangerous": {
    "allow_path_app_sharing": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    },
    "allow_path_app_site_owner_access": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    }
  },
  "derp": {
    "config": {
      "path": {
        "default": "string",
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": "string"
      },
      "url": {
        "default": "string",
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": "string"
      }
    },
    "server": {
      "enable": {
        "default": true,
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": true
      },
      "region_code": {
        "default": "string",
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": "string"
      },
      "region_id": {
        "default": 0,
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": 0
      },
      "region_name": {
        "default": "string",
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": "string"
      },
      "relay_url": {
        "default": "string",
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": "string"
      },
      "stun_addresses": {
        "default": ["string"],
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": ["string"]
      }
    }
  },
  "disable_password_auth": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "disable_path_apps": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "disable_session_expiry_refresh": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "experimental": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "experiments": {
    "default": ["string"],
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": ["string"]
  },
  "gitauth": {
    "default": [
      {
        "auth_url": "string",
        "client_id": "string",
        "id": "string",
        "no_refresh": true,
        "regex": "string",
        "scopes": ["string"],
        "token_url": "string",
        "type": "string",
        "validate_url": "string"
      }
    ],
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": [
      {
        "auth_url": "string",
        "client_id": "string",
        "id": "string",
        "no_refresh": true,
        "regex": "string",
        "scopes": ["string"],
        "token_url": "string",
        "type": "string",
        "validate_url": "string"
      }
    ]
  },
  "http_address": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "in_memory_database": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "logging": {
    "human": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "json": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "stackdriver": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    }
  },
  "max_session_expiry": {
    "default": 0,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": 0
  },
  "max_token_lifetime": {
    "default": 0,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": 0
  },
  "metrics_cache_refresh_interval": {
    "default": 0,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": 0
  },
  "oauth2": {
    "github": {
      "allow_everyone": {
        "default": true,
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": true
      },
      "allow_signups": {
        "default": true,
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": true
      },
      "allowed_orgs": {
        "default": ["string"],
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": ["string"]
      },
      "allowed_teams": {
        "default": ["string"],
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": ["string"]
      },
      "client_id": {
        "default": "string",
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": "string"
      },
      "client_secret": {
        "default": "string",
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": "string"
      },
      "enterprise_base_url": {
        "default": "string",
        "enterprise": true,
        "flag": "string",
        "hidden": true,
        "name": "string",
        "secret": true,
        "shorthand": "string",
        "usage": "string",
        "value": "string"
      }
    }
  },
  "oidc": {
    "allow_signups": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    },
    "client_id": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "client_secret": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "email_domain": {
      "default": ["string"],
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": ["string"]
    },
    "icon_url": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "ignore_email_verified": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    },
    "issuer_url": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "scopes": {
      "default": ["string"],
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": ["string"]
    },
    "sign_in_text": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "username_field": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    }
  },
  "pg_connection_url": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "pprof": {
    "address": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "enable": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    }
  },
  "prometheus": {
    "address": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "enable": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    }
  },
  "provisioner": {
    "daemon_poll_interval": {
      "default": 0,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": 0
    },
    "daemon_poll_jitter": {
      "default": 0,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": 0
    },
    "daemons": {
      "default": 0,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": 0
    },
    "force_cancel_interval": {
      "default": 0,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": 0
    }
  },
  "proxy_trusted_headers": {
    "default": ["string"],
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": ["string"]
  },
  "proxy_trusted_origins": {
    "default": ["string"],
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": ["string"]
  },
  "rate_limit": {
    "api": {
      "default": 0,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": 0
    },
    "disable_all": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    }
  },
  "redirect_to_access_url": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "scim_api_key": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "secure_auth_cookie": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "ssh_keygen_algorithm": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "strict_transport_security": {
    "default": 0,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": 0
  },
  "strict_transport_security_options": {
    "default": ["string"],
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": ["string"]
  },
  "swagger": {
    "enable": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    }
  },
  "telemetry": {
    "enable": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    },
    "trace": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    },
    "url": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    }
  },
  "tls": {
    "address": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "cert_file": {
      "default": ["string"],
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": ["string"]
    },
    "client_auth": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "client_ca_file": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "client_cert_file": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "client_key_file": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "enable": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    },
    "key_file": {
      "default": ["string"],
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": ["string"]
    },
    "min_version": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "redirect_http": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    }
  },
  "trace": {
    "capture_logs": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    },
    "enable": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    },
    "honeycomb_api_key": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    }
  },
  "update_check": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "wildcard_access_url": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  }
}
```

### Properties

| Name                                 | Type                                                                                                                       | Required | Restrictions | Description                                     |
| ------------------------------------ | -------------------------------------------------------------------------------------------------------------------------- | -------- | ------------ | ----------------------------------------------- |
| `access_url`                         | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)                                             | false    |              |                                                 |
| `address`                            | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)                                             | false    |              | Address Use HTTPAddress or TLS.Address instead. |
| `agent_fallback_troubleshooting_url` | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)                                             | false    |              |                                                 |
| `agent_stat_refresh_interval`        | [codersdk.DeploymentConfigField-time_Duration](#codersdkdeploymentconfigfield-time_duration)                               | false    |              |                                                 |
| `audit_logging`                      | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)                                                 | false    |              |                                                 |
| `autobuild_poll_interval`            | [codersdk.DeploymentConfigField-time_Duration](#codersdkdeploymentconfigfield-time_duration)                               | false    |              |                                                 |
| `browser_only`                       | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)                                                 | false    |              |                                                 |
| `cache_directory`                    | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)                                             | false    |              |                                                 |
| `dangerous`                          | [codersdk.DangerousConfig](#codersdkdangerousconfig)                                                                       | false    |              |                                                 |
| `derp`                               | [codersdk.DERP](#codersdkderp)                                                                                             | false    |              |                                                 |
| `disable_password_auth`              | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)                                                 | false    |              |                                                 |
| `disable_path_apps`                  | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)                                                 | false    |              |                                                 |
| `disable_session_expiry_refresh`     | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)                                                 | false    |              |                                                 |
| `experimental`                       | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)                                                 | false    |              | Experimental Use Experiments instead.           |
| `experiments`                        | [codersdk.DeploymentConfigField-array_string](#codersdkdeploymentconfigfield-array_string)                                 | false    |              |                                                 |
| `gitauth`                            | [codersdk.DeploymentConfigField-array_codersdk_GitAuthConfig](#codersdkdeploymentconfigfield-array_codersdk_gitauthconfig) | false    |              |                                                 |
| `http_address`                       | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)                                             | false    |              |                                                 |
| `in_memory_database`                 | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)                                                 | false    |              |                                                 |
| `logging`                            | [codersdk.LoggingConfig](#codersdkloggingconfig)                                                                           | false    |              |                                                 |
| `max_session_expiry`                 | [codersdk.DeploymentConfigField-time_Duration](#codersdkdeploymentconfigfield-time_duration)                               | false    |              |                                                 |
| `max_token_lifetime`                 | [codersdk.DeploymentConfigField-time_Duration](#codersdkdeploymentconfigfield-time_duration)                               | false    |              |                                                 |
| `metrics_cache_refresh_interval`     | [codersdk.DeploymentConfigField-time_Duration](#codersdkdeploymentconfigfield-time_duration)                               | false    |              |                                                 |
| `oauth2`                             | [codersdk.OAuth2Config](#codersdkoauth2config)                                                                             | false    |              |                                                 |
| `oidc`                               | [codersdk.OIDCConfig](#codersdkoidcconfig)                                                                                 | false    |              |                                                 |
| `pg_connection_url`                  | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)                                             | false    |              |                                                 |
| `pprof`                              | [codersdk.PprofConfig](#codersdkpprofconfig)                                                                               | false    |              |                                                 |
| `prometheus`                         | [codersdk.PrometheusConfig](#codersdkprometheusconfig)                                                                     | false    |              |                                                 |
| `provisioner`                        | [codersdk.ProvisionerConfig](#codersdkprovisionerconfig)                                                                   | false    |              |                                                 |
| `proxy_trusted_headers`              | [codersdk.DeploymentConfigField-array_string](#codersdkdeploymentconfigfield-array_string)                                 | false    |              |                                                 |
| `proxy_trusted_origins`              | [codersdk.DeploymentConfigField-array_string](#codersdkdeploymentconfigfield-array_string)                                 | false    |              |                                                 |
| `rate_limit`                         | [codersdk.RateLimitConfig](#codersdkratelimitconfig)                                                                       | false    |              |                                                 |
| `redirect_to_access_url`             | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)                                                 | false    |              |                                                 |
| `scim_api_key`                       | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)                                             | false    |              |                                                 |
| `secure_auth_cookie`                 | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)                                                 | false    |              |                                                 |
| `ssh_keygen_algorithm`               | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)                                             | false    |              |                                                 |
| `strict_transport_security`          | [codersdk.DeploymentConfigField-int](#codersdkdeploymentconfigfield-int)                                                   | false    |              |                                                 |
| `strict_transport_security_options`  | [codersdk.DeploymentConfigField-array_string](#codersdkdeploymentconfigfield-array_string)                                 | false    |              |                                                 |
| `swagger`                            | [codersdk.SwaggerConfig](#codersdkswaggerconfig)                                                                           | false    |              |                                                 |
| `telemetry`                          | [codersdk.TelemetryConfig](#codersdktelemetryconfig)                                                                       | false    |              |                                                 |
| `tls`                                | [codersdk.TLSConfig](#codersdktlsconfig)                                                                                   | false    |              |                                                 |
| `trace`                              | [codersdk.TraceConfig](#codersdktraceconfig)                                                                               | false    |              |                                                 |
| `update_check`                       | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)                                                 | false    |              |                                                 |
| `wildcard_access_url`                | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)                                             | false    |              |                                                 |

## codersdk.DeploymentConfigField-array_codersdk_GitAuthConfig

```json
{
  "default": [
    {
      "auth_url": "string",
      "client_id": "string",
      "id": "string",
      "no_refresh": true,
      "regex": "string",
      "scopes": ["string"],
      "token_url": "string",
      "type": "string",
      "validate_url": "string"
    }
  ],
  "enterprise": true,
  "flag": "string",
  "hidden": true,
  "name": "string",
  "secret": true,
  "shorthand": "string",
  "usage": "string",
  "value": [
    {
      "auth_url": "string",
      "client_id": "string",
      "id": "string",
      "no_refresh": true,
      "regex": "string",
      "scopes": ["string"],
      "token_url": "string",
      "type": "string",
      "validate_url": "string"
    }
  ]
}
```

### Properties

| Name         | Type                                                      | Required | Restrictions | Description |
| ------------ | --------------------------------------------------------- | -------- | ------------ | ----------- |
| `default`    | array of [codersdk.GitAuthConfig](#codersdkgitauthconfig) | false    |              |             |
| `enterprise` | boolean                                                   | false    |              |             |
| `flag`       | string                                                    | false    |              |             |
| `hidden`     | boolean                                                   | false    |              |             |
| `name`       | string                                                    | false    |              |             |
| `secret`     | boolean                                                   | false    |              |             |
| `shorthand`  | string                                                    | false    |              |             |
| `usage`      | string                                                    | false    |              |             |
| `value`      | array of [codersdk.GitAuthConfig](#codersdkgitauthconfig) | false    |              |             |

## codersdk.DeploymentConfigField-array_string

```json
{
  "default": ["string"],
  "enterprise": true,
  "flag": "string",
  "hidden": true,
  "name": "string",
  "secret": true,
  "shorthand": "string",
  "usage": "string",
  "value": ["string"]
}
```

### Properties

| Name         | Type            | Required | Restrictions | Description |
| ------------ | --------------- | -------- | ------------ | ----------- |
| `default`    | array of string | false    |              |             |
| `enterprise` | boolean         | false    |              |             |
| `flag`       | string          | false    |              |             |
| `hidden`     | boolean         | false    |              |             |
| `name`       | string          | false    |              |             |
| `secret`     | boolean         | false    |              |             |
| `shorthand`  | string          | false    |              |             |
| `usage`      | string          | false    |              |             |
| `value`      | array of string | false    |              |             |

## codersdk.DeploymentConfigField-bool

```json
{
  "default": true,
  "enterprise": true,
  "flag": "string",
  "hidden": true,
  "name": "string",
  "secret": true,
  "shorthand": "string",
  "usage": "string",
  "value": true
}
```

### Properties

| Name         | Type    | Required | Restrictions | Description |
| ------------ | ------- | -------- | ------------ | ----------- |
| `default`    | boolean | false    |              |             |
| `enterprise` | boolean | false    |              |             |
| `flag`       | string  | false    |              |             |
| `hidden`     | boolean | false    |              |             |
| `name`       | string  | false    |              |             |
| `secret`     | boolean | false    |              |             |
| `shorthand`  | string  | false    |              |             |
| `usage`      | string  | false    |              |             |
| `value`      | boolean | false    |              |             |

## codersdk.DeploymentConfigField-int

```json
{
  "default": 0,
  "enterprise": true,
  "flag": "string",
  "hidden": true,
  "name": "string",
  "secret": true,
  "shorthand": "string",
  "usage": "string",
  "value": 0
}
```

### Properties

| Name         | Type    | Required | Restrictions | Description |
| ------------ | ------- | -------- | ------------ | ----------- |
| `default`    | integer | false    |              |             |
| `enterprise` | boolean | false    |              |             |
| `flag`       | string  | false    |              |             |
| `hidden`     | boolean | false    |              |             |
| `name`       | string  | false    |              |             |
| `secret`     | boolean | false    |              |             |
| `shorthand`  | string  | false    |              |             |
| `usage`      | string  | false    |              |             |
| `value`      | integer | false    |              |             |

## codersdk.DeploymentConfigField-string

```json
{
  "default": "string",
  "enterprise": true,
  "flag": "string",
  "hidden": true,
  "name": "string",
  "secret": true,
  "shorthand": "string",
  "usage": "string",
  "value": "string"
}
```

### Properties

| Name         | Type    | Required | Restrictions | Description |
| ------------ | ------- | -------- | ------------ | ----------- |
| `default`    | string  | false    |              |             |
| `enterprise` | boolean | false    |              |             |
| `flag`       | string  | false    |              |             |
| `hidden`     | boolean | false    |              |             |
| `name`       | string  | false    |              |             |
| `secret`     | boolean | false    |              |             |
| `shorthand`  | string  | false    |              |             |
| `usage`      | string  | false    |              |             |
| `value`      | string  | false    |              |             |

## codersdk.DeploymentConfigField-time_Duration

```json
{
  "default": 0,
  "enterprise": true,
  "flag": "string",
  "hidden": true,
  "name": "string",
  "secret": true,
  "shorthand": "string",
  "usage": "string",
  "value": 0
}
```

### Properties

| Name         | Type    | Required | Restrictions | Description |
| ------------ | ------- | -------- | ------------ | ----------- |
| `default`    | integer | false    |              |             |
| `enterprise` | boolean | false    |              |             |
| `flag`       | string  | false    |              |             |
| `hidden`     | boolean | false    |              |             |
| `name`       | string  | false    |              |             |
| `secret`     | boolean | false    |              |             |
| `shorthand`  | string  | false    |              |             |
| `usage`      | string  | false    |              |             |
| `value`      | integer | false    |              |             |

## codersdk.DeploymentDAUsResponse

```json
{
  "entries": [
    {
      "amount": 0,
      "date": "2019-08-24T14:15:22Z"
    }
  ]
}
```

### Properties

| Name      | Type                                            | Required | Restrictions | Description |
| --------- | ----------------------------------------------- | -------- | ------------ | ----------- |
| `entries` | array of [codersdk.DAUEntry](#codersdkdauentry) | false    |              |             |

## codersdk.Entitlement

```json
"entitled"
```

### Properties

#### Enumerated Values

| Value          |
| -------------- |
| `entitled`     |
| `grace_period` |
| `not_entitled` |

## codersdk.Entitlements

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

### Properties

| Name                | Type                                 | Required | Restrictions | Description                           |
| ------------------- | ------------------------------------ | -------- | ------------ | ------------------------------------- |
| `errors`            | array of string                      | false    |              |                                       |
| `experimental`      | boolean                              | false    |              | Experimental use Experiments instead. |
| `features`          | object                               | false    |              |                                       |
| » `[any property]`  | [codersdk.Feature](#codersdkfeature) | false    |              |                                       |
| `has_license`       | boolean                              | false    |              |                                       |
| `require_telemetry` | boolean                              | false    |              |                                       |
| `trial`             | boolean                              | false    |              |                                       |
| `warnings`          | array of string                      | false    |              |                                       |

## codersdk.Experiment

```json
"authz_querier"
```

### Properties

#### Enumerated Values

| Value             |
| ----------------- |
| `authz_querier`   |
| `template_editor` |

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
| ------------- | -------------------------------------------- | -------- | ------------ | ----------- |
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
| ----- | ------ | -------- | ------------ | ----------- |
| `key` | string | false    |              |             |

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
```

### Properties

| Name    | Type                                    | Required | Restrictions | Description |
| ------- | --------------------------------------- | -------- | ------------ | ----------- |
| `count` | integer                                 | false    |              |             |
| `users` | array of [codersdk.User](#codersdkuser) | false    |              |             |

## codersdk.GitAuthConfig

```json
{
  "auth_url": "string",
  "client_id": "string",
  "id": "string",
  "no_refresh": true,
  "regex": "string",
  "scopes": ["string"],
  "token_url": "string",
  "type": "string",
  "validate_url": "string"
}
```

### Properties

| Name           | Type            | Required | Restrictions | Description |
| -------------- | --------------- | -------- | ------------ | ----------- |
| `auth_url`     | string          | false    |              |             |
| `client_id`    | string          | false    |              |             |
| `id`           | string          | false    |              |             |
| `no_refresh`   | boolean         | false    |              |             |
| `regex`        | string          | false    |              |             |
| `scopes`       | array of string | false    |              |             |
| `token_url`    | string          | false    |              |             |
| `type`         | string          | false    |              |             |
| `validate_url` | string          | false    |              |             |

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

| Name         | Type   | Required | Restrictions | Description |
| ------------ | ------ | -------- | ------------ | ----------- |
| `created_at` | string | false    |              |             |
| `public_key` | string | false    |              |             |
| `updated_at` | string | false    |              |             |
| `user_id`    | string | false    |              |             |

## codersdk.Group

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

### Properties

| Name              | Type                                    | Required | Restrictions | Description |
| ----------------- | --------------------------------------- | -------- | ------------ | ----------- |
| `avatar_url`      | string                                  | false    |              |             |
| `id`              | string                                  | false    |              |             |
| `members`         | array of [codersdk.User](#codersdkuser) | false    |              |             |
| `name`            | string                                  | false    |              |             |
| `organization_id` | string                                  | false    |              |             |
| `quota_allowance` | integer                                 | false    |              |             |

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
| ----------- | ------- | -------- | ------------ | ------------------------------------------------------------------------------------------------ |
| `interval`  | integer | false    |              | Interval specifies the seconds between each health check.                                        |
| `threshold` | integer | false    |              | Threshold specifies the number of consecutive failed health checks before returning "unhealthy". |
| `url`       | string  | false    |              | URL specifies the endpoint to check for the app health.                                          |

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

| Name          | Type    | Required | Restrictions | Description                                                                                                                                                                                            |
| ------------- | ------- | -------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `claims`      | object  | false    |              | Claims are the JWT claims asserted by the license. Here we use a generic string map to ensure that all data from the server is parsed verbatim, not just the fields this version of Coder understands. |
| `id`          | integer | false    |              |                                                                                                                                                                                                        |
| `uploaded_at` | string  | false    |              |                                                                                                                                                                                                        |
| `uuid`        | string  | false    |              |                                                                                                                                                                                                        |

## codersdk.LogLevel

```json
"trace"
```

### Properties

#### Enumerated Values

| Value   |
| ------- |
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
| -------------------- |
| `provisioner_daemon` |
| `provisioner`        |

## codersdk.LoggingConfig

```json
{
  "human": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "json": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "stackdriver": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  }
}
```

### Properties

| Name          | Type                                                                           | Required | Restrictions | Description |
| ------------- | ------------------------------------------------------------------------------ | -------- | ------------ | ----------- |
| `human`       | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string) | false    |              |             |
| `json`        | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string) | false    |              |             |
| `stackdriver` | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string) | false    |              |             |

## codersdk.LoginType

```json
"password"
```

### Properties

#### Enumerated Values

| Value      |
| ---------- |
| `password` |
| `github`   |
| `oidc`     |
| `token`    |

## codersdk.LoginWithPasswordRequest

```json
{
  "email": "user@example.com",
  "password": "string"
}
```

### Properties

| Name       | Type   | Required | Restrictions | Description |
| ---------- | ------ | -------- | ------------ | ----------- |
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
| --------------- | ------ | -------- | ------------ | ----------- |
| `session_token` | string | true     |              |             |

## codersdk.OAuth2Config

```json
{
  "github": {
    "allow_everyone": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    },
    "allow_signups": {
      "default": true,
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": true
    },
    "allowed_orgs": {
      "default": ["string"],
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": ["string"]
    },
    "allowed_teams": {
      "default": ["string"],
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": ["string"]
    },
    "client_id": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "client_secret": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    },
    "enterprise_base_url": {
      "default": "string",
      "enterprise": true,
      "flag": "string",
      "hidden": true,
      "name": "string",
      "secret": true,
      "shorthand": "string",
      "usage": "string",
      "value": "string"
    }
  }
}
```

### Properties

| Name     | Type                                                       | Required | Restrictions | Description |
| -------- | ---------------------------------------------------------- | -------- | ------------ | ----------- |
| `github` | [codersdk.OAuth2GithubConfig](#codersdkoauth2githubconfig) | false    |              |             |

## codersdk.OAuth2GithubConfig

```json
{
  "allow_everyone": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "allow_signups": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "allowed_orgs": {
    "default": ["string"],
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": ["string"]
  },
  "allowed_teams": {
    "default": ["string"],
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": ["string"]
  },
  "client_id": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "client_secret": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "enterprise_base_url": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  }
}
```

### Properties

| Name                  | Type                                                                                       | Required | Restrictions | Description |
| --------------------- | ------------------------------------------------------------------------------------------ | -------- | ------------ | ----------- |
| `allow_everyone`      | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)                 | false    |              |             |
| `allow_signups`       | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)                 | false    |              |             |
| `allowed_orgs`        | [codersdk.DeploymentConfigField-array_string](#codersdkdeploymentconfigfield-array_string) | false    |              |             |
| `allowed_teams`       | [codersdk.DeploymentConfigField-array_string](#codersdkdeploymentconfigfield-array_string) | false    |              |             |
| `client_id`           | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)             | false    |              |             |
| `client_secret`       | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)             | false    |              |             |
| `enterprise_base_url` | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)             | false    |              |             |

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
| ------------ | ------- | -------- | ------------ | ----------- |
| `enabled`    | boolean | false    |              |             |
| `iconUrl`    | string  | false    |              |             |
| `signInText` | string  | false    |              |             |

## codersdk.OIDCConfig

```json
{
  "allow_signups": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "client_id": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "client_secret": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "email_domain": {
    "default": ["string"],
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": ["string"]
  },
  "icon_url": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "ignore_email_verified": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "issuer_url": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "scopes": {
    "default": ["string"],
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": ["string"]
  },
  "sign_in_text": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "username_field": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  }
}
```

### Properties

| Name                    | Type                                                                                       | Required | Restrictions | Description |
| ----------------------- | ------------------------------------------------------------------------------------------ | -------- | ------------ | ----------- |
| `allow_signups`         | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)                 | false    |              |             |
| `client_id`             | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)             | false    |              |             |
| `client_secret`         | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)             | false    |              |             |
| `email_domain`          | [codersdk.DeploymentConfigField-array_string](#codersdkdeploymentconfigfield-array_string) | false    |              |             |
| `icon_url`              | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)             | false    |              |             |
| `ignore_email_verified` | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)                 | false    |              |             |
| `issuer_url`            | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)             | false    |              |             |
| `scopes`                | [codersdk.DeploymentConfigField-array_string](#codersdkdeploymentconfigfield-array_string) | false    |              |             |
| `sign_in_text`          | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)             | false    |              |             |
| `username_field`        | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)             | false    |              |             |

## codersdk.Organization

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Properties

| Name         | Type   | Required | Restrictions | Description |
| ------------ | ------ | -------- | ------------ | ----------- |
| `created_at` | string | true     |              |             |
| `id`         | string | true     |              |             |
| `name`       | string | true     |              |             |
| `updated_at` | string | true     |              |             |

## codersdk.OrganizationMember

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

### Properties

| Name              | Type                                    | Required | Restrictions | Description |
| ----------------- | --------------------------------------- | -------- | ------------ | ----------- |
| `created_at`      | string                                  | false    |              |             |
| `organization_id` | string                                  | false    |              |             |
| `roles`           | array of [codersdk.Role](#codersdkrole) | false    |              |             |
| `updated_at`      | string                                  | false    |              |             |
| `user_id`         | string                                  | false    |              |             |

## codersdk.Parameter

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "destination_scheme": "none",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "scope": "template",
  "scope_id": "5d3fe357-12dd-4f62-b004-6d1fb3b8454f",
  "source_scheme": "none",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

Parameter represents a set value for the scope.

### Properties

| Name                 | Type                                                                       | Required | Restrictions | Description |
| -------------------- | -------------------------------------------------------------------------- | -------- | ------------ | ----------- |
| `created_at`         | string                                                                     | false    |              |             |
| `destination_scheme` | [codersdk.ParameterDestinationScheme](#codersdkparameterdestinationscheme) | false    |              |             |
| `id`                 | string                                                                     | false    |              |             |
| `name`               | string                                                                     | false    |              |             |
| `scope`              | [codersdk.ParameterScope](#codersdkparameterscope)                         | false    |              |             |
| `scope_id`           | string                                                                     | false    |              |             |
| `source_scheme`      | [codersdk.ParameterSourceScheme](#codersdkparametersourcescheme)           | false    |              |             |
| `updated_at`         | string                                                                     | false    |              |             |

#### Enumerated Values

| Property             | Value                  |
| -------------------- | ---------------------- |
| `destination_scheme` | `none`                 |
| `destination_scheme` | `environment_variable` |
| `destination_scheme` | `provisioner_variable` |
| `scope`              | `template`             |
| `scope`              | `workspace`            |
| `scope`              | `import_job`           |
| `source_scheme`      | `none`                 |
| `source_scheme`      | `data`                 |

## codersdk.ParameterDestinationScheme

```json
"none"
```

### Properties

#### Enumerated Values

| Value                  |
| ---------------------- |
| `none`                 |
| `environment_variable` |
| `provisioner_variable` |

## codersdk.ParameterSchema

```json
{
  "allow_override_destination": true,
  "allow_override_source": true,
  "created_at": "2019-08-24T14:15:22Z",
  "default_destination_scheme": "none",
  "default_refresh": "string",
  "default_source_scheme": "none",
  "default_source_value": "string",
  "description": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "job_id": "453bd7d7-5355-4d6d-a38e-d9e7eb218c3f",
  "name": "string",
  "redisplay_value": true,
  "validation_condition": "string",
  "validation_contains": ["string"],
  "validation_error": "string",
  "validation_type_system": "string",
  "validation_value_type": "string"
}
```

### Properties

| Name                         | Type                                                                       | Required | Restrictions | Description                                                                                                             |
| ---------------------------- | -------------------------------------------------------------------------- | -------- | ------------ | ----------------------------------------------------------------------------------------------------------------------- |
| `allow_override_destination` | boolean                                                                    | false    |              |                                                                                                                         |
| `allow_override_source`      | boolean                                                                    | false    |              |                                                                                                                         |
| `created_at`                 | string                                                                     | false    |              |                                                                                                                         |
| `default_destination_scheme` | [codersdk.ParameterDestinationScheme](#codersdkparameterdestinationscheme) | false    |              |                                                                                                                         |
| `default_refresh`            | string                                                                     | false    |              |                                                                                                                         |
| `default_source_scheme`      | [codersdk.ParameterSourceScheme](#codersdkparametersourcescheme)           | false    |              |                                                                                                                         |
| `default_source_value`       | string                                                                     | false    |              |                                                                                                                         |
| `description`                | string                                                                     | false    |              |                                                                                                                         |
| `id`                         | string                                                                     | false    |              |                                                                                                                         |
| `job_id`                     | string                                                                     | false    |              |                                                                                                                         |
| `name`                       | string                                                                     | false    |              |                                                                                                                         |
| `redisplay_value`            | boolean                                                                    | false    |              |                                                                                                                         |
| `validation_condition`       | string                                                                     | false    |              |                                                                                                                         |
| `validation_contains`        | array of string                                                            | false    |              | This is a special array of items provided if the validation condition explicitly states the value must be one of a set. |
| `validation_error`           | string                                                                     | false    |              |                                                                                                                         |
| `validation_type_system`     | string                                                                     | false    |              |                                                                                                                         |
| `validation_value_type`      | string                                                                     | false    |              |                                                                                                                         |

#### Enumerated Values

| Property                     | Value                  |
| ---------------------------- | ---------------------- |
| `default_destination_scheme` | `none`                 |
| `default_destination_scheme` | `environment_variable` |
| `default_destination_scheme` | `provisioner_variable` |
| `default_source_scheme`      | `none`                 |
| `default_source_scheme`      | `data`                 |

## codersdk.ParameterScope

```json
"template"
```

### Properties

#### Enumerated Values

| Value        |
| ------------ |
| `template`   |
| `workspace`  |
| `import_job` |

## codersdk.ParameterSourceScheme

```json
"none"
```

### Properties

#### Enumerated Values

| Value  |
| ------ |
| `none` |
| `data` |

## codersdk.PprofConfig

```json
{
  "address": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "enable": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  }
}
```

### Properties

| Name      | Type                                                                           | Required | Restrictions | Description |
| --------- | ------------------------------------------------------------------------------ | -------- | ------------ | ----------- |
| `address` | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string) | false    |              |             |
| `enable`  | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)     | false    |              |             |

## codersdk.PrometheusConfig

```json
{
  "address": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "enable": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  }
}
```

### Properties

| Name      | Type                                                                           | Required | Restrictions | Description |
| --------- | ------------------------------------------------------------------------------ | -------- | ------------ | ----------- |
| `address` | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string) | false    |              |             |
| `enable`  | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)     | false    |              |             |

## codersdk.ProvisionerConfig

```json
{
  "daemon_poll_interval": {
    "default": 0,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": 0
  },
  "daemon_poll_jitter": {
    "default": 0,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": 0
  },
  "daemons": {
    "default": 0,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": 0
  },
  "force_cancel_interval": {
    "default": 0,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": 0
  }
}
```

### Properties

| Name                    | Type                                                                                         | Required | Restrictions | Description |
| ----------------------- | -------------------------------------------------------------------------------------------- | -------- | ------------ | ----------- |
| `daemon_poll_interval`  | [codersdk.DeploymentConfigField-time_Duration](#codersdkdeploymentconfigfield-time_duration) | false    |              |             |
| `daemon_poll_jitter`    | [codersdk.DeploymentConfigField-time_Duration](#codersdkdeploymentconfigfield-time_duration) | false    |              |             |
| `daemons`               | [codersdk.DeploymentConfigField-int](#codersdkdeploymentconfigfield-int)                     | false    |              |             |
| `force_cancel_interval` | [codersdk.DeploymentConfigField-time_Duration](#codersdkdeploymentconfigfield-time_duration) | false    |              |             |

## codersdk.ProvisionerDaemon

```json
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
```

### Properties

| Name               | Type                         | Required | Restrictions | Description |
| ------------------ | ---------------------------- | -------- | ------------ | ----------- |
| `created_at`       | string                       | false    |              |             |
| `id`               | string                       | false    |              |             |
| `name`             | string                       | false    |              |             |
| `provisioners`     | array of string              | false    |              |             |
| `tags`             | object                       | false    |              |             |
| » `[any property]` | string                       | false    |              |             |
| `updated_at`       | [sql.NullTime](#sqlnulltime) | false    |              |             |

## codersdk.ProvisionerJob

```json
{
  "canceled_at": "2019-08-24T14:15:22Z",
  "completed_at": "2019-08-24T14:15:22Z",
  "created_at": "2019-08-24T14:15:22Z",
  "error": "string",
  "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "started_at": "2019-08-24T14:15:22Z",
  "status": "pending",
  "tags": {
    "property1": "string",
    "property2": "string"
  },
  "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
}
```

### Properties

| Name               | Type                                                           | Required | Restrictions | Description |
| ------------------ | -------------------------------------------------------------- | -------- | ------------ | ----------- |
| `canceled_at`      | string                                                         | false    |              |             |
| `completed_at`     | string                                                         | false    |              |             |
| `created_at`       | string                                                         | false    |              |             |
| `error`            | string                                                         | false    |              |             |
| `file_id`          | string                                                         | false    |              |             |
| `id`               | string                                                         | false    |              |             |
| `started_at`       | string                                                         | false    |              |             |
| `status`           | [codersdk.ProvisionerJobStatus](#codersdkprovisionerjobstatus) | false    |              |             |
| `tags`             | object                                                         | false    |              |             |
| » `[any property]` | string                                                         | false    |              |             |
| `worker_id`        | string                                                         | false    |              |             |

#### Enumerated Values

| Property | Value       |
| -------- | ----------- |
| `status` | `pending`   |
| `status` | `running`   |
| `status` | `succeeded` |
| `status` | `canceling` |
| `status` | `canceled`  |
| `status` | `failed`    |

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
| ------------ | ---------------------------------------- | -------- | ------------ | ----------- |
| `created_at` | string                                   | false    |              |             |
| `id`         | integer                                  | false    |              |             |
| `log_level`  | [codersdk.LogLevel](#codersdkloglevel)   | false    |              |             |
| `log_source` | [codersdk.LogSource](#codersdklogsource) | false    |              |             |
| `output`     | string                                   | false    |              |             |
| `stage`      | string                                   | false    |              |             |

#### Enumerated Values

| Property    | Value   |
| ----------- | ------- |
| `log_level` | `trace` |
| `log_level` | `debug` |
| `log_level` | `info`  |
| `log_level` | `warn`  |
| `log_level` | `error` |

## codersdk.ProvisionerJobStatus

```json
"pending"
```

### Properties

#### Enumerated Values

| Value       |
| ----------- |
| `pending`   |
| `running`   |
| `succeeded` |
| `canceling` |
| `canceled`  |
| `failed`    |

## codersdk.PutExtendWorkspaceRequest

```json
{
  "deadline": "2019-08-24T14:15:22Z"
}
```

### Properties

| Name       | Type   | Required | Restrictions | Description |
| ---------- | ------ | -------- | ------------ | ----------- |
| `deadline` | string | true     |              |             |

## codersdk.RateLimitConfig

```json
{
  "api": {
    "default": 0,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": 0
  },
  "disable_all": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  }
}
```

### Properties

| Name          | Type                                                                       | Required | Restrictions | Description |
| ------------- | -------------------------------------------------------------------------- | -------- | ------------ | ----------- |
| `api`         | [codersdk.DeploymentConfigField-int](#codersdkdeploymentconfigfield-int)   | false    |              |             |
| `disable_all` | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool) | false    |              |             |

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
| ------------------ | ------- | -------- | ------------ | ------------------------------------------------------------------ |
| `created_at`       | string  | false    |              | Created at is the timestamp when the replica was first seen.       |
| `database_latency` | integer | false    |              | Database latency is the latency in microseconds to the database.   |
| `error`            | string  | false    |              | Error is the replica error.                                        |
| `hostname`         | string  | false    |              | Hostname is the hostname of the replica.                           |
| `id`               | string  | false    |              | ID is the unique identifier for the replica.                       |
| `region_id`        | integer | false    |              | Region ID is the region of the replica.                            |
| `relay_address`    | string  | false    |              | Relay address is the accessible address to relay DERP connections. |

## codersdk.ResourceType

```json
"template"
```

### Properties

#### Enumerated Values

| Value              |
| ------------------ |
| `template`         |
| `template_version` |
| `user`             |
| `workspace`        |
| `workspace_build`  |
| `git_ssh_key`      |
| `api_key`          |
| `group`            |
| `license`          |

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
| ------------- | ------------------------------------------------------------- | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `detail`      | string                                                        | false    |              | Detail is a debug message that provides further insight into why the action failed. This information can be technical and a regular golang err.Error() text. - "database: too many open connections" - "stat: too many open files" |
| `message`     | string                                                        | false    |              | Message is an actionable message that depicts actions the request took. These messages should be fully formed sentences with proper punctuation. Examples: - "A user has been created." - "Failed to create a user."               |
| `validations` | array of [codersdk.ValidationError](#codersdkvalidationerror) | false    |              | Validations are form field-specific friendly error messages. They will be shown on a form field in the UI. These can also be used to add additional context if there is a set of errors in the primary 'Message'.                  |

## codersdk.Role

```json
{
  "display_name": "string",
  "name": "string"
}
```

### Properties

| Name           | Type   | Required | Restrictions | Description |
| -------------- | ------ | -------- | ------------ | ----------- |
| `display_name` | string | false    |              |             |
| `name`         | string | false    |              |             |

## codersdk.ServiceBannerConfig

```json
{
  "background_color": "string",
  "enabled": true,
  "message": "string"
}
```

### Properties

| Name               | Type    | Required | Restrictions | Description |
| ------------------ | ------- | -------- | ------------ | ----------- |
| `background_color` | string  | false    |              |             |
| `enabled`          | boolean | false    |              |             |
| `message`          | string  | false    |              |             |

## codersdk.SwaggerConfig

```json
{
  "enable": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  }
}
```

### Properties

| Name     | Type                                                                       | Required | Restrictions | Description |
| -------- | -------------------------------------------------------------------------- | -------- | ------------ | ----------- |
| `enable` | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool) | false    |              |             |

## codersdk.TLSConfig

```json
{
  "address": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "cert_file": {
    "default": ["string"],
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": ["string"]
  },
  "client_auth": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "client_ca_file": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "client_cert_file": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "client_key_file": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "enable": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "key_file": {
    "default": ["string"],
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": ["string"]
  },
  "min_version": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  },
  "redirect_http": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  }
}
```

### Properties

| Name               | Type                                                                                       | Required | Restrictions | Description |
| ------------------ | ------------------------------------------------------------------------------------------ | -------- | ------------ | ----------- |
| `address`          | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)             | false    |              |             |
| `cert_file`        | [codersdk.DeploymentConfigField-array_string](#codersdkdeploymentconfigfield-array_string) | false    |              |             |
| `client_auth`      | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)             | false    |              |             |
| `client_ca_file`   | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)             | false    |              |             |
| `client_cert_file` | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)             | false    |              |             |
| `client_key_file`  | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)             | false    |              |             |
| `enable`           | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)                 | false    |              |             |
| `key_file`         | [codersdk.DeploymentConfigField-array_string](#codersdkdeploymentconfigfield-array_string) | false    |              |             |
| `min_version`      | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string)             | false    |              |             |
| `redirect_http`    | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)                 | false    |              |             |

## codersdk.TelemetryConfig

```json
{
  "enable": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "trace": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "url": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  }
}
```

### Properties

| Name     | Type                                                                           | Required | Restrictions | Description |
| -------- | ------------------------------------------------------------------------------ | -------- | ------------ | ----------- |
| `enable` | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)     | false    |              |             |
| `trace`  | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)     | false    |              |             |
| `url`    | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string) | false    |              |             |

## codersdk.Template

```json
{
  "active_user_count": 0,
  "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
  "allow_user_cancel_workspace_jobs": true,
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
  "description": "string",
  "display_name": "string",
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "provisioner": "terraform",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Properties

| Name                               | Type                                                               | Required | Restrictions | Description                                  |
| ---------------------------------- | ------------------------------------------------------------------ | -------- | ------------ | -------------------------------------------- |
| `active_user_count`                | integer                                                            | false    |              | Active user count is set to -1 when loading. |
| `active_version_id`                | string                                                             | false    |              |                                              |
| `allow_user_cancel_workspace_jobs` | boolean                                                            | false    |              |                                              |
| `build_time_stats`                 | [codersdk.TemplateBuildTimeStats](#codersdktemplatebuildtimestats) | false    |              |                                              |
| `created_at`                       | string                                                             | false    |              |                                              |
| `created_by_id`                    | string                                                             | false    |              |                                              |
| `created_by_name`                  | string                                                             | false    |              |                                              |
| `default_ttl_ms`                   | integer                                                            | false    |              |                                              |
| `description`                      | string                                                             | false    |              |                                              |
| `display_name`                     | string                                                             | false    |              |                                              |
| `icon`                             | string                                                             | false    |              |                                              |
| `id`                               | string                                                             | false    |              |                                              |
| `name`                             | string                                                             | false    |              |                                              |
| `organization_id`                  | string                                                             | false    |              |                                              |
| `provisioner`                      | string                                                             | false    |              |                                              |
| `updated_at`                       | string                                                             | false    |              |                                              |

#### Enumerated Values

| Property      | Value       |
| ------------- | ----------- |
| `provisioner` | `terraform` |

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
| ---------------- | ---------------------------------------------------- | -------- | ------------ | ----------- |
| `[any property]` | [codersdk.TransitionStats](#codersdktransitionstats) | false    |              |             |

## codersdk.TemplateDAUsResponse

```json
{
  "entries": [
    {
      "amount": 0,
      "date": "2019-08-24T14:15:22Z"
    }
  ]
}
```

### Properties

| Name      | Type                                            | Required | Restrictions | Description |
| --------- | ----------------------------------------------- | -------- | ------------ | ----------- |
| `entries` | array of [codersdk.DAUEntry](#codersdkdauentry) | false    |              |             |

## codersdk.TemplateExample

```json
{
  "description": "string",
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "markdown": "string",
  "name": "string",
  "tags": ["string"],
  "url": "string"
}
```

### Properties

| Name          | Type            | Required | Restrictions | Description |
| ------------- | --------------- | -------- | ------------ | ----------- |
| `description` | string          | false    |              |             |
| `icon`        | string          | false    |              |             |
| `id`          | string          | false    |              |             |
| `markdown`    | string          | false    |              |             |
| `name`        | string          | false    |              |             |
| `tags`        | array of string | false    |              |             |
| `url`         | string          | false    |              |             |

## codersdk.TemplateRole

```json
"admin"
```

### Properties

#### Enumerated Values

| Value   |
| ------- |
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
```

### Properties

| Name               | Type                                           | Required | Restrictions | Description |
| ------------------ | ---------------------------------------------- | -------- | ------------ | ----------- |
| `avatar_url`       | string                                         | false    |              |             |
| `created_at`       | string                                         | true     |              |             |
| `email`            | string                                         | true     |              |             |
| `id`               | string                                         | true     |              |             |
| `last_seen_at`     | string                                         | false    |              |             |
| `organization_ids` | array of string                                | false    |              |             |
| `role`             | [codersdk.TemplateRole](#codersdktemplaterole) | false    |              |             |
| `roles`            | array of [codersdk.Role](#codersdkrole)        | false    |              |             |
| `status`           | [codersdk.UserStatus](#codersdkuserstatus)     | false    |              |             |
| `username`         | string                                         | true     |              |             |

#### Enumerated Values

| Property | Value       |
| -------- | ----------- |
| `role`   | `admin`     |
| `role`   | `use`       |
| `status` | `active`    |
| `status` | `suspended` |

## codersdk.TemplateVersion

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "created_by": {
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
  },
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "job": {
    "canceled_at": "2019-08-24T14:15:22Z",
    "completed_at": "2019-08-24T14:15:22Z",
    "created_at": "2019-08-24T14:15:22Z",
    "error": "string",
    "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "started_at": "2019-08-24T14:15:22Z",
    "status": "pending",
    "tags": {
      "property1": "string",
      "property2": "string"
    },
    "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
  },
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "readme": "string",
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Properties

| Name              | Type                                               | Required | Restrictions | Description |
| ----------------- | -------------------------------------------------- | -------- | ------------ | ----------- |
| `created_at`      | string                                             | false    |              |             |
| `created_by`      | [codersdk.User](#codersdkuser)                     | false    |              |             |
| `id`              | string                                             | false    |              |             |
| `job`             | [codersdk.ProvisionerJob](#codersdkprovisionerjob) | false    |              |             |
| `name`            | string                                             | false    |              |             |
| `organization_id` | string                                             | false    |              |             |
| `readme`          | string                                             | false    |              |             |
| `template_id`     | string                                             | false    |              |             |
| `updated_at`      | string                                             | false    |              |             |

## codersdk.TemplateVersionParameter

```json
{
  "default_value": "string",
  "description": "string",
  "description_plaintext": "string",
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
| ----------------------- | ------------------------------------------------------------------------------------------- | -------- | ------------ | ----------- |
| `default_value`         | string                                                                                      | false    |              |             |
| `description`           | string                                                                                      | false    |              |             |
| `description_plaintext` | string                                                                                      | false    |              |             |
| `icon`                  | string                                                                                      | false    |              |             |
| `mutable`               | boolean                                                                                     | false    |              |             |
| `name`                  | string                                                                                      | false    |              |             |
| `options`               | array of [codersdk.TemplateVersionParameterOption](#codersdktemplateversionparameteroption) | false    |              |             |
| `type`                  | string                                                                                      | false    |              |             |
| `validation_error`      | string                                                                                      | false    |              |             |
| `validation_max`        | integer                                                                                     | false    |              |             |
| `validation_min`        | integer                                                                                     | false    |              |             |
| `validation_monotonic`  | [codersdk.ValidationMonotonicOrder](#codersdkvalidationmonotonicorder)                      | false    |              |             |
| `validation_regex`      | string                                                                                      | false    |              |             |

#### Enumerated Values

| Property               | Value        |
| ---------------------- | ------------ |
| `type`                 | `string`     |
| `type`                 | `number`     |
| `type`                 | `bool`       |
| `validation_monotonic` | `increasing` |
| `validation_monotonic` | `decreasing` |

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
| ------------- | ------ | -------- | ------------ | ----------- |
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
| --------------- | ------- | -------- | ------------ | ----------- |
| `default_value` | string  | false    |              |             |
| `description`   | string  | false    |              |             |
| `name`          | string  | false    |              |             |
| `required`      | boolean | false    |              |             |
| `sensitive`     | boolean | false    |              |             |
| `type`          | string  | false    |              |             |
| `value`         | string  | false    |              |             |

#### Enumerated Values

| Property | Value    |
| -------- | -------- |
| `type`   | `string` |
| `type`   | `number` |
| `type`   | `bool`   |

## codersdk.TraceConfig

```json
{
  "capture_logs": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "enable": {
    "default": true,
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": true
  },
  "honeycomb_api_key": {
    "default": "string",
    "enterprise": true,
    "flag": "string",
    "hidden": true,
    "name": "string",
    "secret": true,
    "shorthand": "string",
    "usage": "string",
    "value": "string"
  }
}
```

### Properties

| Name                | Type                                                                           | Required | Restrictions | Description |
| ------------------- | ------------------------------------------------------------------------------ | -------- | ------------ | ----------- |
| `capture_logs`      | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)     | false    |              |             |
| `enable`            | [codersdk.DeploymentConfigField-bool](#codersdkdeploymentconfigfield-bool)     | false    |              |             |
| `honeycomb_api_key` | [codersdk.DeploymentConfigField-string](#codersdkdeploymentconfigfield-string) | false    |              |             |

## codersdk.TransitionStats

```json
{
  "p50": 123,
  "p95": 146
}
```

### Properties

| Name  | Type    | Required | Restrictions | Description |
| ----- | ------- | -------- | ------------ | ----------- |
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
| ---- | ------ | -------- | ------------ | ----------- |
| `id` | string | true     |              |             |

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
| --------- | ------- | -------- | ------------ | ----------------------------------------------------------------------- |
| `current` | boolean | false    |              | Current indicates whether the server version is the same as the latest. |
| `url`     | string  | false    |              | URL to download the latest release of Coder.                            |
| `version` | string  | false    |              | Version is the semantic version for the latest release of Coder.        |

## codersdk.UpdateRoles

```json
{
  "roles": ["string"]
}
```

### Properties

| Name    | Type            | Required | Restrictions | Description |
| ------- | --------------- | -------- | ------------ | ----------- |
| `roles` | array of string | false    |              |             |

## codersdk.UpdateTemplateACL

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

### Properties

| Name               | Type                                           | Required | Restrictions | Description |
| ------------------ | ---------------------------------------------- | -------- | ------------ | ----------- |
| `group_perms`      | object                                         | false    |              |             |
| » `[any property]` | [codersdk.TemplateRole](#codersdktemplaterole) | false    |              |             |
| `user_perms`       | object                                         | false    |              |             |
| » `[any property]` | [codersdk.TemplateRole](#codersdktemplaterole) | false    |              |             |

## codersdk.UpdateUserPasswordRequest

```json
{
  "old_password": "string",
  "password": "string"
}
```

### Properties

| Name           | Type   | Required | Restrictions | Description |
| -------------- | ------ | -------- | ------------ | ----------- |
| `old_password` | string | false    |              |             |
| `password`     | string | true     |              |             |

## codersdk.UpdateUserProfileRequest

```json
{
  "username": "string"
}
```

### Properties

| Name       | Type   | Required | Restrictions | Description |
| ---------- | ------ | -------- | ------------ | ----------- |
| `username` | string | true     |              |             |

## codersdk.UpdateWorkspaceAutostartRequest

```json
{
  "schedule": "string"
}
```

### Properties

| Name       | Type   | Required | Restrictions | Description |
| ---------- | ------ | -------- | ------------ | ----------- |
| `schedule` | string | false    |              |             |

## codersdk.UpdateWorkspaceRequest

```json
{
  "name": "string"
}
```

### Properties

| Name   | Type   | Required | Restrictions | Description |
| ------ | ------ | -------- | ------------ | ----------- |
| `name` | string | false    |              |             |

## codersdk.UpdateWorkspaceTTLRequest

```json
{
  "ttl_ms": 0
}
```

### Properties

| Name     | Type    | Required | Restrictions | Description |
| -------- | ------- | -------- | ------------ | ----------- |
| `ttl_ms` | integer | false    |              |             |

## codersdk.UploadResponse

```json
{
  "hash": "19686d84-b10d-4f90-b18e-84fd3fa038fd"
}
```

### Properties

| Name   | Type   | Required | Restrictions | Description |
| ------ | ------ | -------- | ------------ | ----------- |
| `hash` | string | false    |              |             |

## codersdk.User

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

### Properties

| Name               | Type                                       | Required | Restrictions | Description |
| ------------------ | ------------------------------------------ | -------- | ------------ | ----------- |
| `avatar_url`       | string                                     | false    |              |             |
| `created_at`       | string                                     | true     |              |             |
| `email`            | string                                     | true     |              |             |
| `id`               | string                                     | true     |              |             |
| `last_seen_at`     | string                                     | false    |              |             |
| `organization_ids` | array of string                            | false    |              |             |
| `roles`            | array of [codersdk.Role](#codersdkrole)    | false    |              |             |
| `status`           | [codersdk.UserStatus](#codersdkuserstatus) | false    |              |             |
| `username`         | string                                     | true     |              |             |

#### Enumerated Values

| Property | Value       |
| -------- | ----------- |
| `status` | `active`    |
| `status` | `suspended` |

## codersdk.UserStatus

```json
"active"
```

### Properties

#### Enumerated Values

| Value       |
| ----------- |
| `active`    |
| `suspended` |

## codersdk.ValidationError

```json
{
  "detail": "string",
  "field": "string"
}
```

### Properties

| Name     | Type   | Required | Restrictions | Description |
| -------- | ------ | -------- | ------------ | ----------- |
| `detail` | string | true     |              |             |
| `field`  | string | true     |              |             |

## codersdk.ValidationMonotonicOrder

```json
"increasing"
```

### Properties

#### Enumerated Values

| Value        |
| ------------ |
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
| ------- | ------ | -------- | ------------ | ----------- |
| `name`  | string | false    |              |             |
| `value` | string | false    |              |             |

## codersdk.Workspace

```json
{
  "autostart_schedule": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "last_used_at": "2019-08-24T14:15:22Z",
  "latest_build": {
    "build_number": 0,
    "created_at": "2019-08-24T14:15:22Z",
    "daily_cost": 0,
    "deadline": "2019-08-24T14:15:22Z",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "initiator_id": "06588898-9a84-4b35-ba8f-f9cbd64946f3",
    "initiator_name": "string",
    "job": {
      "canceled_at": "2019-08-24T14:15:22Z",
      "completed_at": "2019-08-24T14:15:22Z",
      "created_at": "2019-08-24T14:15:22Z",
      "error": "string",
      "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "started_at": "2019-08-24T14:15:22Z",
      "status": "pending",
      "tags": {
        "property1": "string",
        "property2": "string"
      },
      "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
    },
    "reason": "initiator",
    "resources": [
      {
        "agents": [
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
    "workspace_owner_id": "e7078695-5279-4c86-8774-3ac2367a2fc7",
    "workspace_owner_name": "string"
  },
  "name": "string",
  "outdated": true,
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "owner_name": "string",
  "template_allow_user_cancel_workspace_jobs": true,
  "template_display_name": "string",
  "template_icon": "string",
  "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
  "template_name": "string",
  "ttl_ms": 0,
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Properties

| Name                                        | Type                                               | Required | Restrictions | Description |
| ------------------------------------------- | -------------------------------------------------- | -------- | ------------ | ----------- |
| `autostart_schedule`                        | string                                             | false    |              |             |
| `created_at`                                | string                                             | false    |              |             |
| `id`                                        | string                                             | false    |              |             |
| `last_used_at`                              | string                                             | false    |              |             |
| `latest_build`                              | [codersdk.WorkspaceBuild](#codersdkworkspacebuild) | false    |              |             |
| `name`                                      | string                                             | false    |              |             |
| `outdated`                                  | boolean                                            | false    |              |             |
| `owner_id`                                  | string                                             | false    |              |             |
| `owner_name`                                | string                                             | false    |              |             |
| `template_allow_user_cancel_workspace_jobs` | boolean                                            | false    |              |             |
| `template_display_name`                     | string                                             | false    |              |             |
| `template_icon`                             | string                                             | false    |              |             |
| `template_id`                               | string                                             | false    |              |             |
| `template_name`                             | string                                             | false    |              |             |
| `ttl_ms`                                    | integer                                            | false    |              |             |
| `updated_at`                                | string                                             | false    |              |             |

## codersdk.WorkspaceAgent

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

### Properties

| Name                             | Type                                                                 | Required | Restrictions | Description                                                                                                                                                                                                |
| -------------------------------- | -------------------------------------------------------------------- | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `apps`                           | array of [codersdk.WorkspaceApp](#codersdkworkspaceapp)              | false    |              |                                                                                                                                                                                                            |
| `architecture`                   | string                                                               | false    |              |                                                                                                                                                                                                            |
| `connection_timeout_seconds`     | integer                                                              | false    |              |                                                                                                                                                                                                            |
| `created_at`                     | string                                                               | false    |              |                                                                                                                                                                                                            |
| `directory`                      | string                                                               | false    |              |                                                                                                                                                                                                            |
| `disconnected_at`                | string                                                               | false    |              |                                                                                                                                                                                                            |
| `environment_variables`          | object                                                               | false    |              |                                                                                                                                                                                                            |
| » `[any property]`               | string                                                               | false    |              |                                                                                                                                                                                                            |
| `expanded_directory`             | string                                                               | false    |              |                                                                                                                                                                                                            |
| `first_connected_at`             | string                                                               | false    |              |                                                                                                                                                                                                            |
| `id`                             | string                                                               | false    |              |                                                                                                                                                                                                            |
| `instance_id`                    | string                                                               | false    |              |                                                                                                                                                                                                            |
| `last_connected_at`              | string                                                               | false    |              |                                                                                                                                                                                                            |
| `latency`                        | object                                                               | false    |              | Latency is mapped by region name (e.g. "New York City", "Seattle").                                                                                                                                        |
| » `[any property]`               | [codersdk.DERPRegion](#codersdkderpregion)                           | false    |              |                                                                                                                                                                                                            |
| `lifecycle_state`                | [codersdk.WorkspaceAgentLifecycle](#codersdkworkspaceagentlifecycle) | false    |              |                                                                                                                                                                                                            |
| `login_before_ready`             | boolean                                                              | false    |              | Login before ready if true, the agent will delay logins until it is ready (e.g. executing startup script has ended).                                                                                       |
| `name`                           | string                                                               | false    |              |                                                                                                                                                                                                            |
| `operating_system`               | string                                                               | false    |              |                                                                                                                                                                                                            |
| `resource_id`                    | string                                                               | false    |              |                                                                                                                                                                                                            |
| `startup_script`                 | string                                                               | false    |              |                                                                                                                                                                                                            |
| `startup_script_timeout_seconds` | integer                                                              | false    |              | Startup script timeout seconds is the number of seconds to wait for the startup script to complete. If the script does not complete within this time, the agent lifecycle will be marked as start_timeout. |
| `status`                         | [codersdk.WorkspaceAgentStatus](#codersdkworkspaceagentstatus)       | false    |              |                                                                                                                                                                                                            |
| `troubleshooting_url`            | string                                                               | false    |              |                                                                                                                                                                                                            |
| `updated_at`                     | string                                                               | false    |              |                                                                                                                                                                                                            |
| `version`                        | string                                                               | false    |              |                                                                                                                                                                                                            |

## codersdk.WorkspaceAgentConnectionInfo

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

### Properties

| Name       | Type                               | Required | Restrictions | Description |
| ---------- | ---------------------------------- | -------- | ------------ | ----------- |
| `derp_map` | [tailcfg.DERPMap](#tailcfgderpmap) | false    |              |             |

## codersdk.WorkspaceAgentLifecycle

```json
"created"
```

### Properties

#### Enumerated Values

| Value           |
| --------------- |
| `created`       |
| `starting`      |
| `start_timeout` |
| `start_error`   |
| `ready`         |

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
| -------------- | ------- | -------- | ------------ | ------------------------ |
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
| ------- | ------------------------------------------------------------------------------------- | -------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `ports` | array of [codersdk.WorkspaceAgentListeningPort](#codersdkworkspaceagentlisteningport) | false    |              | If there are no ports in the list, nothing should be displayed in the UI. There must not be a "no ports available" message or anything similar, as there will always be no ports displayed on platforms where our port detection logic is unsupported. |

## codersdk.WorkspaceAgentStatus

```json
"connecting"
```

### Properties

#### Enumerated Values

| Value          |
| -------------- |
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
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "sharing_level": "owner",
  "slug": "string",
  "subdomain": true,
  "url": "string"
}
```

### Properties

| Name            | Type                                                                   | Required | Restrictions | Description                                                                                                                                                                                                                                    |
| --------------- | ---------------------------------------------------------------------- | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `command`       | string                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `display_name`  | string                                                                 | false    |              | Display name is a friendly name for the app.                                                                                                                                                                                                   |
| `external`      | boolean                                                                | false    |              | External specifies whether the URL should be opened externally on the client or not.                                                                                                                                                           |
| `health`        | [codersdk.WorkspaceAppHealth](#codersdkworkspaceapphealth)             | false    |              |                                                                                                                                                                                                                                                |
| `healthcheck`   | [codersdk.Healthcheck](#codersdkhealthcheck)                           | false    |              | Healthcheck specifies the configuration for checking app health.                                                                                                                                                                               |
| `icon`          | string                                                                 | false    |              | Icon is a relative path or external URL that specifies an icon to be displayed in the dashboard.                                                                                                                                               |
| `id`            | string                                                                 | false    |              |                                                                                                                                                                                                                                                |
| `sharing_level` | [codersdk.WorkspaceAppSharingLevel](#codersdkworkspaceappsharinglevel) | false    |              |                                                                                                                                                                                                                                                |
| `slug`          | string                                                                 | false    |              | Slug is a unique identifier within the agent.                                                                                                                                                                                                  |
| `subdomain`     | boolean                                                                | false    |              | Subdomain denotes whether the app should be accessed via a path on the `coder server` or via a hostname-based dev URL. If this is set to true and there is no app wildcard configured on the server, the app will not be accessible in the UI. |
| `url`           | string                                                                 | false    |              | URL is the address being proxied to inside the workspace. If external is specified, this will be opened on the client.                                                                                                                         |

#### Enumerated Values

| Property        | Value           |
| --------------- | --------------- |
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
| -------------- |
| `disabled`     |
| `initializing` |
| `healthy`      |
| `unhealthy`    |

## codersdk.WorkspaceAppSharingLevel

```json
"owner"
```

### Properties

#### Enumerated Values

| Value           |
| --------------- |
| `owner`         |
| `authenticated` |
| `public`        |

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
    "canceled_at": "2019-08-24T14:15:22Z",
    "completed_at": "2019-08-24T14:15:22Z",
    "created_at": "2019-08-24T14:15:22Z",
    "error": "string",
    "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "started_at": "2019-08-24T14:15:22Z",
    "status": "pending",
    "tags": {
      "property1": "string",
      "property2": "string"
    },
    "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
  },
  "reason": "initiator",
  "resources": [
    {
      "agents": [
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
  "workspace_owner_id": "e7078695-5279-4c86-8774-3ac2367a2fc7",
  "workspace_owner_name": "string"
}
```

### Properties

| Name                    | Type                                                              | Required | Restrictions | Description |
| ----------------------- | ----------------------------------------------------------------- | -------- | ------------ | ----------- |
| `build_number`          | integer                                                           | false    |              |             |
| `created_at`            | string                                                            | false    |              |             |
| `daily_cost`            | integer                                                           | false    |              |             |
| `deadline`              | string                                                            | false    |              |             |
| `id`                    | string                                                            | false    |              |             |
| `initiator_id`          | string                                                            | false    |              |             |
| `initiator_name`        | string                                                            | false    |              |             |
| `job`                   | [codersdk.ProvisionerJob](#codersdkprovisionerjob)                | false    |              |             |
| `reason`                | [codersdk.BuildReason](#codersdkbuildreason)                      | false    |              |             |
| `resources`             | array of [codersdk.WorkspaceResource](#codersdkworkspaceresource) | false    |              |             |
| `status`                | [codersdk.WorkspaceStatus](#codersdkworkspacestatus)              | false    |              |             |
| `template_version_id`   | string                                                            | false    |              |             |
| `template_version_name` | string                                                            | false    |              |             |
| `transition`            | [codersdk.WorkspaceTransition](#codersdkworkspacetransition)      | false    |              |             |
| `updated_at`            | string                                                            | false    |              |             |
| `workspace_id`          | string                                                            | false    |              |             |
| `workspace_name`        | string                                                            | false    |              |             |
| `workspace_owner_id`    | string                                                            | false    |              |             |
| `workspace_owner_name`  | string                                                            | false    |              |             |

#### Enumerated Values

| Property     | Value       |
| ------------ | ----------- |
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
| ------- | ------ | -------- | ------------ | ----------- |
| `name`  | string | false    |              |             |
| `value` | string | false    |              |             |

## codersdk.WorkspaceQuota

```json
{
  "budget": 0,
  "credits_consumed": 0
}
```

### Properties

| Name               | Type    | Required | Restrictions | Description |
| ------------------ | ------- | -------- | ------------ | ----------- |
| `budget`           | integer | false    |              |             |
| `credits_consumed` | integer | false    |              |             |

## codersdk.WorkspaceResource

```json
{
  "agents": [
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
| ---------------------- | --------------------------------------------------------------------------------- | -------- | ------------ | ----------- |
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
| ---------------------- | -------- |
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
| ----------- | ------- | -------- | ------------ | ----------- |
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
| ----------- |
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
| -------- |
| `start`  |
| `stop`   |
| `delete` |

## codersdk.WorkspacesResponse

```json
{
  "count": 0,
  "workspaces": [
    {
      "autostart_schedule": "string",
      "created_at": "2019-08-24T14:15:22Z",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "last_used_at": "2019-08-24T14:15:22Z",
      "latest_build": {
        "build_number": 0,
        "created_at": "2019-08-24T14:15:22Z",
        "daily_cost": 0,
        "deadline": "2019-08-24T14:15:22Z",
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "initiator_id": "06588898-9a84-4b35-ba8f-f9cbd64946f3",
        "initiator_name": "string",
        "job": {
          "canceled_at": "2019-08-24T14:15:22Z",
          "completed_at": "2019-08-24T14:15:22Z",
          "created_at": "2019-08-24T14:15:22Z",
          "error": "string",
          "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
          "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
          "started_at": "2019-08-24T14:15:22Z",
          "status": "pending",
          "tags": {
            "property1": "string",
            "property2": "string"
          },
          "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
        },
        "reason": "initiator",
        "resources": [
          {
            "agents": [
              {
                "apps": [
                  {
                    "command": "string",
                    "display_name": "string",
                    "external": true,
                    "health": "disabled",
                    "healthcheck": {},
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
        "workspace_owner_id": "e7078695-5279-4c86-8774-3ac2367a2fc7",
        "workspace_owner_name": "string"
      },
      "name": "string",
      "outdated": true,
      "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
      "owner_name": "string",
      "template_allow_user_cancel_workspace_jobs": true,
      "template_display_name": "string",
      "template_icon": "string",
      "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
      "template_name": "string",
      "ttl_ms": 0,
      "updated_at": "2019-08-24T14:15:22Z"
    }
  ]
}
```

### Properties

| Name         | Type                                              | Required | Restrictions | Description |
| ------------ | ------------------------------------------------- | -------- | ------------ | ----------- |
| `count`      | integer                                           | false    |              |             |
| `workspaces` | array of [codersdk.Workspace](#codersdkworkspace) | false    |              |             |

## database.ParameterDestinationScheme

```json
"none"
```

### Properties

#### Enumerated Values

| Value                  |
| ---------------------- |
| `none`                 |
| `environment_variable` |
| `provisioner_variable` |

## database.ParameterScope

```json
"template"
```

### Properties

#### Enumerated Values

| Value        |
| ------------ |
| `template`   |
| `import_job` |
| `workspace`  |

## database.ParameterSourceScheme

```json
"none"
```

### Properties

#### Enumerated Values

| Value  |
| ------ |
| `none` |
| `data` |

## parameter.ComputedValue

```json
{
  "created_at": "string",
  "default_source_value": true,
  "destination_scheme": "none",
  "id": "string",
  "name": "string",
  "schema_id": "string",
  "scope": "template",
  "scope_id": "string",
  "source_scheme": "none",
  "source_value": "string",
  "updated_at": "string"
}
```

### Properties

| Name                   | Type                                                                       | Required | Restrictions | Description |
| ---------------------- | -------------------------------------------------------------------------- | -------- | ------------ | ----------- |
| `created_at`           | string                                                                     | false    |              |             |
| `default_source_value` | boolean                                                                    | false    |              |             |
| `destination_scheme`   | [database.ParameterDestinationScheme](#databaseparameterdestinationscheme) | false    |              |             |
| `id`                   | string                                                                     | false    |              |             |
| `name`                 | string                                                                     | false    |              |             |
| `schema_id`            | string                                                                     | false    |              |             |
| `scope`                | [database.ParameterScope](#databaseparameterscope)                         | false    |              |             |
| `scope_id`             | string                                                                     | false    |              |             |
| `source_scheme`        | [database.ParameterSourceScheme](#databaseparametersourcescheme)           | false    |              |             |
| `source_value`         | string                                                                     | false    |              |             |
| `updated_at`           | string                                                                     | false    |              |             |

## sql.NullTime

```json
{
  "time": "string",
  "valid": true
}
```

### Properties

| Name    | Type    | Required | Restrictions | Description                       |
| ------- | ------- | -------- | ------------ | --------------------------------- |
| `time`  | string  | false    |              |                                   |
| `valid` | boolean | false    |              | Valid is true if Time is not NULL |

## tailcfg.DERPMap

```json
{
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
```

### Properties

| Name                 | Type    | Required | Restrictions | Description                                                                                                                                                                    |
| -------------------- | ------- | -------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `omitDefaultRegions` | boolean | false    |              | Omitdefaultregions specifies to not use Tailscale's DERP servers, and only use those specified in this DERPMap. If there are none set outside of the defaults, this is a noop. |
| `regions`            | object  | false    |              | Regions is the set of geographic regions running DERP node(s).                                                                                                                 |

It's keyed by the DERPRegion.RegionID.
The numbers are not necessarily contiguous.|
|» `[any property]`|[tailcfg.DERPRegion](#tailcfgderpregion)|false|||

## tailcfg.DERPNode

```json
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
```

### Properties

| Name                                                                                                                  | Type    | Required | Restrictions | Description                                                                                                                                                                                                                                                       |
| --------------------------------------------------------------------------------------------------------------------- | ------- | -------- | ------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `certName`                                                                                                            | string  | false    |              | Certname optionally specifies the expected TLS cert common name. If empty, HostName is used. If CertName is non-empty, HostName is only used for the TCP dial (if IPv4/IPv6 are not present) + TLS ClientHello.                                                   |
| `derpport`                                                                                                            | integer | false    |              | Derpport optionally provides an alternate TLS port number for the DERP HTTPS server.                                                                                                                                                                              |
| If zero, 443 is used.                                                                                                 |
| `forceHTTP`                                                                                                           | boolean | false    |              | Forcehttp is used by unit tests to force HTTP. It should not be set by users.                                                                                                                                                                                     |
| `hostName`                                                                                                            | string  | false    |              | Hostname is the DERP node's hostname.                                                                                                                                                                                                                             |
| It is required but need not be unique; multiple nodes may have the same HostName but vary in configuration otherwise. |
| `insecureForTests`                                                                                                    | boolean | false    |              | Insecurefortests is used by unit tests to disable TLS verification. It should not be set by users.                                                                                                                                                                |
| `ipv4`                                                                                                                | string  | false    |              | Ipv4 optionally forces an IPv4 address to use, instead of using DNS. If empty, A record(s) from DNS lookups of HostName are used. If the string is not an IPv4 address, IPv4 is not used; the conventional string to disable IPv4 (and not use DNS) is "none".    |
| `ipv6`                                                                                                                | string  | false    |              | Ipv6 optionally forces an IPv6 address to use, instead of using DNS. If empty, AAAA record(s) from DNS lookups of HostName are used. If the string is not an IPv6 address, IPv6 is not used; the conventional string to disable IPv6 (and not use DNS) is "none". |
| `name`                                                                                                                | string  | false    |              | Name is a unique node name (across all regions). It is not a host name. It's typically of the form "1b", "2a", "3b", etc. (region ID + suffix within that region)                                                                                                 |
| `regionID`                                                                                                            | integer | false    |              | Regionid is the RegionID of the DERPRegion that this node is running in.                                                                                                                                                                                          |
| `stunonly`                                                                                                            | boolean | false    |              | Stunonly marks a node as only a STUN server and not a DERP server.                                                                                                                                                                                                |
| `stunport`                                                                                                            | integer | false    |              | Port optionally specifies a STUN port to use. Zero means 3478. To disable STUN on this node, use -1.                                                                                                                                                              |
| `stuntestIP`                                                                                                          | string  | false    |              | Stuntestip is used in tests to override the STUN server's IP. If empty, it's assumed to be the same as the DERP server.                                                                                                                                           |

## tailcfg.DERPRegion

```json
{
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
```

### Properties

| Name                                                                                                                                                                                                                                                                                                        | Type                                          | Required | Restrictions | Description                                                                                                                                                                                                                                        |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------- | -------- | ------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `avoid`                                                                                                                                                                                                                                                                                                     | boolean                                       | false    |              | Avoid is whether the client should avoid picking this as its home region. The region should only be used if a peer is there. Clients already using this region as their home should migrate away to a new region without Avoid set.                |
| `embeddedRelay`                                                                                                                                                                                                                                                                                             | boolean                                       | false    |              | Embeddedrelay is true when the region is bundled with the Coder control plane.                                                                                                                                                                     |
| `nodes`                                                                                                                                                                                                                                                                                                     | array of [tailcfg.DERPNode](#tailcfgderpnode) | false    |              | Nodes are the DERP nodes running in this region, in priority order for the current client. Client TLS connections should ideally only go to the first entry (falling back to the second if necessary). STUN packets should go to the first 1 or 2. |
| If nodes within a region route packets amongst themselves, but not to other regions. That said, each user/domain should get a the same preferred node order, so if all nodes for a user/network pick the first one (as they should, when things are healthy), the inter-cluster routing is minimal to zero. |
| `regionCode`                                                                                                                                                                                                                                                                                                | string                                        | false    |              | Regioncode is a short name for the region. It's usually a popular city or airport code in the region: "nyc", "sf", "sin", "fra", etc.                                                                                                              |
| `regionID`                                                                                                                                                                                                                                                                                                  | integer                                       | false    |              | Regionid is a unique integer for a geographic region.                                                                                                                                                                                              |

It corresponds to the legacy derpN.tailscale.com hostnames used by older clients. (Older clients will continue to resolve derpN.tailscale.com when contacting peers, rather than use the server-provided DERPMap)
RegionIDs must be non-zero, positive, and guaranteed to fit in a JavaScript number.
RegionIDs in range 900-999 are reserved for end users to run their own DERP nodes.|
|`regionName`|string|false||Regionname is a long English name for the region: "New York City", "San Francisco", "Singapore", "Frankfurt", etc.|
