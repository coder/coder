# Schemas

## agentsdk.Manifest

```json
{
    "agent_id": "151321db-0713-473c-ab42-2cc6ddeab1a4",
    "agent_name": "string",
    "owner_name": "string",
    "workspace_id": "8ef13a0d-a5c9-4fb4-abf2-f8f65c3830fb",
    "workspace_name": "string",
    "git_auth_configs": 1,
    "vscode_port_proxy_uri": "string",
    "apps": [
        {
            "id": "c488c933-688a-444e-a55d-f1e88ecc78f5",
            "url": "string",
            "external": false,
            "slug": "string",
            "display_name": "string",
            "icon": "string",
            "subdomain": false,
            "sharing_level": "owner",
            "healthcheck": {
                "url": "string",
                "interval": 5,
                "threshold": 6
            },
            "health": "initializing"
        }
    ],
    "derpmap": {
        "HomeParams": {},
        "Regions": {
            "1000": {
                "EmbeddedRelay": false,
                "RegionID": 1000,
                "RegionCode": "string",
                "RegionName": "string",
                "Nodes": [
                    {
                        "Name": "string",
                        "RegionID": 1000,
                        "HostName": "string",
                        "STUNPort": 19302,
                        "STUNOnly": true
                    }
                ]
            }
        }
    },
    "derp_force_websockets": false,
    "environment_variables": {
        "OIDC_TOKEN": "string"
    },
    "directory": "string",
    "motd_file": "string",
    "disable_direct_connections": false,
    "metadata": [
        {
            "display_name": "string",
            "key": "string",
            "script": "string",
            "interval": 10,
            "timeout": 1
        }
    ],
    "scripts": [
        {
            "log_source_id": "3e79c8da-08ae-48f4-b73e-11e194cdea06",
            "log_path": "string",
            "script": "string",
            "cron": "string",
            "run_on_start": true,
            "run_on_stop": false,
            "start_blocks_login": true,
            "timeout": 0
        }
    ]
}
```

### Properties

| Name                         | Type                                                                                              | Required | Restrictions | Description |
|------------------------------|---------------------------------------------------------------------------------------------------|----------|--------------|-------------|
| `agent_id`                   | string                                                                                            | true     |              |             |
| `agent_name`                 | string                                                                                            | true     |              |             |
| `owner_name`                 | string                                                                                            | true     |              |             |
| `workspace_id`               | string                                                                                            | true     |              |             |
| `workspace_name`             | string                                                                                            | true     |              |             |
| `git_auth_configs`           | int                                                                                               | true     |              |             |
| `vscode_port_proxy_uri`      | string                                                                                            | true     |              |             |
| `apps`                       | array of [codersdk.WorkspaceApp](../api/schemas.md#codersdkworkspaceapp)                          | true     |              |             |
| `derpmap`                    | [tailcfg.DERPMap](../api/schemas.md#tailcfgderpmap)                                               | true     |              |             |
| `derp_force_websockets`      | boolean                                                                                           | true     |              |             |
| `environment_variables`      | object                                                                                            | true     |              |             |
| `directory`                  | string                                                                                            | true     |              |             |
| `motd_file`                  | string                                                                                            | true     |              |             |
| `disable_direct_connections` | boolean                                                                                           | true     |              |             |
| `metadata`                   | array of [codersdk.WorkspaceAgentMetadataDescription](#codersdkworkspaceagentmetadatadescription) | true     |              |             |
| `scripts`                    | array of [codersdk.WorkspaceAgentScript](../api/schemas.md#codersdkworkspaceagentscript)          | true     |              |             |

## codersdk.WorkspaceAgentMetadataDescription

```json
{
    "display_name": "string",
    "key": "string",
    "script": "string",
    "interval": 10,
    "timeout": 1
}
```

### Properties

| Name           | Type    | Required | Restrictions | Description |
|----------------|---------|----------|--------------|-------------|
| `display_name` | string  | true     |              |             |
| `key`          | string  | true     |              |             |
| `script`       | string  | true     |              |             |
| `interval`     | integer | true     |              |             |
| `timeout`      | integer | true     |              |             |
