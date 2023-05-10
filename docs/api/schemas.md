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

## agentsdk.AgentMetric

```json
{
  "name": "string",
  "type": "counter",
  "value": 0
}
```

### Properties

| Name    | Type                                                 | Required | Restrictions | Description |
| ------- | ---------------------------------------------------- | -------- | ------------ | ----------- |
| `name`  | string                                               | true     |              |             |
| `type`  | [agentsdk.AgentMetricType](#agentsdkagentmetrictype) | true     |              |             |
| `value` | number                                               | true     |              |             |

#### Enumerated Values

| Property | Value     |
| -------- | --------- |
| `type`   | `counter` |
| `type`   | `gauge`   |

## agentsdk.AgentMetricType

```json
"counter"
```

### Properties

#### Enumerated Values

| Value     |
| --------- |
| `counter` |
| `gauge`   |

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

## agentsdk.Manifest

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
  "metadata": [
    {
      "display_name": "string",
      "interval": 0,
      "key": "string",
      "script": "string",
      "timeout": 0
    }
  ],
  "motd_file": "string",
  "shutdown_script": "string",
  "shutdown_script_timeout": 0,
  "startup_script": "string",
  "startup_script_timeout": 0,
  "vscode_port_proxy_uri": "string"
}
```

### Properties

| Name                      | Type                                                                                              | Required | Restrictions | Description                                                                                                                                                |
| ------------------------- | ------------------------------------------------------------------------------------------------- | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `apps`                    | array of [codersdk.WorkspaceApp](#codersdkworkspaceapp)                                           | false    |              |                                                                                                                                                            |
| `derpmap`                 | [tailcfg.DERPMap](#tailcfgderpmap)                                                                | false    |              |                                                                                                                                                            |
| `directory`               | string                                                                                            | false    |              |                                                                                                                                                            |
| `environment_variables`   | object                                                                                            | false    |              |                                                                                                                                                            |
| » `[any property]`        | string                                                                                            | false    |              |                                                                                                                                                            |
| `git_auth_configs`        | integer                                                                                           | false    |              | Git auth configs stores the number of Git configurations the Coder deployment has. If this number is >0, we set up special configuration in the workspace. |
| `metadata`                | array of [codersdk.WorkspaceAgentMetadataDescription](#codersdkworkspaceagentmetadatadescription) | false    |              |                                                                                                                                                            |
| `motd_file`               | string                                                                                            | false    |              |                                                                                                                                                            |
| `shutdown_script`         | string                                                                                            | false    |              |                                                                                                                                                            |
| `shutdown_script_timeout` | integer                                                                                           | false    |              |                                                                                                                                                            |
| `startup_script`          | string                                                                                            | false    |              |                                                                                                                                                            |
| `startup_script_timeout`  | integer                                                                                           | false    |              |                                                                                                                                                            |
| `vscode_port_proxy_uri`   | string                                                                                            | false    |              |                                                                                                                                                            |

## agentsdk.PatchStartupLogs

```json
{
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

| Name   | Type                                                | Required | Restrictions | Description |
| ------ | --------------------------------------------------- | -------- | ------------ | ----------- |
| `logs` | array of [agentsdk.StartupLog](#agentsdkstartuplog) | false    |              |             |

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

## agentsdk.PostMetadataRequest

```json
{
  "age": 0,
  "collected_at": "2019-08-24T14:15:22Z",
  "error": "string",
  "value": "string"
}
```

### Properties

| Name           | Type    | Required | Restrictions | Description                                                                                                                             |
| -------------- | ------- | -------- | ------------ | --------------------------------------------------------------------------------------------------------------------------------------- |
| `age`          | integer | false    |              | Age is the number of seconds since the metadata was collected. It is provided in addition to CollectedAt to protect against clock skew. |
| `collected_at` | string  | false    |              |                                                                                                                                         |
| `error`        | string  | false    |              |                                                                                                                                         |
| `value`        | string  | false    |              |                                                                                                                                         |

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

## agentsdk.StartupLog

```json
{
  "created_at": "string",
  "level": "trace",
  "output": "string"
}
```

### Properties

| Name         | Type                                   | Required | Restrictions | Description |
| ------------ | -------------------------------------- | -------- | ------------ | ----------- |
| `created_at` | string                                 | false    |              |             |
| `level`      | [codersdk.LogLevel](#codersdkloglevel) | false    |              |             |
| `output`     | string                                 | false    |              |             |

## agentsdk.Stats

```json
{
  "connection_count": 0,
  "connection_median_latency_ms": 0,
  "connections_by_proto": {
    "property1": 0,
    "property2": 0
  },
  "metrics": [
    {
      "name": "string",
      "type": "counter",
      "value": 0
    }
  ],
  "rx_bytes": 0,
  "rx_packets": 0,
  "session_count_jetbrains": 0,
  "session_count_reconnecting_pty": 0,
  "session_count_ssh": 0,
  "session_count_vscode": 0,
  "tx_bytes": 0,
  "tx_packets": 0
}
```

### Properties

| Name                             | Type                                                  | Required | Restrictions | Description                                                                                                                   |
| -------------------------------- | ----------------------------------------------------- | -------- | ------------ | ----------------------------------------------------------------------------------------------------------------------------- |
| `connection_count`               | integer                                               | false    |              | Connection count is the number of connections received by an agent.                                                           |
| `connection_median_latency_ms`   | number                                                | false    |              | Connection median latency ms is the median latency of all connections in milliseconds.                                        |
| `connections_by_proto`           | object                                                | false    |              | Connections by proto is a count of connections by protocol.                                                                   |
| » `[any property]`               | integer                                               | false    |              |                                                                                                                               |
| `metrics`                        | array of [agentsdk.AgentMetric](#agentsdkagentmetric) | false    |              | Metrics collected by the agent                                                                                                |
| `rx_bytes`                       | integer                                               | false    |              | Rx bytes is the number of received bytes.                                                                                     |
| `rx_packets`                     | integer                                               | false    |              | Rx packets is the number of received packets.                                                                                 |
| `session_count_jetbrains`        | integer                                               | false    |              | Session count jetbrains is the number of connections received by an agent that are from our JetBrains extension.              |
| `session_count_reconnecting_pty` | integer                                               | false    |              | Session count reconnecting pty is the number of connections received by an agent that are from the reconnecting web terminal. |
| `session_count_ssh`              | integer                                               | false    |              | Session count ssh is the number of connections received by an agent that are normal, non-tagged SSH sessions.                 |
| `session_count_vscode`           | integer                                               | false    |              | Session count vscode is the number of connections received by an agent that are from our VS Code extension.                   |
| `tx_bytes`                       | integer                                               | false    |              | Tx bytes is the number of transmitted bytes.                                                                                  |
| `tx_packets`                     | integer                                               | false    |              | Tx packets is the number of transmitted bytes.                                                                                |

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

## clibase.Annotations

```json
{
  "property1": "string",
  "property2": "string"
}
```

### Properties

| Name             | Type   | Required | Restrictions | Description |
| ---------------- | ------ | -------- | ------------ | ----------- |
| `[any property]` | string | false    |              |             |

## clibase.Group

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
| ------------- | ------------------------------ | -------- | ------------ | ----------- |
| `description` | string                         | false    |              |             |
| `name`        | string                         | false    |              |             |
| `parent`      | [clibase.Group](#clibasegroup) | false    |              |             |
| `yaml`        | string                         | false    |              |             |

## clibase.HostPort

```json
{
  "host": "string",
  "port": "string"
}
```

### Properties

| Name   | Type   | Required | Restrictions | Description |
| ------ | ------ | -------- | ------------ | ----------- |
| `host` | string | false    |              |             |
| `port` | string | false    |              |             |

## clibase.Option

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

| Name             | Type                                       | Required | Restrictions | Description                                                                                                                    |
| ---------------- | ------------------------------------------ | -------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------ |
| `annotations`    | [clibase.Annotations](#clibaseannotations) | false    |              | Annotations enable extensions to clibase higher up in the stack. It's useful for help formatting and documentation generation. |
| `default`        | string                                     | false    |              | Default is parsed into Value if set.                                                                                           |
| `description`    | string                                     | false    |              |                                                                                                                                |
| `env`            | string                                     | false    |              | Env is the environment variable used to configure this option. If unset, environment configuring is disabled.                  |
| `flag`           | string                                     | false    |              | Flag is the long name of the flag used to configure this option. If unset, flag configuring is disabled.                       |
| `flag_shorthand` | string                                     | false    |              | Flag shorthand is the one-character shorthand for the flag. If unset, no shorthand is used.                                    |
| `group`          | [clibase.Group](#clibasegroup)             | false    |              | Group is a group hierarchy that helps organize this option in help, configs and other documentation.                           |
| `hidden`         | boolean                                    | false    |              |                                                                                                                                |
| `name`           | string                                     | false    |              |                                                                                                                                |
| `use_instead`    | array of [clibase.Option](#clibaseoption)  | false    |              | Use instead is a list of options that should be used instead of this one. The field is used to generate a deprecation warning. |
| `value`          | any                                        | false    |              | Value includes the types listed in values.go.                                                                                  |
| `value_source`   | [clibase.ValueSource](#clibasevaluesource) | false    |              |                                                                                                                                |
| `yaml`           | string                                     | false    |              | Yaml is the YAML key used to configure this option. If unset, YAML configuring is disabled.                                    |

## clibase.Struct-array_codersdk_GitAuthConfig

```json
{
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

| Name    | Type                                                      | Required | Restrictions | Description |
| ------- | --------------------------------------------------------- | -------- | ------------ | ----------- |
| `value` | array of [codersdk.GitAuthConfig](#codersdkgitauthconfig) | false    |              |             |

## clibase.Struct-array_codersdk_LinkConfig

```json
{
  "value": [
    {
      "icon": "string",
      "name": "string",
      "target": "string"
    }
  ]
}
```

### Properties

| Name    | Type                                                | Required | Restrictions | Description |
| ------- | --------------------------------------------------- | -------- | ------------ | ----------- |
| `value` | array of [codersdk.LinkConfig](#codersdklinkconfig) | false    |              |             |

## clibase.URL

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
| ------------- | ---------------------------- | -------- | ------------ | -------------------------------------------------- |
| `forceQuery`  | boolean                      | false    |              | append a query ('?') even if RawQuery is empty     |
| `fragment`    | string                       | false    |              | fragment for references, without '#'               |
| `host`        | string                       | false    |              | host or host:port                                  |
| `omitHost`    | boolean                      | false    |              | do not emit empty host (authority)                 |
| `opaque`      | string                       | false    |              | encoded opaque data                                |
| `path`        | string                       | false    |              | path (relative paths may omit leading slash)       |
| `rawFragment` | string                       | false    |              | encoded fragment hint (see EscapedFragment method) |
| `rawPath`     | string                       | false    |              | encoded path hint (see EscapedPath method)         |
| `rawQuery`    | string                       | false    |              | encoded query values, without '?'                  |
| `scheme`      | string                       | false    |              |                                                    |
| `user`        | [url.Userinfo](#urluserinfo) | false    |              | username and password information                  |

## clibase.ValueSource

```json
""
```

### Properties

#### Enumerated Values

| Value     |
| --------- |
| ``        |
| `flag`    |
| `env`     |
| `yaml`    |
| `default` |

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
  "token_name": "string",
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
| `token_name`       | string                                       | true     |              |             |
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

### Properties

| Name             | Type                                                         | Required | Restrictions | Description |
| ---------------- | ------------------------------------------------------------ | -------- | ------------ | ----------- |
| `logo_url`       | string                                                       | false    |              |             |
| `service_banner` | [codersdk.ServiceBannerConfig](#codersdkservicebannerconfig) | false    |              |             |
| `support_links`  | array of [codersdk.LinkConfig](#codersdklinkconfig)          | false    |              |             |

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

| Value      |
| ---------- |
| `create`   |
| `write`    |
| `delete`   |
| `start`    |
| `stop`     |
| `login`    |
| `logout`   |
| `register` |

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
    "resource_type": "workspace"
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
  "resource_type": "workspace"
}
```

AuthorizationObject can represent a "set" of objects, such as: all workspaces in an organization, all workspaces owned by me, all workspaces across the entire product.

### Properties

| Name              | Type                                           | Required | Restrictions | Description                                                                                                                                                                                                                                                                                                                                                          |
| ----------------- | ---------------------------------------------- | -------- | ------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
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
        "organization_id": "string",
        "owner_id": "string",
        "resource_id": "string",
        "resource_type": "workspace"
      }
    },
    "property2": {
      "action": "create",
      "object": {
        "organization_id": "string",
        "owner_id": "string",
        "resource_id": "string",
        "resource_type": "workspace"
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
  "dashboard_url": "string",
  "external_url": "string",
  "version": "string",
  "workspace_proxy": true
}
```

### Properties

| Name              | Type    | Required | Restrictions | Description                                                                                                                                                         |
| ----------------- | ------- | -------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `dashboard_url`   | string  | false    |              | Dashboard URL is the URL to hit the deployment's dashboard. For external workspace proxies, this is the coderd they are connected to.                               |
| `external_url`    | string  | false    |              | External URL references the current Coder version. For production builds, this will link directly to a release. For development builds, this will link to a commit. |
| `version`         | string  | false    |              | Version returns the semantic version of the build.                                                                                                                  |
| `workspace_proxy` | boolean | false    |              |                                                                                                                                                                     |

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
  "allow_user_autostart": true,
  "allow_user_autostop": true,
  "allow_user_cancel_workspace_jobs": true,
  "default_ttl_ms": 0,
  "description": "string",
  "display_name": "string",
  "failure_ttl_ms": 0,
  "icon": "string",
  "inactivity_ttl_ms": 0,
  "max_ttl_ms": 0,
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

| Name                                                                                                                                                                                      | Type                                                                        | Required | Restrictions | Description                                                                                                                                                                                                                                           |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------- | -------- | ------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `allow_user_autostart`                                                                                                                                                                    | boolean                                                                     | false    |              | Allow user autostart allows users to set a schedule for autostarting their workspace. By default this is true. This can only be disabled when using an enterprise license.                                                                            |
| `allow_user_autostop`                                                                                                                                                                     | boolean                                                                     | false    |              | Allow user autostop allows users to set a custom workspace TTL to use in place of the template's DefaultTTL field. By default this is true. If false, the DefaultTTL will always be used. This can only be disabled when using an enterprise license. |
| `allow_user_cancel_workspace_jobs`                                                                                                                                                        | boolean                                                                     | false    |              | Allow users to cancel in-progress workspace jobs. \*bool as the default value is "true".                                                                                                                                                              |
| `default_ttl_ms`                                                                                                                                                                          | integer                                                                     | false    |              | Default ttl ms allows optionally specifying the default TTL for all workspaces created from this template.                                                                                                                                            |
| `description`                                                                                                                                                                             | string                                                                      | false    |              | Description is a description of what the template contains. It must be less than 128 bytes.                                                                                                                                                           |
| `display_name`                                                                                                                                                                            | string                                                                      | false    |              | Display name is the displayed name of the template.                                                                                                                                                                                                   |
| `failure_ttl_ms`                                                                                                                                                                          | integer                                                                     | false    |              | Failure ttl ms allows optionally specifying the max lifetime before Coder stops all resources for failed workspaces created from this template.                                                                                                       |
| `icon`                                                                                                                                                                                    | string                                                                      | false    |              | Icon is a relative path or external URL that specifies an icon to be displayed in the dashboard.                                                                                                                                                      |
| `inactivity_ttl_ms`                                                                                                                                                                       | integer                                                                     | false    |              | Inactivity ttl ms allows optionally specifying the max lifetime before Coder deletes inactive workspaces created from this template.                                                                                                                  |
| `max_ttl_ms`                                                                                                                                                                              | integer                                                                     | false    |              | Max ttl ms allows optionally specifying the max lifetime for workspaces created from this template.                                                                                                                                                   |
| `name`                                                                                                                                                                                    | string                                                                      | true     |              | Name is the name of the template.                                                                                                                                                                                                                     |
| `parameter_values`                                                                                                                                                                        | array of [codersdk.CreateParameterRequest](#codersdkcreateparameterrequest) | false    |              | Parameter values is a structure used to create a new parameter value for a scope.]                                                                                                                                                                    |
| `template_version_id`                                                                                                                                                                     | string                                                                      | true     |              | Template version ID is an in-progress or completed job to use as an initial version of the template.                                                                                                                                                  |
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

## codersdk.CreateTemplateVersionRequest

```json
{
  "example_id": "string",
  "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
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

| Name                   | Type                                                                        | Required | Restrictions | Description                                                                                          |
| ---------------------- | --------------------------------------------------------------------------- | -------- | ------------ | ---------------------------------------------------------------------------------------------------- |
| `example_id`           | string                                                                      | false    |              |                                                                                                      |
| `file_id`              | string                                                                      | false    |              |                                                                                                      |
| `name`                 | string                                                                      | false    |              |                                                                                                      |
| `parameter_values`     | array of [codersdk.CreateParameterRequest](#codersdkcreateparameterrequest) | false    |              | Parameter values allows for additional parameters to be provided during the dry-run provision stage. |
| `provisioner`          | string                                                                      | true     |              |                                                                                                      |
| `storage_method`       | [codersdk.ProvisionerStorageMethod](#codersdkprovisionerstoragemethod)      | true     |              |                                                                                                      |
| `tags`                 | object                                                                      | false    |              |                                                                                                      |
| » `[any property]`     | string                                                                      | false    |              |                                                                                                      |
| `template_id`          | string                                                                      | false    |              | Template ID optionally associates a version with a template.                                         |
| `user_variable_values` | array of [codersdk.VariableValue](#codersdkvariablevalue)                   | false    |              |                                                                                                      |

#### Enumerated Values

| Property         | Value       |
| ---------------- | ----------- |
| `provisioner`    | `terraform` |
| `provisioner`    | `echo`      |
| `storage_method` | `file`      |

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
  "scope": "all",
  "token_name": "string"
}
```

### Properties

| Name         | Type                                         | Required | Restrictions | Description |
| ------------ | -------------------------------------------- | -------- | ------------ | ----------- |
| `lifetime`   | integer                                      | false    |              |             |
| `scope`      | [codersdk.APIKeyScope](#codersdkapikeyscope) | false    |              |             |
| `token_name` | string                                       | false    |              |             |

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
  "log_level": "debug",
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
| `log_level`             | [codersdk.ProvisionerLogLevel](#codersdkprovisionerloglevel)                  | false    |              | Log level changes the default logging verbosity of a provider ("info" if empty).                                                                                                                         |
| `orphan`                | boolean                                                                       | false    |              | Orphan may be set for the Destroy transition.                                                                                                                                                            |
| `parameter_values`      | array of [codersdk.CreateParameterRequest](#codersdkcreateparameterrequest)   | false    |              | Parameter values are optional. It will write params to the 'workspace' scope. This will overwrite any existing parameters with the same name. This will not delete old params not included in this list. |
| `rich_parameter_values` | array of [codersdk.WorkspaceBuildParameter](#codersdkworkspacebuildparameter) | false    |              |                                                                                                                                                                                                          |
| `state`                 | array of integer                                                              | false    |              |                                                                                                                                                                                                          |
| `template_version_id`   | string                                                                        | false    |              |                                                                                                                                                                                                          |
| `transition`            | [codersdk.WorkspaceTransition](#codersdkworkspacetransition)                  | true     |              |                                                                                                                                                                                                          |

#### Enumerated Values

| Property     | Value    |
| ------------ | -------- |
| `log_level`  | `debug`  |
| `transition` | `create` |
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
| -------------- | ------ | -------- | ------------ | ----------- |
| `display_name` | string | false    |              |             |
| `icon`         | string | false    |              |             |
| `name`         | string | true     |              |             |

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
    "stun_addresses": ["string"]
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
  "path": "string",
  "url": "string"
}
```

### Properties

| Name   | Type   | Required | Restrictions | Description |
| ------ | ------ | -------- | ------------ | ----------- |
| `path` | string | false    |              |             |
| `url`  | string | false    |              |             |

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
  "stun_addresses": ["string"]
}
```

### Properties

| Name             | Type                       | Required | Restrictions | Description |
| ---------------- | -------------------------- | -------- | ------------ | ----------- |
| `enable`         | boolean                    | false    |              |             |
| `region_code`    | string                     | false    |              |             |
| `region_id`      | integer                    | false    |              |             |
| `region_name`    | string                     | false    |              |             |
| `relay_url`      | [clibase.URL](#clibaseurl) | false    |              |             |
| `stun_addresses` | array of string            | false    |              |             |

## codersdk.DangerousConfig

```json
{
  "allow_path_app_sharing": true,
  "allow_path_app_site_owner_access": true
}
```

### Properties

| Name                               | Type    | Required | Restrictions | Description |
| ---------------------------------- | ------- | -------- | ------------ | ----------- |
| `allow_path_app_sharing`           | boolean | false    |              |             |
| `allow_path_app_site_owner_access` | boolean | false    |              |             |

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
    "autobuild_poll_interval": 0,
    "browser_only": true,
    "cache_directory": "string",
    "config": "string",
    "config_ssh": {
      "deploymentName": "string",
      "sshconfigOptions": ["string"]
    },
    "dangerous": {
      "allow_path_app_sharing": true,
      "allow_path_app_site_owner_access": true
    },
    "derp": {
      "config": {
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
        "stun_addresses": ["string"]
      }
    },
    "disable_owner_workspace_exec": true,
    "disable_password_auth": true,
    "disable_path_apps": true,
    "disable_session_expiry_refresh": true,
    "experiments": ["string"],
    "git_auth": {
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
    "http_address": "string",
    "in_memory_database": true,
    "logging": {
      "human": "string",
      "json": "string",
      "stackdriver": "string"
    },
    "max_session_expiry": 0,
    "max_token_lifetime": 0,
    "metrics_cache_refresh_interval": 0,
    "oauth2": {
      "github": {
        "allow_everyone": true,
        "allow_signups": true,
        "allowed_orgs": ["string"],
        "allowed_teams": ["string"],
        "client_id": "string",
        "client_secret": "string",
        "enterprise_base_url": "string"
      }
    },
    "oidc": {
      "allow_signups": true,
      "auth_url_params": {},
      "client_id": "string",
      "client_secret": "string",
      "email_domain": ["string"],
      "email_field": "string",
      "group_mapping": {},
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
      "scopes": ["string"],
      "sign_in_text": "string",
      "username_field": "string"
    },
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
      "collect_agent_stats": true,
      "enable": true
    },
    "provisioner": {
      "daemon_poll_interval": 0,
      "daemon_poll_jitter": 0,
      "daemons": 0,
      "force_cancel_interval": 0
    },
    "proxy_trusted_headers": ["string"],
    "proxy_trusted_origins": ["string"],
    "rate_limit": {
      "api": 0,
      "disable_all": true
    },
    "redirect_to_access_url": true,
    "scim_api_key": "string",
    "secure_auth_cookie": true,
    "ssh_keygen_algorithm": "string",
    "strict_transport_security": 0,
    "strict_transport_security_options": ["string"],
    "support": {
      "links": {
        "value": [
          {
            "icon": "string",
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
    "tls": {
      "address": {
        "host": "string",
        "port": "string"
      },
      "cert_file": ["string"],
      "client_auth": "string",
      "client_ca_file": "string",
      "client_cert_file": "string",
      "client_key_file": "string",
      "enable": true,
      "key_file": ["string"],
      "min_version": "string",
      "redirect_http": true
    },
    "trace": {
      "capture_logs": true,
      "enable": true,
      "honeycomb_api_key": "string"
    },
    "update_check": true,
    "verbose": true,
    "wgtunnel_host": "string",
    "wildcard_access_url": {
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
      "use_instead": [{}],
      "value": null,
      "value_source": "",
      "yaml": "string"
    }
  ]
}
```

### Properties

| Name      | Type                                                   | Required | Restrictions | Description |
| --------- | ------------------------------------------------------ | -------- | ------------ | ----------- |
| `config`  | [codersdk.DeploymentValues](#codersdkdeploymentvalues) | false    |              |             |
| `options` | array of [clibase.Option](#clibaseoption)              | false    |              |             |

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
| ----------------- | ---------------------------------------------------------------------------- | -------- | ------------ | --------------------------------------------------------------------------------------------------------------------------- |
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
  "autobuild_poll_interval": 0,
  "browser_only": true,
  "cache_directory": "string",
  "config": "string",
  "config_ssh": {
    "deploymentName": "string",
    "sshconfigOptions": ["string"]
  },
  "dangerous": {
    "allow_path_app_sharing": true,
    "allow_path_app_site_owner_access": true
  },
  "derp": {
    "config": {
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
      "stun_addresses": ["string"]
    }
  },
  "disable_owner_workspace_exec": true,
  "disable_password_auth": true,
  "disable_path_apps": true,
  "disable_session_expiry_refresh": true,
  "experiments": ["string"],
  "git_auth": {
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
  "http_address": "string",
  "in_memory_database": true,
  "logging": {
    "human": "string",
    "json": "string",
    "stackdriver": "string"
  },
  "max_session_expiry": 0,
  "max_token_lifetime": 0,
  "metrics_cache_refresh_interval": 0,
  "oauth2": {
    "github": {
      "allow_everyone": true,
      "allow_signups": true,
      "allowed_orgs": ["string"],
      "allowed_teams": ["string"],
      "client_id": "string",
      "client_secret": "string",
      "enterprise_base_url": "string"
    }
  },
  "oidc": {
    "allow_signups": true,
    "auth_url_params": {},
    "client_id": "string",
    "client_secret": "string",
    "email_domain": ["string"],
    "email_field": "string",
    "group_mapping": {},
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
    "scopes": ["string"],
    "sign_in_text": "string",
    "username_field": "string"
  },
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
    "collect_agent_stats": true,
    "enable": true
  },
  "provisioner": {
    "daemon_poll_interval": 0,
    "daemon_poll_jitter": 0,
    "daemons": 0,
    "force_cancel_interval": 0
  },
  "proxy_trusted_headers": ["string"],
  "proxy_trusted_origins": ["string"],
  "rate_limit": {
    "api": 0,
    "disable_all": true
  },
  "redirect_to_access_url": true,
  "scim_api_key": "string",
  "secure_auth_cookie": true,
  "ssh_keygen_algorithm": "string",
  "strict_transport_security": 0,
  "strict_transport_security_options": ["string"],
  "support": {
    "links": {
      "value": [
        {
          "icon": "string",
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
  "tls": {
    "address": {
      "host": "string",
      "port": "string"
    },
    "cert_file": ["string"],
    "client_auth": "string",
    "client_ca_file": "string",
    "client_cert_file": "string",
    "client_key_file": "string",
    "enable": true,
    "key_file": ["string"],
    "min_version": "string",
    "redirect_http": true
  },
  "trace": {
    "capture_logs": true,
    "enable": true,
    "honeycomb_api_key": "string"
  },
  "update_check": true,
  "verbose": true,
  "wgtunnel_host": "string",
  "wildcard_access_url": {
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
  "write_config": true
}
```

### Properties

| Name                                 | Type                                                                                       | Required | Restrictions | Description                                                        |
| ------------------------------------ | ------------------------------------------------------------------------------------------ | -------- | ------------ | ------------------------------------------------------------------ |
| `access_url`                         | [clibase.URL](#clibaseurl)                                                                 | false    |              |                                                                    |
| `address`                            | [clibase.HostPort](#clibasehostport)                                                       | false    |              | Address Use HTTPAddress or TLS.Address instead.                    |
| `agent_fallback_troubleshooting_url` | [clibase.URL](#clibaseurl)                                                                 | false    |              |                                                                    |
| `agent_stat_refresh_interval`        | integer                                                                                    | false    |              |                                                                    |
| `autobuild_poll_interval`            | integer                                                                                    | false    |              |                                                                    |
| `browser_only`                       | boolean                                                                                    | false    |              |                                                                    |
| `cache_directory`                    | string                                                                                     | false    |              |                                                                    |
| `config`                             | string                                                                                     | false    |              |                                                                    |
| `config_ssh`                         | [codersdk.SSHConfig](#codersdksshconfig)                                                   | false    |              |                                                                    |
| `dangerous`                          | [codersdk.DangerousConfig](#codersdkdangerousconfig)                                       | false    |              |                                                                    |
| `derp`                               | [codersdk.DERP](#codersdkderp)                                                             | false    |              |                                                                    |
| `disable_owner_workspace_exec`       | boolean                                                                                    | false    |              |                                                                    |
| `disable_password_auth`              | boolean                                                                                    | false    |              |                                                                    |
| `disable_path_apps`                  | boolean                                                                                    | false    |              |                                                                    |
| `disable_session_expiry_refresh`     | boolean                                                                                    | false    |              |                                                                    |
| `experiments`                        | array of string                                                                            | false    |              |                                                                    |
| `git_auth`                           | [clibase.Struct-array_codersdk_GitAuthConfig](#clibasestruct-array_codersdk_gitauthconfig) | false    |              |                                                                    |
| `http_address`                       | string                                                                                     | false    |              | Http address is a string because it may be set to zero to disable. |
| `in_memory_database`                 | boolean                                                                                    | false    |              |                                                                    |
| `logging`                            | [codersdk.LoggingConfig](#codersdkloggingconfig)                                           | false    |              |                                                                    |
| `max_session_expiry`                 | integer                                                                                    | false    |              |                                                                    |
| `max_token_lifetime`                 | integer                                                                                    | false    |              |                                                                    |
| `metrics_cache_refresh_interval`     | integer                                                                                    | false    |              |                                                                    |
| `oauth2`                             | [codersdk.OAuth2Config](#codersdkoauth2config)                                             | false    |              |                                                                    |
| `oidc`                               | [codersdk.OIDCConfig](#codersdkoidcconfig)                                                 | false    |              |                                                                    |
| `pg_connection_url`                  | string                                                                                     | false    |              |                                                                    |
| `pprof`                              | [codersdk.PprofConfig](#codersdkpprofconfig)                                               | false    |              |                                                                    |
| `prometheus`                         | [codersdk.PrometheusConfig](#codersdkprometheusconfig)                                     | false    |              |                                                                    |
| `provisioner`                        | [codersdk.ProvisionerConfig](#codersdkprovisionerconfig)                                   | false    |              |                                                                    |
| `proxy_trusted_headers`              | array of string                                                                            | false    |              |                                                                    |
| `proxy_trusted_origins`              | array of string                                                                            | false    |              |                                                                    |
| `rate_limit`                         | [codersdk.RateLimitConfig](#codersdkratelimitconfig)                                       | false    |              |                                                                    |
| `redirect_to_access_url`             | boolean                                                                                    | false    |              |                                                                    |
| `scim_api_key`                       | string                                                                                     | false    |              |                                                                    |
| `secure_auth_cookie`                 | boolean                                                                                    | false    |              |                                                                    |
| `ssh_keygen_algorithm`               | string                                                                                     | false    |              |                                                                    |
| `strict_transport_security`          | integer                                                                                    | false    |              |                                                                    |
| `strict_transport_security_options`  | array of string                                                                            | false    |              |                                                                    |
| `support`                            | [codersdk.SupportConfig](#codersdksupportconfig)                                           | false    |              |                                                                    |
| `swagger`                            | [codersdk.SwaggerConfig](#codersdkswaggerconfig)                                           | false    |              |                                                                    |
| `telemetry`                          | [codersdk.TelemetryConfig](#codersdktelemetryconfig)                                       | false    |              |                                                                    |
| `tls`                                | [codersdk.TLSConfig](#codersdktlsconfig)                                                   | false    |              |                                                                    |
| `trace`                              | [codersdk.TraceConfig](#codersdktraceconfig)                                               | false    |              |                                                                    |
| `update_check`                       | boolean                                                                                    | false    |              |                                                                    |
| `verbose`                            | boolean                                                                                    | false    |              |                                                                    |
| `wgtunnel_host`                      | string                                                                                     | false    |              |                                                                    |
| `wildcard_access_url`                | [clibase.URL](#clibaseurl)                                                                 | false    |              |                                                                    |
| `write_config`                       | boolean                                                                                    | false    |              |                                                                    |

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

| Name                | Type                                 | Required | Restrictions | Description |
| ------------------- | ------------------------------------ | -------- | ------------ | ----------- |
| `errors`            | array of string                      | false    |              |             |
| `features`          | object                               | false    |              |             |
| » `[any property]`  | [codersdk.Feature](#codersdkfeature) | false    |              |             |
| `has_license`       | boolean                              | false    |              |             |
| `require_telemetry` | boolean                              | false    |              |             |
| `trial`             | boolean                              | false    |              |             |
| `warnings`          | array of string                      | false    |              |             |

## codersdk.Experiment

```json
"moons"
```

### Properties

#### Enumerated Values

| Value               |
| ------------------- |
| `moons`             |
| `workspace_actions` |

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

## codersdk.GitProvider

```json
"azure-devops"
```

### Properties

#### Enumerated Values

| Value          |
| -------------- |
| `azure-devops` |
| `github`       |
| `gitlab`       |
| `bitbucket`    |

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

## codersdk.IssueReconnectingPTYSignedTokenRequest

```json
{
  "agentID": "bc282582-04f9-45ce-b904-3e3bfab66958",
  "url": "string"
}
```

### Properties

| Name      | Type   | Required | Restrictions | Description                                                            |
| --------- | ------ | -------- | ------------ | ---------------------------------------------------------------------- |
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
| -------------- | ------ | -------- | ------------ | ----------- |
| `signed_token` | string | false    |              |             |

## codersdk.JobErrorCode

```json
"MISSING_TEMPLATE_PARAMETER"
```

### Properties

#### Enumerated Values

| Value                         |
| ----------------------------- |
| `MISSING_TEMPLATE_PARAMETER`  |
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

| Name          | Type    | Required | Restrictions | Description                                                                                                                                                                                            |
| ------------- | ------- | -------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `claims`      | object  | false    |              | Claims are the JWT claims asserted by the license. Here we use a generic string map to ensure that all data from the server is parsed verbatim, not just the fields this version of Coder understands. |
| `id`          | integer | false    |              |                                                                                                                                                                                                        |
| `uploaded_at` | string  | false    |              |                                                                                                                                                                                                        |
| `uuid`        | string  | false    |              |                                                                                                                                                                                                        |

## codersdk.LinkConfig

```json
{
  "icon": "string",
  "name": "string",
  "target": "string"
}
```

### Properties

| Name     | Type   | Required | Restrictions | Description |
| -------- | ------ | -------- | ------------ | ----------- |
| `icon`   | string | false    |              |             |
| `name`   | string | false    |              |             |
| `target` | string | false    |              |             |

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
  "human": "string",
  "json": "string",
  "stackdriver": "string"
}
```

### Properties

| Name          | Type   | Required | Restrictions | Description |
| ------------- | ------ | -------- | ------------ | ----------- |
| `human`       | string | false    |              |             |
| `json`        | string | false    |              |             |
| `stackdriver` | string | false    |              |             |

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
    "allow_everyone": true,
    "allow_signups": true,
    "allowed_orgs": ["string"],
    "allowed_teams": ["string"],
    "client_id": "string",
    "client_secret": "string",
    "enterprise_base_url": "string"
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
  "allow_everyone": true,
  "allow_signups": true,
  "allowed_orgs": ["string"],
  "allowed_teams": ["string"],
  "client_id": "string",
  "client_secret": "string",
  "enterprise_base_url": "string"
}
```

### Properties

| Name                  | Type            | Required | Restrictions | Description |
| --------------------- | --------------- | -------- | ------------ | ----------- |
| `allow_everyone`      | boolean         | false    |              |             |
| `allow_signups`       | boolean         | false    |              |             |
| `allowed_orgs`        | array of string | false    |              |             |
| `allowed_teams`       | array of string | false    |              |             |
| `client_id`           | string          | false    |              |             |
| `client_secret`       | string          | false    |              |             |
| `enterprise_base_url` | string          | false    |              |             |

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
  "allow_signups": true,
  "auth_url_params": {},
  "client_id": "string",
  "client_secret": "string",
  "email_domain": ["string"],
  "email_field": "string",
  "group_mapping": {},
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
  "scopes": ["string"],
  "sign_in_text": "string",
  "username_field": "string"
}
```

### Properties

| Name                    | Type                       | Required | Restrictions | Description |
| ----------------------- | -------------------------- | -------- | ------------ | ----------- |
| `allow_signups`         | boolean                    | false    |              |             |
| `auth_url_params`       | object                     | false    |              |             |
| `client_id`             | string                     | false    |              |             |
| `client_secret`         | string                     | false    |              |             |
| `email_domain`          | array of string            | false    |              |             |
| `email_field`           | string                     | false    |              |             |
| `group_mapping`         | object                     | false    |              |             |
| `groups_field`          | string                     | false    |              |             |
| `icon_url`              | [clibase.URL](#clibaseurl) | false    |              |             |
| `ignore_email_verified` | boolean                    | false    |              |             |
| `ignore_user_info`      | boolean                    | false    |              |             |
| `issuer_url`            | string                     | false    |              |             |
| `scopes`                | array of string            | false    |              |             |
| `sign_in_text`          | string                     | false    |              |             |
| `username_field`        | string                     | false    |              |             |

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

## codersdk.PatchTemplateVersionRequest

```json
{
  "name": "string"
}
```

### Properties

| Name   | Type   | Required | Restrictions | Description |
| ------ | ------ | -------- | ------------ | ----------- |
| `name` | string | false    |              |             |

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
| ------------------ | ------- | -------- | ------------ | ----------- |
| `display_name`     | string  | true     |              |             |
| `icon`             | string  | true     |              |             |
| `id`               | string  | true     |              |             |
| `name`             | string  | true     |              |             |
| `regenerate_token` | boolean | false    |              |             |

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
| --------- | ------------------------------------ | -------- | ------------ | ----------- |
| `address` | [clibase.HostPort](#clibasehostport) | false    |              |             |
| `enable`  | boolean                              | false    |              |             |

## codersdk.PrometheusConfig

```json
{
  "address": {
    "host": "string",
    "port": "string"
  },
  "collect_agent_stats": true,
  "enable": true
}
```

### Properties

| Name                  | Type                                 | Required | Restrictions | Description |
| --------------------- | ------------------------------------ | -------- | ------------ | ----------- |
| `address`             | [clibase.HostPort](#clibasehostport) | false    |              |             |
| `collect_agent_stats` | boolean                              | false    |              |             |
| `enable`              | boolean                              | false    |              |             |

## codersdk.ProvisionerConfig

```json
{
  "daemon_poll_interval": 0,
  "daemon_poll_jitter": 0,
  "daemons": 0,
  "force_cancel_interval": 0
}
```

### Properties

| Name                    | Type    | Required | Restrictions | Description |
| ----------------------- | ------- | -------- | ------------ | ----------- |
| `daemon_poll_interval`  | integer | false    |              |             |
| `daemon_poll_jitter`    | integer | false    |              |             |
| `daemons`               | integer | false    |              |             |
| `force_cancel_interval` | integer | false    |              |             |

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
  "error_code": "MISSING_TEMPLATE_PARAMETER",
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
| `error_code`       | [codersdk.JobErrorCode](#codersdkjoberrorcode)                 | false    |              |             |
| `file_id`          | string                                                         | false    |              |             |
| `id`               | string                                                         | false    |              |             |
| `started_at`       | string                                                         | false    |              |             |
| `status`           | [codersdk.ProvisionerJobStatus](#codersdkprovisionerjobstatus) | false    |              |             |
| `tags`             | object                                                         | false    |              |             |
| » `[any property]` | string                                                         | false    |              |             |
| `worker_id`        | string                                                         | false    |              |             |

#### Enumerated Values

| Property     | Value                         |
| ------------ | ----------------------------- |
| `error_code` | `MISSING_TEMPLATE_PARAMETER`  |
| `error_code` | `REQUIRED_TEMPLATE_VARIABLES` |
| `status`     | `pending`                     |
| `status`     | `running`                     |
| `status`     | `succeeded`                   |
| `status`     | `canceling`                   |
| `status`     | `canceled`                    |
| `status`     | `failed`                      |

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

## codersdk.ProvisionerLogLevel

```json
"debug"
```

### Properties

#### Enumerated Values

| Value   |
| ------- |
| `debug` |

## codersdk.ProvisionerStorageMethod

```json
"file"
```

### Properties

#### Enumerated Values

| Value  |
| ------ |
| `file` |

## codersdk.ProxyHealthReport

```json
{
  "errors": ["string"],
  "warnings": ["string"]
}
```

### Properties

| Name       | Type            | Required | Restrictions | Description                                                                              |
| ---------- | --------------- | -------- | ------------ | ---------------------------------------------------------------------------------------- |
| `errors`   | array of string | false    |              | Errors are problems that prevent the workspace proxy from being healthy                  |
| `warnings` | array of string | false    |              | Warnings do not prevent the workspace proxy from being healthy, but should be addressed. |

## codersdk.ProxyHealthStatus

```json
"reachable"
```

### Properties

#### Enumerated Values

| Value          |
| -------------- |
| `reachable`    |
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
| ---------- | ------ | -------- | ------------ | ----------- |
| `deadline` | string | true     |              |             |

## codersdk.RBACResource

```json
"workspace"
```

### Properties

#### Enumerated Values

| Value                 |
| --------------------- |
| `workspace`           |
| `workspace_proxy`     |
| `workspace_execution` |
| `application_connect` |
| `audit_log`           |
| `template`            |
| `group`               |
| `file`                |
| `provisioner_daemon`  |
| `organization`        |
| `assign_role`         |
| `assign_org_role`     |
| `api_key`             |
| `user`                |
| `user_data`           |
| `organization_member` |
| `license`             |
| `deployment_config`   |
| `deployment_stats`    |
| `replicas`            |
| `debug_info`          |
| `system`              |

## codersdk.RateLimitConfig

```json
{
  "api": 0,
  "disable_all": true
}
```

### Properties

| Name          | Type    | Required | Restrictions | Description |
| ------------- | ------- | -------- | ------------ | ----------- |
| `api`         | integer | false    |              |             |
| `disable_all` | boolean | false    |              |             |

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

| Name                | Type    | Required | Restrictions | Description                                                                                                                                                                        |
| ------------------- | ------- | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `display_name`      | string  | false    |              |                                                                                                                                                                                    |
| `healthy`           | boolean | false    |              |                                                                                                                                                                                    |
| `icon_url`          | string  | false    |              |                                                                                                                                                                                    |
| `id`                | string  | false    |              |                                                                                                                                                                                    |
| `name`              | string  | false    |              |                                                                                                                                                                                    |
| `path_app_url`      | string  | false    |              | Path app URL is the URL to the base path for path apps. Optional unless wildcard_hostname is set. E.g. https://us.example.com                                                      |
| `wildcard_hostname` | string  | false    |              | Wildcard hostname is the wildcard hostname for subdomain apps. E.g. _.us.example.com E.g. _--suffix.au.example.com Optional. Does not need to be on the same domain as PathAppURL. |

## codersdk.RegionsResponse

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
| --------- | ------------------------------------------- | -------- | ------------ | ----------- |
| `regions` | array of [codersdk.Region](#codersdkregion) | false    |              |             |

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

## codersdk.SSHConfig

```json
{
  "deploymentName": "string",
  "sshconfigOptions": ["string"]
}
```

### Properties

| Name               | Type            | Required | Restrictions | Description                                                                                         |
| ------------------ | --------------- | -------- | ------------ | --------------------------------------------------------------------------------------------------- |
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
| -------------------- | ------ | -------- | ------------ | ----------- |
| `hostname_prefix`    | string | false    |              |             |
| `ssh_config_options` | object | false    |              |             |
| » `[any property]`   | string | false    |              |             |

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
| ------------------ | ------- | -------- | ------------ | ----------- |
| `jetbrains`        | integer | false    |              |             |
| `reconnecting_pty` | integer | false    |              |             |
| `ssh`              | integer | false    |              |             |
| `vscode`           | integer | false    |              |             |

## codersdk.SupportConfig

```json
{
  "links": {
    "value": [
      {
        "icon": "string",
        "name": "string",
        "target": "string"
      }
    ]
  }
}
```

### Properties

| Name    | Type                                                                                 | Required | Restrictions | Description |
| ------- | ------------------------------------------------------------------------------------ | -------- | ------------ | ----------- |
| `links` | [clibase.Struct-array_codersdk_LinkConfig](#clibasestruct-array_codersdk_linkconfig) | false    |              |             |

## codersdk.SwaggerConfig

```json
{
  "enable": true
}
```

### Properties

| Name     | Type    | Required | Restrictions | Description |
| -------- | ------- | -------- | ------------ | ----------- |
| `enable` | boolean | false    |              |             |

## codersdk.TLSConfig

```json
{
  "address": {
    "host": "string",
    "port": "string"
  },
  "cert_file": ["string"],
  "client_auth": "string",
  "client_ca_file": "string",
  "client_cert_file": "string",
  "client_key_file": "string",
  "enable": true,
  "key_file": ["string"],
  "min_version": "string",
  "redirect_http": true
}
```

### Properties

| Name               | Type                                 | Required | Restrictions | Description |
| ------------------ | ------------------------------------ | -------- | ------------ | ----------- |
| `address`          | [clibase.HostPort](#clibasehostport) | false    |              |             |
| `cert_file`        | array of string                      | false    |              |             |
| `client_auth`      | string                               | false    |              |             |
| `client_ca_file`   | string                               | false    |              |             |
| `client_cert_file` | string                               | false    |              |             |
| `client_key_file`  | string                               | false    |              |             |
| `enable`           | boolean                              | false    |              |             |
| `key_file`         | array of string                      | false    |              |             |
| `min_version`      | string                               | false    |              |             |
| `redirect_http`    | boolean                              | false    |              |             |

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
| -------- | -------------------------- | -------- | ------------ | ----------- |
| `enable` | boolean                    | false    |              |             |
| `trace`  | boolean                    | false    |              |             |
| `url`    | [clibase.URL](#clibaseurl) | false    |              |             |

## codersdk.Template

```json
{
  "active_user_count": 0,
  "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
  "allow_user_autostart": true,
  "allow_user_autostop": true,
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
  "failure_ttl_ms": 0,
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "inactivity_ttl_ms": 0,
  "max_ttl_ms": 0,
  "name": "string",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "provisioner": "terraform",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Properties

| Name                               | Type                                                               | Required | Restrictions | Description                                                                                                                                                             |
| ---------------------------------- | ------------------------------------------------------------------ | -------- | ------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `active_user_count`                | integer                                                            | false    |              | Active user count is set to -1 when loading.                                                                                                                            |
| `active_version_id`                | string                                                             | false    |              |                                                                                                                                                                         |
| `allow_user_autostart`             | boolean                                                            | false    |              | Allow user autostart and AllowUserAutostop are enterprise-only. Their values are only used if your license is entitled to use the advanced template scheduling feature. |
| `allow_user_autostop`              | boolean                                                            | false    |              |                                                                                                                                                                         |
| `allow_user_cancel_workspace_jobs` | boolean                                                            | false    |              |                                                                                                                                                                         |
| `build_time_stats`                 | [codersdk.TemplateBuildTimeStats](#codersdktemplatebuildtimestats) | false    |              |                                                                                                                                                                         |
| `created_at`                       | string                                                             | false    |              |                                                                                                                                                                         |
| `created_by_id`                    | string                                                             | false    |              |                                                                                                                                                                         |
| `created_by_name`                  | string                                                             | false    |              |                                                                                                                                                                         |
| `default_ttl_ms`                   | integer                                                            | false    |              |                                                                                                                                                                         |
| `description`                      | string                                                             | false    |              |                                                                                                                                                                         |
| `display_name`                     | string                                                             | false    |              |                                                                                                                                                                         |
| `failure_ttl_ms`                   | integer                                                            | false    |              | Failure ttl ms and InactivityTTLMillis are enterprise-only. Their values are used if your license is entitled to use the advanced template scheduling feature.          |
| `icon`                             | string                                                             | false    |              |                                                                                                                                                                         |
| `id`                               | string                                                             | false    |              |                                                                                                                                                                         |
| `inactivity_ttl_ms`                | integer                                                            | false    |              |                                                                                                                                                                         |
| `max_ttl_ms`                       | integer                                                            | false    |              | Max ttl ms is an enterprise feature. It's value is only used if your license is entitled to use the advanced template scheduling feature.                               |
| `name`                             | string                                                             | false    |              |                                                                                                                                                                         |
| `organization_id`                  | string                                                             | false    |              |                                                                                                                                                                         |
| `provisioner`                      | string                                                             | false    |              |                                                                                                                                                                         |
| `updated_at`                       | string                                                             | false    |              |                                                                                                                                                                         |

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
    "error_code": "MISSING_TEMPLATE_PARAMETER",
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

## codersdk.TemplateVersionGitAuth

```json
{
  "authenticate_url": "string",
  "authenticated": true,
  "id": "string",
  "type": "azure-devops"
}
```

### Properties

| Name               | Type                                         | Required | Restrictions | Description |
| ------------------ | -------------------------------------------- | -------- | ------------ | ----------- |
| `authenticate_url` | string                                       | false    |              |             |
| `authenticated`    | boolean                                      | false    |              |             |
| `id`               | string                                       | false    |              |             |
| `type`             | [codersdk.GitProvider](#codersdkgitprovider) | false    |              |             |

## codersdk.TemplateVersionParameter

```json
{
  "default_value": "string",
  "description": "string",
  "description_plaintext": "string",
  "display_name": "string",
  "icon": "string",
  "legacy_variable_name": "string",
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
| ----------------------- | ------------------------------------------------------------------------------------------- | -------- | ------------ | ----------- |
| `default_value`         | string                                                                                      | false    |              |             |
| `description`           | string                                                                                      | false    |              |             |
| `description_plaintext` | string                                                                                      | false    |              |             |
| `display_name`          | string                                                                                      | false    |              |             |
| `icon`                  | string                                                                                      | false    |              |             |
| `legacy_variable_name`  | string                                                                                      | false    |              |             |
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
| ---------------------- | -------------- |
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

## codersdk.TokenConfig

```json
{
  "max_token_lifetime": 0
}
```

### Properties

| Name                 | Type    | Required | Restrictions | Description |
| -------------------- | ------- | -------- | ------------ | ----------- |
| `max_token_lifetime` | integer | false    |              |             |

## codersdk.TraceConfig

```json
{
  "capture_logs": true,
  "enable": true,
  "honeycomb_api_key": "string"
}
```

### Properties

| Name                | Type    | Required | Restrictions | Description |
| ------------------- | ------- | -------- | ------------ | ----------- |
| `capture_logs`      | boolean | false    |              |             |
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

## codersdk.UpdateAppearanceConfig

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
      "error_code": "MISSING_TEMPLATE_PARAMETER",
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
    "max_deadline": "2019-08-24T14:15:22Z",
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
            "shutdown_script": "string",
            "shutdown_script_timeout_seconds": 0,
            "startup_logs_length": 0,
            "startup_logs_overflowed": true,
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
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
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
| `organization_id`                           | string                                             | false    |              |             |
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
  "shutdown_script": "string",
  "shutdown_script_timeout_seconds": 0,
  "startup_logs_length": 0,
  "startup_logs_overflowed": true,
  "startup_script": "string",
  "startup_script_timeout_seconds": 0,
  "status": "connecting",
  "troubleshooting_url": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "version": "string"
}
```

### Properties

| Name                              | Type                                                                 | Required | Restrictions | Description                                                                                                                                                                                                |
| --------------------------------- | -------------------------------------------------------------------- | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `apps`                            | array of [codersdk.WorkspaceApp](#codersdkworkspaceapp)              | false    |              |                                                                                                                                                                                                            |
| `architecture`                    | string                                                               | false    |              |                                                                                                                                                                                                            |
| `connection_timeout_seconds`      | integer                                                              | false    |              |                                                                                                                                                                                                            |
| `created_at`                      | string                                                               | false    |              |                                                                                                                                                                                                            |
| `directory`                       | string                                                               | false    |              |                                                                                                                                                                                                            |
| `disconnected_at`                 | string                                                               | false    |              |                                                                                                                                                                                                            |
| `environment_variables`           | object                                                               | false    |              |                                                                                                                                                                                                            |
| » `[any property]`                | string                                                               | false    |              |                                                                                                                                                                                                            |
| `expanded_directory`              | string                                                               | false    |              |                                                                                                                                                                                                            |
| `first_connected_at`              | string                                                               | false    |              |                                                                                                                                                                                                            |
| `id`                              | string                                                               | false    |              |                                                                                                                                                                                                            |
| `instance_id`                     | string                                                               | false    |              |                                                                                                                                                                                                            |
| `last_connected_at`               | string                                                               | false    |              |                                                                                                                                                                                                            |
| `latency`                         | object                                                               | false    |              | Latency is mapped by region name (e.g. "New York City", "Seattle").                                                                                                                                        |
| » `[any property]`                | [codersdk.DERPRegion](#codersdkderpregion)                           | false    |              |                                                                                                                                                                                                            |
| `lifecycle_state`                 | [codersdk.WorkspaceAgentLifecycle](#codersdkworkspaceagentlifecycle) | false    |              |                                                                                                                                                                                                            |
| `login_before_ready`              | boolean                                                              | false    |              | Login before ready if true, the agent will delay logins until it is ready (e.g. executing startup script has ended).                                                                                       |
| `name`                            | string                                                               | false    |              |                                                                                                                                                                                                            |
| `operating_system`                | string                                                               | false    |              |                                                                                                                                                                                                            |
| `resource_id`                     | string                                                               | false    |              |                                                                                                                                                                                                            |
| `shutdown_script`                 | string                                                               | false    |              |                                                                                                                                                                                                            |
| `shutdown_script_timeout_seconds` | integer                                                              | false    |              |                                                                                                                                                                                                            |
| `startup_logs_length`             | integer                                                              | false    |              |                                                                                                                                                                                                            |
| `startup_logs_overflowed`         | boolean                                                              | false    |              |                                                                                                                                                                                                            |
| `startup_script`                  | string                                                               | false    |              |                                                                                                                                                                                                            |
| `startup_script_timeout_seconds`  | integer                                                              | false    |              | Startup script timeout seconds is the number of seconds to wait for the startup script to complete. If the script does not complete within this time, the agent lifecycle will be marked as start_timeout. |
| `status`                          | [codersdk.WorkspaceAgentStatus](#codersdkworkspaceagentstatus)       | false    |              |                                                                                                                                                                                                            |
| `troubleshooting_url`             | string                                                               | false    |              |                                                                                                                                                                                                            |
| `updated_at`                      | string                                                               | false    |              |                                                                                                                                                                                                            |
| `version`                         | string                                                               | false    |              |                                                                                                                                                                                                            |

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

| Value              |
| ------------------ |
| `created`          |
| `starting`         |
| `start_timeout`    |
| `start_error`      |
| `ready`            |
| `shutting_down`    |
| `shutdown_timeout` |
| `shutdown_error`   |
| `off`              |

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

## codersdk.WorkspaceAgentMetadataDescription

```json
{
  "display_name": "string",
  "interval": 0,
  "key": "string",
  "script": "string",
  "timeout": 0
}
```

### Properties

| Name           | Type    | Required | Restrictions | Description |
| -------------- | ------- | -------- | ------------ | ----------- |
| `display_name` | string  | false    |              |             |
| `interval`     | integer | false    |              |             |
| `key`          | string  | false    |              |             |
| `script`       | string  | false    |              |             |
| `timeout`      | integer | false    |              |             |

## codersdk.WorkspaceAgentStartupLog

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "id": 0,
  "level": "trace",
  "output": "string"
}
```

### Properties

| Name         | Type                                   | Required | Restrictions | Description |
| ------------ | -------------------------------------- | -------- | ------------ | ----------- |
| `created_at` | string                                 | false    |              |             |
| `id`         | integer                                | false    |              |             |
| `level`      | [codersdk.LogLevel](#codersdkloglevel) | false    |              |             |
| `output`     | string                                 | false    |              |             |

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
    "error_code": "MISSING_TEMPLATE_PARAMETER",
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
  "max_deadline": "2019-08-24T14:15:22Z",
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
          "shutdown_script": "string",
          "shutdown_script_timeout_seconds": 0,
          "startup_logs_length": 0,
          "startup_logs_overflowed": true,
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
| `max_deadline`          | string                                                            | false    |              |             |
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

## codersdk.WorkspaceConnectionLatencyMS

```json
{
  "p50": 0,
  "p95": 0
}
```

### Properties

| Name  | Type   | Required | Restrictions | Description |
| ----- | ------ | -------- | ------------ | ----------- |
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
| ----------------------- | ------------------------------------------------------------------------------ | -------- | ------------ | ----------- |
| `building`              | integer                                                                        | false    |              |             |
| `connection_latency_ms` | [codersdk.WorkspaceConnectionLatencyMS](#codersdkworkspaceconnectionlatencyms) | false    |              |             |
| `failed`                | integer                                                                        | false    |              |             |
| `pending`               | integer                                                                        | false    |              |             |
| `running`               | integer                                                                        | false    |              |             |
| `rx_bytes`              | integer                                                                        | false    |              |             |
| `stopped`               | integer                                                                        | false    |              |             |
| `tx_bytes`              | integer                                                                        | false    |              |             |

## codersdk.WorkspaceProxy

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "deleted": true,
  "display_name": "string",
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "status": {
    "checked_at": "2019-08-24T14:15:22Z",
    "report": {
      "errors": ["string"],
      "warnings": ["string"]
    },
    "status": "reachable"
  },
  "updated_at": "2019-08-24T14:15:22Z",
  "url": "string",
  "wildcard_hostname": "string"
}
```

### Properties

| Name                | Type                                                           | Required | Restrictions | Description                                                                                                                                                                   |
| ------------------- | -------------------------------------------------------------- | -------- | ------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `created_at`        | string                                                         | false    |              |                                                                                                                                                                               |
| `deleted`           | boolean                                                        | false    |              |                                                                                                                                                                               |
| `display_name`      | string                                                         | false    |              |                                                                                                                                                                               |
| `icon`              | string                                                         | false    |              |                                                                                                                                                                               |
| `id`                | string                                                         | false    |              |                                                                                                                                                                               |
| `name`              | string                                                         | false    |              |                                                                                                                                                                               |
| `status`            | [codersdk.WorkspaceProxyStatus](#codersdkworkspaceproxystatus) | false    |              | Status is the latest status check of the proxy. This will be empty for deleted proxies. This value can be used to determine if a workspace proxy is healthy and ready to use. |
| `updated_at`        | string                                                         | false    |              |                                                                                                                                                                               |
| `url`               | string                                                         | false    |              | Full URL including scheme of the proxy api url: https://us.example.com                                                                                                        |
| `wildcard_hostname` | string                                                         | false    |              | Wildcard hostname with the wildcard for subdomain based app hosting: \*.us.example.com                                                                                        |

## codersdk.WorkspaceProxyStatus

```json
{
  "checked_at": "2019-08-24T14:15:22Z",
  "report": {
    "errors": ["string"],
    "warnings": ["string"]
  },
  "status": "reachable"
}
```

### Properties

| Name         | Type                                                     | Required | Restrictions | Description                                                               |
| ------------ | -------------------------------------------------------- | -------- | ------------ | ------------------------------------------------------------------------- |
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
      "shutdown_script": "string",
      "shutdown_script_timeout_seconds": 0,
      "startup_logs_length": 0,
      "startup_logs_overflowed": true,
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
          "error_code": "MISSING_TEMPLATE_PARAMETER",
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
        "max_deadline": "2019-08-24T14:15:22Z",
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
                "shutdown_script": "string",
                "shutdown_script_timeout_seconds": 0,
                "startup_logs_length": 0,
                "startup_logs_overflowed": true,
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
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
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

## derp.ServerInfoMessage

```json
{
  "tokenBucketBytesBurst": 0,
  "tokenBucketBytesPerSecond": 0
}
```

### Properties

| Name                                                                                       | Type    | Required | Restrictions | Description                                                                                                              |
| ------------------------------------------------------------------------------------------ | ------- | -------- | ------------ | ------------------------------------------------------------------------------------------------------------------------ |
| `tokenBucketBytesBurst`                                                                    | integer | false    |              | Tokenbucketbytesburst is how many bytes the server will allow to burst, temporarily violating TokenBucketBytesPerSecond. |
| Zero means unspecified. There might be a limit, but the client need not try to respect it. |
| `tokenBucketBytesPerSecond`                                                                | integer | false    |              | Tokenbucketbytespersecond is how many bytes per second the server says it will accept, including all framing bytes.      |
| Zero means unspecified. There might be a limit, but the client need not try to respect it. |

## healthcheck.AccessURLReport

```json
{
  "error": null,
  "healthy": true,
  "healthzResponse": "string",
  "reachable": true,
  "statusCode": 0
}
```

### Properties

| Name              | Type    | Required | Restrictions | Description |
| ----------------- | ------- | -------- | ------------ | ----------- |
| `error`           | any     | false    |              |             |
| `healthy`         | boolean | false    |              |             |
| `healthzResponse` | string  | false    |              |             |
| `reachable`       | boolean | false    |              |             |
| `statusCode`      | integer | false    |              |             |

## healthcheck.DERPNodeReport

```json
{
  "can_exchange_messages": true,
  "client_errs": [[null]],
  "client_logs": [["string"]],
  "error": null,
  "healthy": true,
  "node": {
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
  "round_trip_ping": 0,
  "stun": {
    "canSTUN": true,
    "enabled": true,
    "error": null
  },
  "uses_websocket": true
}
```

### Properties

| Name                    | Type                                                     | Required | Restrictions | Description |
| ----------------------- | -------------------------------------------------------- | -------- | ------------ | ----------- |
| `can_exchange_messages` | boolean                                                  | false    |              |             |
| `client_errs`           | array of array                                           | false    |              |             |
| `client_logs`           | array of array                                           | false    |              |             |
| `error`                 | any                                                      | false    |              |             |
| `healthy`               | boolean                                                  | false    |              |             |
| `node`                  | [tailcfg.DERPNode](#tailcfgderpnode)                     | false    |              |             |
| `node_info`             | [derp.ServerInfoMessage](#derpserverinfomessage)         | false    |              |             |
| `round_trip_ping`       | integer                                                  | false    |              |             |
| `stun`                  | [healthcheck.DERPStunReport](#healthcheckderpstunreport) | false    |              |             |
| `uses_websocket`        | boolean                                                  | false    |              |             |

## healthcheck.DERPRegionReport

```json
{
  "error": null,
  "healthy": true,
  "node_reports": [
    {
      "can_exchange_messages": true,
      "client_errs": [[null]],
      "client_logs": [["string"]],
      "error": null,
      "healthy": true,
      "node": {
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
      "round_trip_ping": 0,
      "stun": {
        "canSTUN": true,
        "enabled": true,
        "error": null
      },
      "uses_websocket": true
    }
  ],
  "region": {
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
```

### Properties

| Name           | Type                                                              | Required | Restrictions | Description |
| -------------- | ----------------------------------------------------------------- | -------- | ------------ | ----------- |
| `error`        | any                                                               | false    |              |             |
| `healthy`      | boolean                                                           | false    |              |             |
| `node_reports` | array of [healthcheck.DERPNodeReport](#healthcheckderpnodereport) | false    |              |             |
| `region`       | [tailcfg.DERPRegion](#tailcfgderpregion)                          | false    |              |             |

## healthcheck.DERPReport

```json
{
  "error": null,
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
  "netcheck_err": null,
  "netcheck_logs": ["string"],
  "regions": {
    "property1": {
      "error": null,
      "healthy": true,
      "node_reports": [
        {
          "can_exchange_messages": true,
          "client_errs": [[null]],
          "client_logs": [["string"]],
          "error": null,
          "healthy": true,
          "node": {
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
          "round_trip_ping": 0,
          "stun": {
            "canSTUN": true,
            "enabled": true,
            "error": null
          },
          "uses_websocket": true
        }
      ],
      "region": {
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
    },
    "property2": {
      "error": null,
      "healthy": true,
      "node_reports": [
        {
          "can_exchange_messages": true,
          "client_errs": [[null]],
          "client_logs": [["string"]],
          "error": null,
          "healthy": true,
          "node": {
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
          "round_trip_ping": 0,
          "stun": {
            "canSTUN": true,
            "enabled": true,
            "error": null
          },
          "uses_websocket": true
        }
      ],
      "region": {
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

| Name               | Type                                                         | Required | Restrictions | Description |
| ------------------ | ------------------------------------------------------------ | -------- | ------------ | ----------- |
| `error`            | any                                                          | false    |              |             |
| `healthy`          | boolean                                                      | false    |              |             |
| `netcheck`         | [netcheck.Report](#netcheckreport)                           | false    |              |             |
| `netcheck_err`     | any                                                          | false    |              |             |
| `netcheck_logs`    | array of string                                              | false    |              |             |
| `regions`          | object                                                       | false    |              |             |
| » `[any property]` | [healthcheck.DERPRegionReport](#healthcheckderpregionreport) | false    |              |             |

## healthcheck.DERPStunReport

```json
{
  "canSTUN": true,
  "enabled": true,
  "error": null
}
```

### Properties

| Name      | Type    | Required | Restrictions | Description |
| --------- | ------- | -------- | ------------ | ----------- |
| `canSTUN` | boolean | false    |              |             |
| `enabled` | boolean | false    |              |             |
| `error`   | any     | false    |              |             |

## healthcheck.Report

```json
{
  "access_url": {
    "error": null,
    "healthy": true,
    "healthzResponse": "string",
    "reachable": true,
    "statusCode": 0
  },
  "derp": {
    "error": null,
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
    "netcheck_err": null,
    "netcheck_logs": ["string"],
    "regions": {
      "property1": {
        "error": null,
        "healthy": true,
        "node_reports": [
          {
            "can_exchange_messages": true,
            "client_errs": [[null]],
            "client_logs": [["string"]],
            "error": null,
            "healthy": true,
            "node": {
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
            "round_trip_ping": 0,
            "stun": {
              "canSTUN": true,
              "enabled": true,
              "error": null
            },
            "uses_websocket": true
          }
        ],
        "region": {
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
      },
      "property2": {
        "error": null,
        "healthy": true,
        "node_reports": [
          {
            "can_exchange_messages": true,
            "client_errs": [[null]],
            "client_logs": [["string"]],
            "error": null,
            "healthy": true,
            "node": {
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
            "round_trip_ping": 0,
            "stun": {
              "canSTUN": true,
              "enabled": true,
              "error": null
            },
            "uses_websocket": true
          }
        ],
        "region": {
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
  },
  "pass": true,
  "time": "string"
}
```

### Properties

| Name         | Type                                                       | Required | Restrictions | Description                                      |
| ------------ | ---------------------------------------------------------- | -------- | ------------ | ------------------------------------------------ |
| `access_url` | [healthcheck.AccessURLReport](#healthcheckaccessurlreport) | false    |              |                                                  |
| `derp`       | [healthcheck.DERPReport](#healthcheckderpreport)           | false    |              |                                                  |
| `pass`       | boolean                                                    | false    |              | Healthy is true if the report returns no errors. |
| `time`       | string                                                     | false    |              | Time is the time the report was generated at.    |

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
| ----------------------- | ------- | -------- | ------------ | ---------------------------------------------------------------------------------------------------------------------------------- |
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

## url.Userinfo

```json
{}
```

### Properties

_None_

## workspaceapps.AccessMethod

```json
"path"
```

### Properties

#### Enumerated Values

| Value       |
| ----------- |
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
| ------------------- | ---------------------------------------------- | -------- | ------------ | --------------------------------------------------------------------------------------------------------------- |
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
  "app_slug_or_port": "string",
  "base_path": "string",
  "username_or_id": "string",
  "workspace_name_or_id": "string"
}
```

### Properties

| Name                   | Type                                                     | Required | Restrictions | Description                                                                                                                                                                           |
| ---------------------- | -------------------------------------------------------- | -------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `access_method`        | [workspaceapps.AccessMethod](#workspaceappsaccessmethod) | false    |              |                                                                                                                                                                                       |
| `agent_name_or_id`     | string                                                   | false    |              | Agent name or ID is not required if the workspace has only one agent.                                                                                                                 |
| `app_slug_or_port`     | string                                                   | false    |              |                                                                                                                                                                                       |
| `base_path`            | string                                                   | false    |              | Base path of the app. For path apps, this is the path prefix in the router for this particular app. For subdomain apps, this should be "/". This is used for setting the cookie path. |
| `username_or_id`       | string                                                   | false    |              | For the following fields, if the AccessMethod is AccessMethodTerminal, then only AgentNameOrID may be set and it must be a UUID. The other fields must be left blank.                 |
| `workspace_name_or_id` | string                                                   | false    |              |                                                                                                                                                                                       |

## wsproxysdk.IssueSignedAppTokenResponse

```json
{
  "signed_token_str": "string"
}
```

### Properties

| Name               | Type   | Required | Restrictions | Description                                                 |
| ------------------ | ------ | -------- | ------------ | ----------------------------------------------------------- |
| `signed_token_str` | string | false    |              | Signed token str should be set as a cookie on the response. |

## wsproxysdk.RegisterWorkspaceProxyRequest

```json
{
  "access_url": "string",
  "wildcard_hostname": "string"
}
```

### Properties

| Name                | Type   | Required | Restrictions | Description                                                                   |
| ------------------- | ------ | -------- | ------------ | ----------------------------------------------------------------------------- |
| `access_url`        | string | false    |              | Access URL that hits the workspace proxy api.                                 |
| `wildcard_hostname` | string | false    |              | Wildcard hostname that the workspace proxy api is serving for subdomain apps. |

## wsproxysdk.RegisterWorkspaceProxyResponse

```json
{
  "app_security_key": "string"
}
```

### Properties

| Name               | Type   | Required | Restrictions | Description |
| ------------------ | ------ | -------- | ------------ | ----------- |
| `app_security_key` | string | false    |              |             |
