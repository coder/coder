# Chats

Programmatic API for Coder Agents (the user-facing "Coder Agents" / "Chats" product). Use these endpoints to create, list, and manage AI coding agent sessions.

## List chats

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats`

Experimental: this endpoint is subject to change.

### Parameters

| Name    | In    | Type   | Required | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
|---------|-------|--------|----------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `q`     | query | string | false    | Search query. Supports `title:<substring>` (case-insensitive, quote multi-word values), `archived:bool`, `has_unread:bool`, `pr_status:<draft\|open\|merged\|closed>` as repeated or comma-separated values, `source:<created_by_me\|shared_with_me>`, `diff_url:<url>` (quote values containing colons), `pr:<number>` (exact PR number match), `repo:<owner/repo>` (case-insensitive substring match against git remote origin or URL), `pr_title:<text>` (case-insensitive PR title substring). Bare terms are not supported; use `title:<value>` for title filtering. |
| `label` | query | string | false    | Filter by label as key:value. Repeat for multiple (AND logic).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |

### Example responses

> 200 Response

```json
[
  {
    "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
    "archived": true,
    "build_id": "bfb1f3fa-bf7b-43a5-9e0b-26cc050e44cb",
    "children": [
      {}
    ],
    "client_type": "ui",
    "context": {
      "dirty": true,
      "dirty_since": "2019-08-24T14:15:22Z",
      "error": "string",
      "resources": [
        {
          "error": "string",
          "kind": "instruction_file",
          "size_bytes": 0,
          "skill_description": "string",
          "skill_name": "string",
          "source": "string",
          "status": "ok",
          "tools": [
            {
              "description": "string",
              "name": "string"
            }
          ]
        }
      ]
    },
    "created_at": "2019-08-24T14:15:22Z",
    "diff_status": {
      "additions": 0,
      "approved": true,
      "author_avatar_url": "string",
      "author_login": "string",
      "base_branch": "string",
      "changed_files": 0,
      "changes_requested": true,
      "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
      "commits": 0,
      "deletions": 0,
      "head_branch": "string",
      "pr_number": 0,
      "pull_request_draft": true,
      "pull_request_state": "string",
      "pull_request_title": "string",
      "refreshed_at": "2019-08-24T14:15:22Z",
      "reviewer_count": 0,
      "stale_at": "2019-08-24T14:15:22Z",
      "url": "string"
    },
    "files": [
      {
        "created_at": "2019-08-24T14:15:22Z",
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "mime_type": "string",
        "name": "string",
        "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
        "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05"
      }
    ],
    "has_unread": true,
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "labels": {
      "property1": "string",
      "property2": "string"
    },
    "last_error": {
      "detail": "string",
      "kind": "generic",
      "message": "string",
      "provider": "string",
      "retryable": true,
      "status_code": 0
    },
    "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
    "last_reasoning_effort": "string",
    "last_turn_summary": "string",
    "mcp_server_ids": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
    "owner_name": "string",
    "owner_username": "string",
    "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
    "pin_order": 0,
    "plan_mode": "plan",
    "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
    "shared": true,
    "status": "waiting",
    "title": "string",
    "updated_at": "2019-08-24T14:15:22Z",
    "warnings": [
      "string"
    ],
    "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                            |
|--------|---------------------------------------------------------|-------------|---------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Chat](schemas.md#codersdkchat) |

<h3 id="list-chats-responseschema">Response Schema</h3>

Status Code **200**

| Name                      | Type                                                                               | Required | Restrictions | Description                                                                                                                                                                                                                                                                |
|---------------------------|------------------------------------------------------------------------------------|----------|--------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `[array item]`            | array                                                                              | false    |              |                                                                                                                                                                                                                                                                            |
| `» agent_id`              | string(uuid)                                                                       | false    |              |                                                                                                                                                                                                                                                                            |
| `» archived`              | boolean                                                                            | false    |              |                                                                                                                                                                                                                                                                            |
| `» build_id`              | string(uuid)                                                                       | false    |              |                                                                                                                                                                                                                                                                            |
| `» children`              | [codersdk.Chat](schemas.md#codersdkchat)                                           | false    |              | Children holds child (subagent) chats nested under this root chat. Always initialized to an empty slice so the JSON field is present as []. Child chats cannot create their own subagents, so nesting depth is capped at 1 and this slice is always empty for child chats. |
| `» client_type`           | [codersdk.ChatClientType](schemas.md#codersdkchatclienttype)                       | false    |              |                                                                                                                                                                                                                                                                            |
| `» context`               | [codersdk.ChatContext](schemas.md#codersdkchatcontext)                             | false    |              | Context reports the chat's pinned workspace-context state and whether it has drifted from the agent's latest pushed snapshot. Nil when the chat has no pinned context yet.                                                                                                 |
| `»» dirty`                | boolean                                                                            | false    |              | Dirty is true when the agent's latest snapshot hash differs from the chat's pinned hash.                                                                                                                                                                                   |
| `»» dirty_since`          | string(date-time)                                                                  | false    |              | Dirty since is when drift was first detected; nil when not dirty.                                                                                                                                                                                                          |
| `»» error`                | string                                                                             | false    |              | Error is the snapshot-level error copied from the pinned snapshot (empty when healthy).                                                                                                                                                                                    |
| `»» resources`            | array                                                                              | false    |              | Resources is the chat's pinned context (instruction files and skills) the prompt is built from, metadata only (no bodies). It is populated only on the single-chat GET response; list and watch payloads leave it nil to stay lightweight.                                 |
| `»»» error`               | string                                                                             | false    |              | Error explains a non-ok Status; empty when healthy. May also carry a non-fatal warning when Status is ok.                                                                                                                                                                  |
| `»»» kind`                | [codersdk.ChatContextResourceKind](schemas.md#codersdkchatcontextresourcekind)     | false    |              |                                                                                                                                                                                                                                                                            |
| `»»» size_bytes`          | integer                                                                            | false    |              | Size bytes is the original payload size in bytes.                                                                                                                                                                                                                          |
| `»»» skill_description`   | string                                                                             | false    |              |                                                                                                                                                                                                                                                                            |
| `»»» skill_name`          | string                                                                             | false    |              | Skill name and SkillDescription are populated only for skill kinds.                                                                                                                                                                                                        |
| `»»» source`              | string                                                                             | false    |              | Source is the resource locator: the canonical file path for an instruction file, the skill directory for a skill, the file path for an MCP config, or the server name for an MCP server.                                                                                   |
| `»»» status`              | [codersdk.ChatContextResourceStatus](schemas.md#codersdkchatcontextresourcestatus) | false    |              | Status is the resource's health. Non-ok resources (invalid, unreadable, oversize, excluded) are still reported so the UI can surface why a resource was dropped from the prompt instead of silently omitting it; their body-specific fields (skill name, tools) are empty. |
| `»»» tools`               | array                                                                              | false    |              | Tools lists the tools exposed by an MCP server. Populated only for the mcp_server kind; nil otherwise.                                                                                                                                                                     |
| `»»»» description`        | string                                                                             | false    |              | Description is the tool's human-readable summary; may be empty.                                                                                                                                                                                                            |
| `»»»» name`               | string                                                                             | false    |              | Name is the tool name with the "<server>__" prefix the agent adds stripped, so it reads as the server exposes it.                                                                                                                                                          |
| `» created_at`            | string(date-time)                                                                  | false    |              |                                                                                                                                                                                                                                                                            |
| `» diff_status`           | [codersdk.ChatDiffStatus](schemas.md#codersdkchatdiffstatus)                       | false    |              |                                                                                                                                                                                                                                                                            |
| `»» additions`            | integer                                                                            | false    |              |                                                                                                                                                                                                                                                                            |
| `»» approved`             | boolean                                                                            | false    |              |                                                                                                                                                                                                                                                                            |
| `»» author_avatar_url`    | string                                                                             | false    |              |                                                                                                                                                                                                                                                                            |
| `»» author_login`         | string                                                                             | false    |              |                                                                                                                                                                                                                                                                            |
| `»» base_branch`          | string                                                                             | false    |              |                                                                                                                                                                                                                                                                            |
| `»» changed_files`        | integer                                                                            | false    |              |                                                                                                                                                                                                                                                                            |
| `»» changes_requested`    | boolean                                                                            | false    |              |                                                                                                                                                                                                                                                                            |
| `»» chat_id`              | string(uuid)                                                                       | false    |              |                                                                                                                                                                                                                                                                            |
| `»» commits`              | integer                                                                            | false    |              |                                                                                                                                                                                                                                                                            |
| `»» deletions`            | integer                                                                            | false    |              |                                                                                                                                                                                                                                                                            |
| `»» head_branch`          | string                                                                             | false    |              |                                                                                                                                                                                                                                                                            |
| `»» pr_number`            | integer                                                                            | false    |              |                                                                                                                                                                                                                                                                            |
| `»» pull_request_draft`   | boolean                                                                            | false    |              |                                                                                                                                                                                                                                                                            |
| `»» pull_request_state`   | string                                                                             | false    |              |                                                                                                                                                                                                                                                                            |
| `»» pull_request_title`   | string                                                                             | false    |              |                                                                                                                                                                                                                                                                            |
| `»» refreshed_at`         | string(date-time)                                                                  | false    |              |                                                                                                                                                                                                                                                                            |
| `»» reviewer_count`       | integer                                                                            | false    |              |                                                                                                                                                                                                                                                                            |
| `»» stale_at`             | string(date-time)                                                                  | false    |              |                                                                                                                                                                                                                                                                            |
| `»» url`                  | string                                                                             | false    |              |                                                                                                                                                                                                                                                                            |
| `» files`                 | array                                                                              | false    |              |                                                                                                                                                                                                                                                                            |
| `»» created_at`           | string(date-time)                                                                  | false    |              |                                                                                                                                                                                                                                                                            |
| `»» id`                   | string(uuid)                                                                       | false    |              |                                                                                                                                                                                                                                                                            |
| `»» mime_type`            | string                                                                             | false    |              |                                                                                                                                                                                                                                                                            |
| `»» name`                 | string                                                                             | false    |              |                                                                                                                                                                                                                                                                            |
| `»» organization_id`      | string(uuid)                                                                       | false    |              |                                                                                                                                                                                                                                                                            |
| `»» owner_id`             | string(uuid)                                                                       | false    |              |                                                                                                                                                                                                                                                                            |
| `» has_unread`            | boolean                                                                            | false    |              | Has unread is true when assistant messages exist beyond the owner's read cursor, which updates on stream connect and disconnect.                                                                                                                                           |
| `» id`                    | string(uuid)                                                                       | false    |              |                                                                                                                                                                                                                                                                            |
| `» labels`                | object                                                                             | false    |              |                                                                                                                                                                                                                                                                            |
| `»» [any property]`       | string                                                                             | false    |              |                                                                                                                                                                                                                                                                            |
| `» last_error`            | [codersdk.ChatError](schemas.md#codersdkchaterror)                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»» detail`               | string                                                                             | false    |              | Detail is optional provider-specific context shown alongside the normalized error message when available.                                                                                                                                                                  |
| `»» kind`                 | [codersdk.ChatErrorKind](schemas.md#codersdkchaterrorkind)                         | false    |              | Kind classifies the error for consistent client rendering.                                                                                                                                                                                                                 |
| `»» message`              | string                                                                             | false    |              | Message is the normalized, user-facing error message.                                                                                                                                                                                                                      |
| `»» provider`             | string                                                                             | false    |              | Provider identifies the upstream model provider when known.                                                                                                                                                                                                                |
| `»» retryable`            | boolean                                                                            | false    |              | Retryable reports whether the underlying error is transient.                                                                                                                                                                                                               |
| `»» status_code`          | integer                                                                            | false    |              | Status code is the best-effort upstream HTTP status code.                                                                                                                                                                                                                  |
| `» last_model_config_id`  | string(uuid)                                                                       | false    |              |                                                                                                                                                                                                                                                                            |
| `» last_reasoning_effort` | string                                                                             | false    |              |                                                                                                                                                                                                                                                                            |
| `» last_turn_summary`     | string                                                                             | false    |              |                                                                                                                                                                                                                                                                            |
| `» mcp_server_ids`        | array                                                                              | false    |              |                                                                                                                                                                                                                                                                            |
| `» organization_id`       | string(uuid)                                                                       | false    |              |                                                                                                                                                                                                                                                                            |
| `» owner_id`              | string(uuid)                                                                       | false    |              |                                                                                                                                                                                                                                                                            |
| `» owner_name`            | string                                                                             | false    |              |                                                                                                                                                                                                                                                                            |
| `» owner_username`        | string                                                                             | false    |              |                                                                                                                                                                                                                                                                            |
| `» parent_chat_id`        | string(uuid)                                                                       | false    |              |                                                                                                                                                                                                                                                                            |
| `» pin_order`             | integer                                                                            | false    |              |                                                                                                                                                                                                                                                                            |
| `» plan_mode`             | [codersdk.ChatPlanMode](schemas.md#codersdkchatplanmode)                           | false    |              |                                                                                                                                                                                                                                                                            |
| `» root_chat_id`          | string(uuid)                                                                       | false    |              |                                                                                                                                                                                                                                                                            |
| `» shared`                | boolean                                                                            | false    |              | Shared is true when this chat's root chat has explicit user or group ACL entries.                                                                                                                                                                                          |
| `» status`                | [codersdk.ChatStatus](schemas.md#codersdkchatstatus)                               | false    |              |                                                                                                                                                                                                                                                                            |
| `» title`                 | string                                                                             | false    |              |                                                                                                                                                                                                                                                                            |
| `» updated_at`            | string(date-time)                                                                  | false    |              |                                                                                                                                                                                                                                                                            |
| `» warnings`              | array                                                                              | false    |              |                                                                                                                                                                                                                                                                            |
| `» workspace_id`          | string(uuid)                                                                       | false    |              |                                                                                                                                                                                                                                                                            |

#### Enumerated Values

| Property      | Value(s)                                                                                                                                                                                                                   |
|---------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `client_type` | `api`, `ui`                                                                                                                                                                                                                |
| `kind`        | `auth`, `config`, `content_filter`, `generic`, `instruction_file`, `mcp_config`, `mcp_server`, `missing_key`, `overloaded`, `provider_disabled`, `rate_limit`, `skill`, `stream_silence_timeout`, `timeout`, `usage_limit` |
| `status`      | `error`, `excluded`, `interrupting`, `invalid`, `ok`, `oversize`, `requires_action`, `running`, `unreadable`, `waiting`                                                                                                    |
| `plan_mode`   | `plan`                                                                                                                                                                                                                     |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create chat

### Code samples

```sh
# Example request using curl
curl -X POST http://coder-server:8080/api/experimental/chats \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/experimental/chats`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "client_type": "ui",
  "content": [
    {
      "content": "string",
      "end_line": 0,
      "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
      "file_name": "string",
      "start_line": 0,
      "text": "string",
      "type": "text"
    }
  ],
  "labels": {
    "property1": "string",
    "property2": "string"
  },
  "mcp_server_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "model_config_id": "f5fb4d91-62ca-4377-9ee6-5d43ba00d205",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "plan_mode": "plan",
  "reasoning_effort": "string",
  "system_prompt": "string",
  "unsafe_dynamic_tools": [
    {
      "description": "string",
      "input_schema": [
        0
      ],
      "name": "string"
    }
  ],
  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
}
```

### Parameters

| Name   | In   | Type                                                               | Required | Description         |
|--------|------|--------------------------------------------------------------------|----------|---------------------|
| `body` | body | [codersdk.CreateChatRequest](schemas.md#codersdkcreatechatrequest) | true     | Create chat request |

### Example responses

> 201 Response

```json
{
  "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
  "archived": true,
  "build_id": "bfb1f3fa-bf7b-43a5-9e0b-26cc050e44cb",
  "children": [
    {
      "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
      "archived": true,
      "build_id": "bfb1f3fa-bf7b-43a5-9e0b-26cc050e44cb",
      "children": [],
      "client_type": "ui",
      "context": {
        "dirty": true,
        "dirty_since": "2019-08-24T14:15:22Z",
        "error": "string",
        "resources": [
          {
            "error": "string",
            "kind": "instruction_file",
            "size_bytes": 0,
            "skill_description": "string",
            "skill_name": "string",
            "source": "string",
            "status": "ok",
            "tools": [
              {
                "description": "string",
                "name": "string"
              }
            ]
          }
        ]
      },
      "created_at": "2019-08-24T14:15:22Z",
      "diff_status": {
        "additions": 0,
        "approved": true,
        "author_avatar_url": "string",
        "author_login": "string",
        "base_branch": "string",
        "changed_files": 0,
        "changes_requested": true,
        "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
        "commits": 0,
        "deletions": 0,
        "head_branch": "string",
        "pr_number": 0,
        "pull_request_draft": true,
        "pull_request_state": "string",
        "pull_request_title": "string",
        "refreshed_at": "2019-08-24T14:15:22Z",
        "reviewer_count": 0,
        "stale_at": "2019-08-24T14:15:22Z",
        "url": "string"
      },
      "files": [
        {
          "created_at": "2019-08-24T14:15:22Z",
          "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
          "mime_type": "string",
          "name": "string",
          "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
          "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05"
        }
      ],
      "has_unread": true,
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "labels": {
        "property1": "string",
        "property2": "string"
      },
      "last_error": {
        "detail": "string",
        "kind": "generic",
        "message": "string",
        "provider": "string",
        "retryable": true,
        "status_code": 0
      },
      "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
      "last_reasoning_effort": "string",
      "last_turn_summary": "string",
      "mcp_server_ids": [
        "497f6eca-6276-4993-bfeb-53cbbbba6f08"
      ],
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
      "owner_name": "string",
      "owner_username": "string",
      "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
      "pin_order": 0,
      "plan_mode": "plan",
      "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
      "shared": true,
      "status": "waiting",
      "title": "string",
      "updated_at": "2019-08-24T14:15:22Z",
      "warnings": [
        "string"
      ],
      "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
    }
  ],
  "client_type": "ui",
  "context": {
    "dirty": true,
    "dirty_since": "2019-08-24T14:15:22Z",
    "error": "string",
    "resources": [
      {
        "error": "string",
        "kind": "instruction_file",
        "size_bytes": 0,
        "skill_description": "string",
        "skill_name": "string",
        "source": "string",
        "status": "ok",
        "tools": [
          {
            "description": "string",
            "name": "string"
          }
        ]
      }
    ]
  },
  "created_at": "2019-08-24T14:15:22Z",
  "diff_status": {
    "additions": 0,
    "approved": true,
    "author_avatar_url": "string",
    "author_login": "string",
    "base_branch": "string",
    "changed_files": 0,
    "changes_requested": true,
    "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
    "commits": 0,
    "deletions": 0,
    "head_branch": "string",
    "pr_number": 0,
    "pull_request_draft": true,
    "pull_request_state": "string",
    "pull_request_title": "string",
    "refreshed_at": "2019-08-24T14:15:22Z",
    "reviewer_count": 0,
    "stale_at": "2019-08-24T14:15:22Z",
    "url": "string"
  },
  "files": [
    {
      "created_at": "2019-08-24T14:15:22Z",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "mime_type": "string",
      "name": "string",
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05"
    }
  ],
  "has_unread": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "labels": {
    "property1": "string",
    "property2": "string"
  },
  "last_error": {
    "detail": "string",
    "kind": "generic",
    "message": "string",
    "provider": "string",
    "retryable": true,
    "status_code": 0
  },
  "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
  "last_reasoning_effort": "string",
  "last_turn_summary": "string",
  "mcp_server_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "owner_name": "string",
  "owner_username": "string",
  "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
  "pin_order": 0,
  "plan_mode": "plan",
  "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
  "shared": true,
  "status": "waiting",
  "title": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "warnings": [
    "string"
  ],
  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                   |
|--------|--------------------------------------------------------------|-------------|------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.Chat](schemas.md#codersdkchat) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Upload chat file

### Code samples

```sh
# Example request using curl
curl -X POST http://coder-server:8080/api/experimental/chats/files?organization=497f6eca-6276-4993-bfeb-53cbbbba6f08 \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/experimental/chats/files`

Experimental: this endpoint is subject to change.

### Parameters

| Name           | In    | Type         | Required | Description     |
|----------------|-------|--------------|----------|-----------------|
| `organization` | query | string(uuid) | true     | Organization ID |

### Example responses

> 201 Response

```json
{
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                                       |
|--------|--------------------------------------------------------------|-------------|------------------------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.UploadChatFileResponse](schemas.md#codersdkuploadchatfileresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get chat file

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/files/{file} \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/files/{file}`

Experimental: this endpoint is subject to change.

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `file` | path | string(uuid) | true     | File ID     |

### Responses

| Status | Meaning                                                 | Description | Schema |
|--------|---------------------------------------------------------|-------------|--------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List chat models

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/models \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/models`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
{
  "providers": [
    {
      "available": true,
      "models": [
        {
          "display_name": "string",
          "id": "string",
          "model": "string",
          "provider": "string"
        }
      ],
      "provider": "string",
      "unavailable_reason": "missing_api_key"
    }
  ],
  "unsupported_providers": [
    {
      "display_name": "string",
      "provider": "string"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                               |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatModelsResponse](schemas.md#codersdkchatmodelsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Watch chat events for a user via WebSockets

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/watch \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/watch`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
{
  "chat": {
    "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
    "archived": true,
    "build_id": "bfb1f3fa-bf7b-43a5-9e0b-26cc050e44cb",
    "children": [
      {}
    ],
    "client_type": "ui",
    "context": {
      "dirty": true,
      "dirty_since": "2019-08-24T14:15:22Z",
      "error": "string",
      "resources": [
        {
          "error": "string",
          "kind": "instruction_file",
          "size_bytes": 0,
          "skill_description": "string",
          "skill_name": "string",
          "source": "string",
          "status": "ok",
          "tools": [
            {
              "description": "string",
              "name": "string"
            }
          ]
        }
      ]
    },
    "created_at": "2019-08-24T14:15:22Z",
    "diff_status": {
      "additions": 0,
      "approved": true,
      "author_avatar_url": "string",
      "author_login": "string",
      "base_branch": "string",
      "changed_files": 0,
      "changes_requested": true,
      "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
      "commits": 0,
      "deletions": 0,
      "head_branch": "string",
      "pr_number": 0,
      "pull_request_draft": true,
      "pull_request_state": "string",
      "pull_request_title": "string",
      "refreshed_at": "2019-08-24T14:15:22Z",
      "reviewer_count": 0,
      "stale_at": "2019-08-24T14:15:22Z",
      "url": "string"
    },
    "files": [
      {
        "created_at": "2019-08-24T14:15:22Z",
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "mime_type": "string",
        "name": "string",
        "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
        "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05"
      }
    ],
    "has_unread": true,
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "labels": {
      "property1": "string",
      "property2": "string"
    },
    "last_error": {
      "detail": "string",
      "kind": "generic",
      "message": "string",
      "provider": "string",
      "retryable": true,
      "status_code": 0
    },
    "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
    "last_reasoning_effort": "string",
    "last_turn_summary": "string",
    "mcp_server_ids": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
    "owner_name": "string",
    "owner_username": "string",
    "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
    "pin_order": 0,
    "plan_mode": "plan",
    "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
    "shared": true,
    "status": "waiting",
    "title": "string",
    "updated_at": "2019-08-24T14:15:22Z",
    "warnings": [
      "string"
    ],
    "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
  },
  "kind": "status_change",
  "tool_calls": [
    {
      "args": "string",
      "tool_call_id": "string",
      "tool_name": "string"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatWatchEvent](schemas.md#codersdkchatwatchevent) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get chat by ID

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/{chat} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/{chat}`

Experimental: this endpoint is subject to change.

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `chat` | path | string(uuid) | true     | Chat ID     |

### Example responses

> 200 Response

```json
{
  "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
  "archived": true,
  "build_id": "bfb1f3fa-bf7b-43a5-9e0b-26cc050e44cb",
  "children": [
    {
      "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
      "archived": true,
      "build_id": "bfb1f3fa-bf7b-43a5-9e0b-26cc050e44cb",
      "children": [],
      "client_type": "ui",
      "context": {
        "dirty": true,
        "dirty_since": "2019-08-24T14:15:22Z",
        "error": "string",
        "resources": [
          {
            "error": "string",
            "kind": "instruction_file",
            "size_bytes": 0,
            "skill_description": "string",
            "skill_name": "string",
            "source": "string",
            "status": "ok",
            "tools": [
              {
                "description": "string",
                "name": "string"
              }
            ]
          }
        ]
      },
      "created_at": "2019-08-24T14:15:22Z",
      "diff_status": {
        "additions": 0,
        "approved": true,
        "author_avatar_url": "string",
        "author_login": "string",
        "base_branch": "string",
        "changed_files": 0,
        "changes_requested": true,
        "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
        "commits": 0,
        "deletions": 0,
        "head_branch": "string",
        "pr_number": 0,
        "pull_request_draft": true,
        "pull_request_state": "string",
        "pull_request_title": "string",
        "refreshed_at": "2019-08-24T14:15:22Z",
        "reviewer_count": 0,
        "stale_at": "2019-08-24T14:15:22Z",
        "url": "string"
      },
      "files": [
        {
          "created_at": "2019-08-24T14:15:22Z",
          "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
          "mime_type": "string",
          "name": "string",
          "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
          "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05"
        }
      ],
      "has_unread": true,
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "labels": {
        "property1": "string",
        "property2": "string"
      },
      "last_error": {
        "detail": "string",
        "kind": "generic",
        "message": "string",
        "provider": "string",
        "retryable": true,
        "status_code": 0
      },
      "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
      "last_reasoning_effort": "string",
      "last_turn_summary": "string",
      "mcp_server_ids": [
        "497f6eca-6276-4993-bfeb-53cbbbba6f08"
      ],
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
      "owner_name": "string",
      "owner_username": "string",
      "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
      "pin_order": 0,
      "plan_mode": "plan",
      "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
      "shared": true,
      "status": "waiting",
      "title": "string",
      "updated_at": "2019-08-24T14:15:22Z",
      "warnings": [
        "string"
      ],
      "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
    }
  ],
  "client_type": "ui",
  "context": {
    "dirty": true,
    "dirty_since": "2019-08-24T14:15:22Z",
    "error": "string",
    "resources": [
      {
        "error": "string",
        "kind": "instruction_file",
        "size_bytes": 0,
        "skill_description": "string",
        "skill_name": "string",
        "source": "string",
        "status": "ok",
        "tools": [
          {
            "description": "string",
            "name": "string"
          }
        ]
      }
    ]
  },
  "created_at": "2019-08-24T14:15:22Z",
  "diff_status": {
    "additions": 0,
    "approved": true,
    "author_avatar_url": "string",
    "author_login": "string",
    "base_branch": "string",
    "changed_files": 0,
    "changes_requested": true,
    "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
    "commits": 0,
    "deletions": 0,
    "head_branch": "string",
    "pr_number": 0,
    "pull_request_draft": true,
    "pull_request_state": "string",
    "pull_request_title": "string",
    "refreshed_at": "2019-08-24T14:15:22Z",
    "reviewer_count": 0,
    "stale_at": "2019-08-24T14:15:22Z",
    "url": "string"
  },
  "files": [
    {
      "created_at": "2019-08-24T14:15:22Z",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "mime_type": "string",
      "name": "string",
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05"
    }
  ],
  "has_unread": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "labels": {
    "property1": "string",
    "property2": "string"
  },
  "last_error": {
    "detail": "string",
    "kind": "generic",
    "message": "string",
    "provider": "string",
    "retryable": true,
    "status_code": 0
  },
  "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
  "last_reasoning_effort": "string",
  "last_turn_summary": "string",
  "mcp_server_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "owner_name": "string",
  "owner_username": "string",
  "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
  "pin_order": 0,
  "plan_mode": "plan",
  "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
  "shared": true,
  "status": "waiting",
  "title": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "warnings": [
    "string"
  ],
  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Chat](schemas.md#codersdkchat) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update chat

### Code samples

```sh
# Example request using curl
curl -X PATCH http://coder-server:8080/api/experimental/chats/{chat} \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /api/experimental/chats/{chat}`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "archived": true,
  "labels": {
    "property1": "string",
    "property2": "string"
  },
  "pin_order": 0,
  "plan_mode": "plan",
  "title": "string",
  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
}
```

### Parameters

| Name   | In   | Type                                                               | Required | Description         |
|--------|------|--------------------------------------------------------------------|----------|---------------------|
| `chat` | path | string(uuid)                                                       | true     | Chat ID             |
| `body` | body | [codersdk.UpdateChatRequest](schemas.md#codersdkupdatechatrequest) | true     | Update chat request |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Refresh chat context

### Code samples

```sh
# Example request using curl
curl -X PUT http://coder-server:8080/api/experimental/chats/{chat}/context \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /api/experimental/chats/{chat}/context`

Experimental: this endpoint is subject to change.

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `chat` | path | string(uuid) | true     | Chat ID     |

### Example responses

> 200 Response

```json
{
  "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
  "archived": true,
  "build_id": "bfb1f3fa-bf7b-43a5-9e0b-26cc050e44cb",
  "children": [
    {
      "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
      "archived": true,
      "build_id": "bfb1f3fa-bf7b-43a5-9e0b-26cc050e44cb",
      "children": [],
      "client_type": "ui",
      "context": {
        "dirty": true,
        "dirty_since": "2019-08-24T14:15:22Z",
        "error": "string",
        "resources": [
          {
            "error": "string",
            "kind": "instruction_file",
            "size_bytes": 0,
            "skill_description": "string",
            "skill_name": "string",
            "source": "string",
            "status": "ok",
            "tools": [
              {
                "description": "string",
                "name": "string"
              }
            ]
          }
        ]
      },
      "created_at": "2019-08-24T14:15:22Z",
      "diff_status": {
        "additions": 0,
        "approved": true,
        "author_avatar_url": "string",
        "author_login": "string",
        "base_branch": "string",
        "changed_files": 0,
        "changes_requested": true,
        "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
        "commits": 0,
        "deletions": 0,
        "head_branch": "string",
        "pr_number": 0,
        "pull_request_draft": true,
        "pull_request_state": "string",
        "pull_request_title": "string",
        "refreshed_at": "2019-08-24T14:15:22Z",
        "reviewer_count": 0,
        "stale_at": "2019-08-24T14:15:22Z",
        "url": "string"
      },
      "files": [
        {
          "created_at": "2019-08-24T14:15:22Z",
          "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
          "mime_type": "string",
          "name": "string",
          "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
          "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05"
        }
      ],
      "has_unread": true,
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "labels": {
        "property1": "string",
        "property2": "string"
      },
      "last_error": {
        "detail": "string",
        "kind": "generic",
        "message": "string",
        "provider": "string",
        "retryable": true,
        "status_code": 0
      },
      "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
      "last_reasoning_effort": "string",
      "last_turn_summary": "string",
      "mcp_server_ids": [
        "497f6eca-6276-4993-bfeb-53cbbbba6f08"
      ],
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
      "owner_name": "string",
      "owner_username": "string",
      "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
      "pin_order": 0,
      "plan_mode": "plan",
      "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
      "shared": true,
      "status": "waiting",
      "title": "string",
      "updated_at": "2019-08-24T14:15:22Z",
      "warnings": [
        "string"
      ],
      "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
    }
  ],
  "client_type": "ui",
  "context": {
    "dirty": true,
    "dirty_since": "2019-08-24T14:15:22Z",
    "error": "string",
    "resources": [
      {
        "error": "string",
        "kind": "instruction_file",
        "size_bytes": 0,
        "skill_description": "string",
        "skill_name": "string",
        "source": "string",
        "status": "ok",
        "tools": [
          {
            "description": "string",
            "name": "string"
          }
        ]
      }
    ]
  },
  "created_at": "2019-08-24T14:15:22Z",
  "diff_status": {
    "additions": 0,
    "approved": true,
    "author_avatar_url": "string",
    "author_login": "string",
    "base_branch": "string",
    "changed_files": 0,
    "changes_requested": true,
    "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
    "commits": 0,
    "deletions": 0,
    "head_branch": "string",
    "pr_number": 0,
    "pull_request_draft": true,
    "pull_request_state": "string",
    "pull_request_title": "string",
    "refreshed_at": "2019-08-24T14:15:22Z",
    "reviewer_count": 0,
    "stale_at": "2019-08-24T14:15:22Z",
    "url": "string"
  },
  "files": [
    {
      "created_at": "2019-08-24T14:15:22Z",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "mime_type": "string",
      "name": "string",
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05"
    }
  ],
  "has_unread": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "labels": {
    "property1": "string",
    "property2": "string"
  },
  "last_error": {
    "detail": "string",
    "kind": "generic",
    "message": "string",
    "provider": "string",
    "retryable": true,
    "status_code": 0
  },
  "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
  "last_reasoning_effort": "string",
  "last_turn_summary": "string",
  "mcp_server_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "owner_name": "string",
  "owner_username": "string",
  "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
  "pin_order": 0,
  "plan_mode": "plan",
  "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
  "shared": true,
  "status": "waiting",
  "title": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "warnings": [
    "string"
  ],
  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Chat](schemas.md#codersdkchat) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get chat diff contents

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/{chat}/diff \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/{chat}/diff`

Experimental: this endpoint is subject to change.

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `chat` | path | string(uuid) | true     | Chat ID     |

### Example responses

> 200 Response

```json
{
  "branch": "string",
  "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
  "diff": "string",
  "provider": "string",
  "pull_request_url": "string",
  "remote_origin": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                           |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatDiffContents](schemas.md#codersdkchatdiffcontents) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Interrupt chat

### Code samples

```sh
# Example request using curl
curl -X POST http://coder-server:8080/api/experimental/chats/{chat}/interrupt \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/experimental/chats/{chat}/interrupt`

Experimental: this endpoint is subject to change.

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `chat` | path | string(uuid) | true     | Chat ID     |

### Example responses

> 200 Response

```json
{
  "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
  "archived": true,
  "build_id": "bfb1f3fa-bf7b-43a5-9e0b-26cc050e44cb",
  "children": [
    {
      "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
      "archived": true,
      "build_id": "bfb1f3fa-bf7b-43a5-9e0b-26cc050e44cb",
      "children": [],
      "client_type": "ui",
      "context": {
        "dirty": true,
        "dirty_since": "2019-08-24T14:15:22Z",
        "error": "string",
        "resources": [
          {
            "error": "string",
            "kind": "instruction_file",
            "size_bytes": 0,
            "skill_description": "string",
            "skill_name": "string",
            "source": "string",
            "status": "ok",
            "tools": [
              {
                "description": "string",
                "name": "string"
              }
            ]
          }
        ]
      },
      "created_at": "2019-08-24T14:15:22Z",
      "diff_status": {
        "additions": 0,
        "approved": true,
        "author_avatar_url": "string",
        "author_login": "string",
        "base_branch": "string",
        "changed_files": 0,
        "changes_requested": true,
        "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
        "commits": 0,
        "deletions": 0,
        "head_branch": "string",
        "pr_number": 0,
        "pull_request_draft": true,
        "pull_request_state": "string",
        "pull_request_title": "string",
        "refreshed_at": "2019-08-24T14:15:22Z",
        "reviewer_count": 0,
        "stale_at": "2019-08-24T14:15:22Z",
        "url": "string"
      },
      "files": [
        {
          "created_at": "2019-08-24T14:15:22Z",
          "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
          "mime_type": "string",
          "name": "string",
          "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
          "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05"
        }
      ],
      "has_unread": true,
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "labels": {
        "property1": "string",
        "property2": "string"
      },
      "last_error": {
        "detail": "string",
        "kind": "generic",
        "message": "string",
        "provider": "string",
        "retryable": true,
        "status_code": 0
      },
      "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
      "last_reasoning_effort": "string",
      "last_turn_summary": "string",
      "mcp_server_ids": [
        "497f6eca-6276-4993-bfeb-53cbbbba6f08"
      ],
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
      "owner_name": "string",
      "owner_username": "string",
      "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
      "pin_order": 0,
      "plan_mode": "plan",
      "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
      "shared": true,
      "status": "waiting",
      "title": "string",
      "updated_at": "2019-08-24T14:15:22Z",
      "warnings": [
        "string"
      ],
      "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
    }
  ],
  "client_type": "ui",
  "context": {
    "dirty": true,
    "dirty_since": "2019-08-24T14:15:22Z",
    "error": "string",
    "resources": [
      {
        "error": "string",
        "kind": "instruction_file",
        "size_bytes": 0,
        "skill_description": "string",
        "skill_name": "string",
        "source": "string",
        "status": "ok",
        "tools": [
          {
            "description": "string",
            "name": "string"
          }
        ]
      }
    ]
  },
  "created_at": "2019-08-24T14:15:22Z",
  "diff_status": {
    "additions": 0,
    "approved": true,
    "author_avatar_url": "string",
    "author_login": "string",
    "base_branch": "string",
    "changed_files": 0,
    "changes_requested": true,
    "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
    "commits": 0,
    "deletions": 0,
    "head_branch": "string",
    "pr_number": 0,
    "pull_request_draft": true,
    "pull_request_state": "string",
    "pull_request_title": "string",
    "refreshed_at": "2019-08-24T14:15:22Z",
    "reviewer_count": 0,
    "stale_at": "2019-08-24T14:15:22Z",
    "url": "string"
  },
  "files": [
    {
      "created_at": "2019-08-24T14:15:22Z",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "mime_type": "string",
      "name": "string",
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05"
    }
  ],
  "has_unread": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "labels": {
    "property1": "string",
    "property2": "string"
  },
  "last_error": {
    "detail": "string",
    "kind": "generic",
    "message": "string",
    "provider": "string",
    "retryable": true,
    "status_code": 0
  },
  "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
  "last_reasoning_effort": "string",
  "last_turn_summary": "string",
  "mcp_server_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "owner_name": "string",
  "owner_username": "string",
  "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
  "pin_order": 0,
  "plan_mode": "plan",
  "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
  "shared": true,
  "status": "waiting",
  "title": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "warnings": [
    "string"
  ],
  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Chat](schemas.md#codersdkchat) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List chat messages

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/{chat}/messages \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/{chat}/messages`

Experimental: this endpoint is subject to change.

### Parameters

| Name        | In    | Type         | Required | Description                          |
|-------------|-------|--------------|----------|--------------------------------------|
| `chat`      | path  | string(uuid) | true     | Chat ID                              |
| `before_id` | query | integer      | false    | Return messages with id < before_id  |
| `after_id`  | query | integer      | false    | Return messages with id > after_id   |
| `limit`     | query | integer      | false    | Page size, 1 to 200. Defaults to 50. |

### Example responses

> 200 Response

```json
{
  "has_more": true,
  "messages": [
    {
      "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
      "content": [
        {
          "args": [
            0
          ],
          "args_delta": "string",
          "completed_at": "2019-08-24T14:15:22Z",
          "content": "string",
          "context_file_agent_id": {
            "uuid": "string",
            "valid": true
          },
          "context_file_content": "string",
          "context_file_directory": "string",
          "context_file_os": "string",
          "context_file_path": "string",
          "context_file_skill_meta_file": "string",
          "context_file_truncated": true,
          "created_at": "2019-08-24T14:15:22Z",
          "data": [
            0
          ],
          "end_line": 0,
          "file_id": {
            "uuid": "string",
            "valid": true
          },
          "file_name": "string",
          "is_error": true,
          "is_media": true,
          "mcp_server_config_id": {
            "uuid": "string",
            "valid": true
          },
          "media_type": "string",
          "name": "string",
          "parsed_commands": [
            [
              "string"
            ]
          ],
          "provider_executed": true,
          "provider_metadata": [
            0
          ],
          "result": [
            0
          ],
          "result_delta": "string",
          "result_reset": true,
          "signature": "string",
          "skill_description": "string",
          "skill_dir": "string",
          "skill_name": "string",
          "source_id": "string",
          "start_line": 0,
          "text": "string",
          "title": "string",
          "tool_call_id": "string",
          "tool_name": "string",
          "type": "text",
          "url": "string"
        }
      ],
      "created_at": "2019-08-24T14:15:22Z",
      "created_by": "ee824cad-d7a6-4f48-87dc-e8461a9201c4",
      "id": 0,
      "model_config_id": "f5fb4d91-62ca-4377-9ee6-5d43ba00d205",
      "role": "system",
      "usage": {
        "cache_creation_tokens": 0,
        "cache_read_tokens": 0,
        "context_limit": 0,
        "input_tokens": 0,
        "output_tokens": 0,
        "reasoning_tokens": 0,
        "total_tokens": 0
      }
    }
  ],
  "queued_messages": [
    {
      "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
      "content": [
        {
          "args": [
            0
          ],
          "args_delta": "string",
          "completed_at": "2019-08-24T14:15:22Z",
          "content": "string",
          "context_file_agent_id": {
            "uuid": "string",
            "valid": true
          },
          "context_file_content": "string",
          "context_file_directory": "string",
          "context_file_os": "string",
          "context_file_path": "string",
          "context_file_skill_meta_file": "string",
          "context_file_truncated": true,
          "created_at": "2019-08-24T14:15:22Z",
          "data": [
            0
          ],
          "end_line": 0,
          "file_id": {
            "uuid": "string",
            "valid": true
          },
          "file_name": "string",
          "is_error": true,
          "is_media": true,
          "mcp_server_config_id": {
            "uuid": "string",
            "valid": true
          },
          "media_type": "string",
          "name": "string",
          "parsed_commands": [
            [
              "string"
            ]
          ],
          "provider_executed": true,
          "provider_metadata": [
            0
          ],
          "result": [
            0
          ],
          "result_delta": "string",
          "result_reset": true,
          "signature": "string",
          "skill_description": "string",
          "skill_dir": "string",
          "skill_name": "string",
          "source_id": "string",
          "start_line": 0,
          "text": "string",
          "title": "string",
          "tool_call_id": "string",
          "tool_name": "string",
          "type": "text",
          "url": "string"
        }
      ],
      "created_at": "2019-08-24T14:15:22Z",
      "id": 0,
      "model_config_id": "f5fb4d91-62ca-4377-9ee6-5d43ba00d205"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                   |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatMessagesResponse](schemas.md#codersdkchatmessagesresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Send chat message

### Code samples

```sh
# Example request using curl
curl -X POST http://coder-server:8080/api/experimental/chats/{chat}/messages \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/experimental/chats/{chat}/messages`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "busy_behavior": "queue",
  "content": [
    {
      "content": "string",
      "end_line": 0,
      "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
      "file_name": "string",
      "start_line": 0,
      "text": "string",
      "type": "text"
    }
  ],
  "mcp_server_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "model_config_id": "f5fb4d91-62ca-4377-9ee6-5d43ba00d205",
  "plan_mode": "plan",
  "reasoning_effort": "string"
}
```

### Parameters

| Name   | In   | Type                                                                             | Required | Description                 |
|--------|------|----------------------------------------------------------------------------------|----------|-----------------------------|
| `chat` | path | string(uuid)                                                                     | true     | Chat ID                     |
| `body` | body | [codersdk.CreateChatMessageRequest](schemas.md#codersdkcreatechatmessagerequest) | true     | Create chat message request |

### Example responses

> 200 Response

```json
{
  "message": {
    "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
    "content": [
      {
        "args": [
          0
        ],
        "args_delta": "string",
        "completed_at": "2019-08-24T14:15:22Z",
        "content": "string",
        "context_file_agent_id": {
          "uuid": "string",
          "valid": true
        },
        "context_file_content": "string",
        "context_file_directory": "string",
        "context_file_os": "string",
        "context_file_path": "string",
        "context_file_skill_meta_file": "string",
        "context_file_truncated": true,
        "created_at": "2019-08-24T14:15:22Z",
        "data": [
          0
        ],
        "end_line": 0,
        "file_id": {
          "uuid": "string",
          "valid": true
        },
        "file_name": "string",
        "is_error": true,
        "is_media": true,
        "mcp_server_config_id": {
          "uuid": "string",
          "valid": true
        },
        "media_type": "string",
        "name": "string",
        "parsed_commands": [
          [
            "string"
          ]
        ],
        "provider_executed": true,
        "provider_metadata": [
          0
        ],
        "result": [
          0
        ],
        "result_delta": "string",
        "result_reset": true,
        "signature": "string",
        "skill_description": "string",
        "skill_dir": "string",
        "skill_name": "string",
        "source_id": "string",
        "start_line": 0,
        "text": "string",
        "title": "string",
        "tool_call_id": "string",
        "tool_name": "string",
        "type": "text",
        "url": "string"
      }
    ],
    "created_at": "2019-08-24T14:15:22Z",
    "created_by": "ee824cad-d7a6-4f48-87dc-e8461a9201c4",
    "id": 0,
    "model_config_id": "f5fb4d91-62ca-4377-9ee6-5d43ba00d205",
    "role": "system",
    "usage": {
      "cache_creation_tokens": 0,
      "cache_read_tokens": 0,
      "context_limit": 0,
      "input_tokens": 0,
      "output_tokens": 0,
      "reasoning_tokens": 0,
      "total_tokens": 0
    }
  },
  "queued": true,
  "queued_message": {
    "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
    "content": [
      {
        "args": [
          0
        ],
        "args_delta": "string",
        "completed_at": "2019-08-24T14:15:22Z",
        "content": "string",
        "context_file_agent_id": {
          "uuid": "string",
          "valid": true
        },
        "context_file_content": "string",
        "context_file_directory": "string",
        "context_file_os": "string",
        "context_file_path": "string",
        "context_file_skill_meta_file": "string",
        "context_file_truncated": true,
        "created_at": "2019-08-24T14:15:22Z",
        "data": [
          0
        ],
        "end_line": 0,
        "file_id": {
          "uuid": "string",
          "valid": true
        },
        "file_name": "string",
        "is_error": true,
        "is_media": true,
        "mcp_server_config_id": {
          "uuid": "string",
          "valid": true
        },
        "media_type": "string",
        "name": "string",
        "parsed_commands": [
          [
            "string"
          ]
        ],
        "provider_executed": true,
        "provider_metadata": [
          0
        ],
        "result": [
          0
        ],
        "result_delta": "string",
        "result_reset": true,
        "signature": "string",
        "skill_description": "string",
        "skill_dir": "string",
        "skill_name": "string",
        "source_id": "string",
        "start_line": 0,
        "text": "string",
        "title": "string",
        "tool_call_id": "string",
        "tool_name": "string",
        "type": "text",
        "url": "string"
      }
    ],
    "created_at": "2019-08-24T14:15:22Z",
    "id": 0,
    "model_config_id": "f5fb4d91-62ca-4377-9ee6-5d43ba00d205"
  },
  "warnings": [
    "string"
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                             |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.CreateChatMessageResponse](schemas.md#codersdkcreatechatmessageresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Edit chat message

### Code samples

```sh
# Example request using curl
curl -X PATCH http://coder-server:8080/api/experimental/chats/{chat}/messages/{message} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /api/experimental/chats/{chat}/messages/{message}`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "content": [
    {
      "content": "string",
      "end_line": 0,
      "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
      "file_name": "string",
      "start_line": 0,
      "text": "string",
      "type": "text"
    }
  ],
  "model_config_id": "f5fb4d91-62ca-4377-9ee6-5d43ba00d205",
  "reasoning_effort": "string"
}
```

### Parameters

| Name      | In   | Type                                                                         | Required | Description               |
|-----------|------|------------------------------------------------------------------------------|----------|---------------------------|
| `chat`    | path | string(uuid)                                                                 | true     | Chat ID                   |
| `message` | path | integer                                                                      | true     | Message ID                |
| `body`    | body | [codersdk.EditChatMessageRequest](schemas.md#codersdkeditchatmessagerequest) | true     | Edit chat message request |

### Example responses

> 200 Response

```json
{
  "message": {
    "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
    "content": [
      {
        "args": [
          0
        ],
        "args_delta": "string",
        "completed_at": "2019-08-24T14:15:22Z",
        "content": "string",
        "context_file_agent_id": {
          "uuid": "string",
          "valid": true
        },
        "context_file_content": "string",
        "context_file_directory": "string",
        "context_file_os": "string",
        "context_file_path": "string",
        "context_file_skill_meta_file": "string",
        "context_file_truncated": true,
        "created_at": "2019-08-24T14:15:22Z",
        "data": [
          0
        ],
        "end_line": 0,
        "file_id": {
          "uuid": "string",
          "valid": true
        },
        "file_name": "string",
        "is_error": true,
        "is_media": true,
        "mcp_server_config_id": {
          "uuid": "string",
          "valid": true
        },
        "media_type": "string",
        "name": "string",
        "parsed_commands": [
          [
            "string"
          ]
        ],
        "provider_executed": true,
        "provider_metadata": [
          0
        ],
        "result": [
          0
        ],
        "result_delta": "string",
        "result_reset": true,
        "signature": "string",
        "skill_description": "string",
        "skill_dir": "string",
        "skill_name": "string",
        "source_id": "string",
        "start_line": 0,
        "text": "string",
        "title": "string",
        "tool_call_id": "string",
        "tool_name": "string",
        "type": "text",
        "url": "string"
      }
    ],
    "created_at": "2019-08-24T14:15:22Z",
    "created_by": "ee824cad-d7a6-4f48-87dc-e8461a9201c4",
    "id": 0,
    "model_config_id": "f5fb4d91-62ca-4377-9ee6-5d43ba00d205",
    "role": "system",
    "usage": {
      "cache_creation_tokens": 0,
      "cache_read_tokens": 0,
      "context_limit": 0,
      "input_tokens": 0,
      "output_tokens": 0,
      "reasoning_tokens": 0,
      "total_tokens": 0
    }
  },
  "warnings": [
    "string"
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                         |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.EditChatMessageResponse](schemas.md#codersdkeditchatmessageresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List chat user prompts

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/{chat}/prompts \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/{chat}/prompts`

Experimental: this endpoint is subject to change.

Returns the user-authored prompts in a chat, newest first,
with each prompt's text parts concatenated in the order they
were authored. Used by the composer to power the up/down
arrow prompt-history cycle without paging through every
message in the chat.

### Parameters

| Name    | In    | Type         | Required | Description                                                                 |
|---------|-------|--------------|----------|-----------------------------------------------------------------------------|
| `chat`  | path  | string(uuid) | true     | Chat ID                                                                     |
| `limit` | query | integer      | false    | Page size, 0 to 2000. 0 (the default) means the server-side default of 500. |

### Example responses

> 200 Response

```json
{
  "prompts": [
    {
      "id": 0,
      "text": "string"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                 |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatPromptsResponse](schemas.md#codersdkchatpromptsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Reconcile invalid chat state

### Code samples

```sh
# Example request using curl
curl -X POST http://coder-server:8080/api/experimental/chats/{chat}/reconcile-invalid \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/experimental/chats/{chat}/reconcile-invalid`

Experimental: this endpoint is subject to change.

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `chat` | path | string(uuid) | true     | Chat ID     |

### Example responses

> 200 Response

```json
{
  "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
  "archived": true,
  "build_id": "bfb1f3fa-bf7b-43a5-9e0b-26cc050e44cb",
  "children": [
    {
      "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
      "archived": true,
      "build_id": "bfb1f3fa-bf7b-43a5-9e0b-26cc050e44cb",
      "children": [],
      "client_type": "ui",
      "context": {
        "dirty": true,
        "dirty_since": "2019-08-24T14:15:22Z",
        "error": "string",
        "resources": [
          {
            "error": "string",
            "kind": "instruction_file",
            "size_bytes": 0,
            "skill_description": "string",
            "skill_name": "string",
            "source": "string",
            "status": "ok",
            "tools": [
              {
                "description": "string",
                "name": "string"
              }
            ]
          }
        ]
      },
      "created_at": "2019-08-24T14:15:22Z",
      "diff_status": {
        "additions": 0,
        "approved": true,
        "author_avatar_url": "string",
        "author_login": "string",
        "base_branch": "string",
        "changed_files": 0,
        "changes_requested": true,
        "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
        "commits": 0,
        "deletions": 0,
        "head_branch": "string",
        "pr_number": 0,
        "pull_request_draft": true,
        "pull_request_state": "string",
        "pull_request_title": "string",
        "refreshed_at": "2019-08-24T14:15:22Z",
        "reviewer_count": 0,
        "stale_at": "2019-08-24T14:15:22Z",
        "url": "string"
      },
      "files": [
        {
          "created_at": "2019-08-24T14:15:22Z",
          "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
          "mime_type": "string",
          "name": "string",
          "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
          "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05"
        }
      ],
      "has_unread": true,
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "labels": {
        "property1": "string",
        "property2": "string"
      },
      "last_error": {
        "detail": "string",
        "kind": "generic",
        "message": "string",
        "provider": "string",
        "retryable": true,
        "status_code": 0
      },
      "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
      "last_reasoning_effort": "string",
      "last_turn_summary": "string",
      "mcp_server_ids": [
        "497f6eca-6276-4993-bfeb-53cbbbba6f08"
      ],
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
      "owner_name": "string",
      "owner_username": "string",
      "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
      "pin_order": 0,
      "plan_mode": "plan",
      "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
      "shared": true,
      "status": "waiting",
      "title": "string",
      "updated_at": "2019-08-24T14:15:22Z",
      "warnings": [
        "string"
      ],
      "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
    }
  ],
  "client_type": "ui",
  "context": {
    "dirty": true,
    "dirty_since": "2019-08-24T14:15:22Z",
    "error": "string",
    "resources": [
      {
        "error": "string",
        "kind": "instruction_file",
        "size_bytes": 0,
        "skill_description": "string",
        "skill_name": "string",
        "source": "string",
        "status": "ok",
        "tools": [
          {
            "description": "string",
            "name": "string"
          }
        ]
      }
    ]
  },
  "created_at": "2019-08-24T14:15:22Z",
  "diff_status": {
    "additions": 0,
    "approved": true,
    "author_avatar_url": "string",
    "author_login": "string",
    "base_branch": "string",
    "changed_files": 0,
    "changes_requested": true,
    "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
    "commits": 0,
    "deletions": 0,
    "head_branch": "string",
    "pr_number": 0,
    "pull_request_draft": true,
    "pull_request_state": "string",
    "pull_request_title": "string",
    "refreshed_at": "2019-08-24T14:15:22Z",
    "reviewer_count": 0,
    "stale_at": "2019-08-24T14:15:22Z",
    "url": "string"
  },
  "files": [
    {
      "created_at": "2019-08-24T14:15:22Z",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "mime_type": "string",
      "name": "string",
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05"
    }
  ],
  "has_unread": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "labels": {
    "property1": "string",
    "property2": "string"
  },
  "last_error": {
    "detail": "string",
    "kind": "generic",
    "message": "string",
    "provider": "string",
    "retryable": true,
    "status_code": 0
  },
  "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
  "last_reasoning_effort": "string",
  "last_turn_summary": "string",
  "mcp_server_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "owner_name": "string",
  "owner_username": "string",
  "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
  "pin_order": 0,
  "plan_mode": "plan",
  "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
  "shared": true,
  "status": "waiting",
  "title": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "warnings": [
    "string"
  ],
  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Chat](schemas.md#codersdkchat) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Stream chat events via WebSockets

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/{chat}/stream \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/{chat}/stream`

Experimental: this endpoint is subject to change.

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `chat` | path | string(uuid) | true     | Chat ID     |

### Example responses

> 200 Response

```json
{
  "action_required": {
    "tool_calls": [
      {
        "args": "string",
        "tool_call_id": "string",
        "tool_name": "string"
      }
    ]
  },
  "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
  "error": {
    "detail": "string",
    "kind": "generic",
    "message": "string",
    "provider": "string",
    "retryable": true,
    "status_code": 0
  },
  "message": {
    "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
    "content": [
      {
        "args": [
          0
        ],
        "args_delta": "string",
        "completed_at": "2019-08-24T14:15:22Z",
        "content": "string",
        "context_file_agent_id": {
          "uuid": "string",
          "valid": true
        },
        "context_file_content": "string",
        "context_file_directory": "string",
        "context_file_os": "string",
        "context_file_path": "string",
        "context_file_skill_meta_file": "string",
        "context_file_truncated": true,
        "created_at": "2019-08-24T14:15:22Z",
        "data": [
          0
        ],
        "end_line": 0,
        "file_id": {
          "uuid": "string",
          "valid": true
        },
        "file_name": "string",
        "is_error": true,
        "is_media": true,
        "mcp_server_config_id": {
          "uuid": "string",
          "valid": true
        },
        "media_type": "string",
        "name": "string",
        "parsed_commands": [
          [
            "string"
          ]
        ],
        "provider_executed": true,
        "provider_metadata": [
          0
        ],
        "result": [
          0
        ],
        "result_delta": "string",
        "result_reset": true,
        "signature": "string",
        "skill_description": "string",
        "skill_dir": "string",
        "skill_name": "string",
        "source_id": "string",
        "start_line": 0,
        "text": "string",
        "title": "string",
        "tool_call_id": "string",
        "tool_name": "string",
        "type": "text",
        "url": "string"
      }
    ],
    "created_at": "2019-08-24T14:15:22Z",
    "created_by": "ee824cad-d7a6-4f48-87dc-e8461a9201c4",
    "id": 0,
    "model_config_id": "f5fb4d91-62ca-4377-9ee6-5d43ba00d205",
    "role": "system",
    "usage": {
      "cache_creation_tokens": 0,
      "cache_read_tokens": 0,
      "context_limit": 0,
      "input_tokens": 0,
      "output_tokens": 0,
      "reasoning_tokens": 0,
      "total_tokens": 0
    }
  },
  "message_part": {
    "generation_attempt": 0,
    "history_version": 0,
    "part": {
      "args": [
        0
      ],
      "args_delta": "string",
      "completed_at": "2019-08-24T14:15:22Z",
      "content": "string",
      "context_file_agent_id": {
        "uuid": "string",
        "valid": true
      },
      "context_file_content": "string",
      "context_file_directory": "string",
      "context_file_os": "string",
      "context_file_path": "string",
      "context_file_skill_meta_file": "string",
      "context_file_truncated": true,
      "created_at": "2019-08-24T14:15:22Z",
      "data": [
        0
      ],
      "end_line": 0,
      "file_id": {
        "uuid": "string",
        "valid": true
      },
      "file_name": "string",
      "is_error": true,
      "is_media": true,
      "mcp_server_config_id": {
        "uuid": "string",
        "valid": true
      },
      "media_type": "string",
      "name": "string",
      "parsed_commands": [
        [
          "string"
        ]
      ],
      "provider_executed": true,
      "provider_metadata": [
        0
      ],
      "result": [
        0
      ],
      "result_delta": "string",
      "result_reset": true,
      "signature": "string",
      "skill_description": "string",
      "skill_dir": "string",
      "skill_name": "string",
      "source_id": "string",
      "start_line": 0,
      "text": "string",
      "title": "string",
      "tool_call_id": "string",
      "tool_name": "string",
      "type": "text",
      "url": "string"
    },
    "role": "system",
    "seq": 0
  },
  "queued_messages": [
    {
      "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
      "content": [
        {
          "args": [
            0
          ],
          "args_delta": "string",
          "completed_at": "2019-08-24T14:15:22Z",
          "content": "string",
          "context_file_agent_id": {
            "uuid": "string",
            "valid": true
          },
          "context_file_content": "string",
          "context_file_directory": "string",
          "context_file_os": "string",
          "context_file_path": "string",
          "context_file_skill_meta_file": "string",
          "context_file_truncated": true,
          "created_at": "2019-08-24T14:15:22Z",
          "data": [
            0
          ],
          "end_line": 0,
          "file_id": {
            "uuid": "string",
            "valid": true
          },
          "file_name": "string",
          "is_error": true,
          "is_media": true,
          "mcp_server_config_id": {
            "uuid": "string",
            "valid": true
          },
          "media_type": "string",
          "name": "string",
          "parsed_commands": [
            [
              "string"
            ]
          ],
          "provider_executed": true,
          "provider_metadata": [
            0
          ],
          "result": [
            0
          ],
          "result_delta": "string",
          "result_reset": true,
          "signature": "string",
          "skill_description": "string",
          "skill_dir": "string",
          "skill_name": "string",
          "source_id": "string",
          "start_line": 0,
          "text": "string",
          "title": "string",
          "tool_call_id": "string",
          "tool_name": "string",
          "type": "text",
          "url": "string"
        }
      ],
      "created_at": "2019-08-24T14:15:22Z",
      "id": 0,
      "model_config_id": "f5fb4d91-62ca-4377-9ee6-5d43ba00d205"
    }
  ],
  "retry": {
    "attempt": 0,
    "delay_ms": 0,
    "error": "string",
    "kind": "generic",
    "provider": "string",
    "retrying_at": "2019-08-24T14:15:22Z",
    "status_code": 0
  },
  "status": {
    "status": "waiting"
  },
  "type": "message_part"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                         |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatStreamEvent](schemas.md#codersdkchatstreamevent) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Connect to chat workspace desktop via WebSockets

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/{chat}/stream/desktop \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/{chat}/stream/desktop`

Raw binary WebSocket stream of the chat workspace desktop.
Experimental: this endpoint is subject to change.

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `chat` | path | string(uuid) | true     | Chat ID     |

### Responses

| Status | Meaning                                                                  | Description         | Schema |
|--------|--------------------------------------------------------------------------|---------------------|--------|
| 101    | [Switching Protocols](https://tools.ietf.org/html/rfc7231#section-6.2.2) | Switching Protocols |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Watch chat workspace git state via WebSockets

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/{chat}/stream/git \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/{chat}/stream/git`

Experimental: this endpoint is subject to change.

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `chat` | path | string(uuid) | true     | Chat ID     |

### Example responses

> 200 Response

```json
{
  "message": "string",
  "repositories": [
    {
      "branch": "string",
      "remote_origin": "string",
      "removed": true,
      "repo_root": "string",
      "unified_diff": "string"
    }
  ],
  "scanned_at": "2019-08-24T14:15:22Z",
  "type": "changes"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                       |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.WorkspaceAgentGitServerMessage](schemas.md#codersdkworkspaceagentgitservermessage) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Regenerate chat title

### Code samples

```sh
# Example request using curl
curl -X POST http://coder-server:8080/api/experimental/chats/{chat}/title/regenerate \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/experimental/chats/{chat}/title/regenerate`

Experimental: this endpoint is subject to change.

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `chat` | path | string(uuid) | true     | Chat ID     |

### Example responses

> 200 Response

```json
{
  "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
  "archived": true,
  "build_id": "bfb1f3fa-bf7b-43a5-9e0b-26cc050e44cb",
  "children": [
    {
      "agent_id": "2b1e3b65-2c04-4fa2-a2d7-467901e98978",
      "archived": true,
      "build_id": "bfb1f3fa-bf7b-43a5-9e0b-26cc050e44cb",
      "children": [],
      "client_type": "ui",
      "context": {
        "dirty": true,
        "dirty_since": "2019-08-24T14:15:22Z",
        "error": "string",
        "resources": [
          {
            "error": "string",
            "kind": "instruction_file",
            "size_bytes": 0,
            "skill_description": "string",
            "skill_name": "string",
            "source": "string",
            "status": "ok",
            "tools": [
              {
                "description": "string",
                "name": "string"
              }
            ]
          }
        ]
      },
      "created_at": "2019-08-24T14:15:22Z",
      "diff_status": {
        "additions": 0,
        "approved": true,
        "author_avatar_url": "string",
        "author_login": "string",
        "base_branch": "string",
        "changed_files": 0,
        "changes_requested": true,
        "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
        "commits": 0,
        "deletions": 0,
        "head_branch": "string",
        "pr_number": 0,
        "pull_request_draft": true,
        "pull_request_state": "string",
        "pull_request_title": "string",
        "refreshed_at": "2019-08-24T14:15:22Z",
        "reviewer_count": 0,
        "stale_at": "2019-08-24T14:15:22Z",
        "url": "string"
      },
      "files": [
        {
          "created_at": "2019-08-24T14:15:22Z",
          "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
          "mime_type": "string",
          "name": "string",
          "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
          "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05"
        }
      ],
      "has_unread": true,
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "labels": {
        "property1": "string",
        "property2": "string"
      },
      "last_error": {
        "detail": "string",
        "kind": "generic",
        "message": "string",
        "provider": "string",
        "retryable": true,
        "status_code": 0
      },
      "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
      "last_reasoning_effort": "string",
      "last_turn_summary": "string",
      "mcp_server_ids": [
        "497f6eca-6276-4993-bfeb-53cbbbba6f08"
      ],
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
      "owner_name": "string",
      "owner_username": "string",
      "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
      "pin_order": 0,
      "plan_mode": "plan",
      "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
      "shared": true,
      "status": "waiting",
      "title": "string",
      "updated_at": "2019-08-24T14:15:22Z",
      "warnings": [
        "string"
      ],
      "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
    }
  ],
  "client_type": "ui",
  "context": {
    "dirty": true,
    "dirty_since": "2019-08-24T14:15:22Z",
    "error": "string",
    "resources": [
      {
        "error": "string",
        "kind": "instruction_file",
        "size_bytes": 0,
        "skill_description": "string",
        "skill_name": "string",
        "source": "string",
        "status": "ok",
        "tools": [
          {
            "description": "string",
            "name": "string"
          }
        ]
      }
    ]
  },
  "created_at": "2019-08-24T14:15:22Z",
  "diff_status": {
    "additions": 0,
    "approved": true,
    "author_avatar_url": "string",
    "author_login": "string",
    "base_branch": "string",
    "changed_files": 0,
    "changes_requested": true,
    "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
    "commits": 0,
    "deletions": 0,
    "head_branch": "string",
    "pr_number": 0,
    "pull_request_draft": true,
    "pull_request_state": "string",
    "pull_request_title": "string",
    "refreshed_at": "2019-08-24T14:15:22Z",
    "reviewer_count": 0,
    "stale_at": "2019-08-24T14:15:22Z",
    "url": "string"
  },
  "files": [
    {
      "created_at": "2019-08-24T14:15:22Z",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "mime_type": "string",
      "name": "string",
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05"
    }
  ],
  "has_unread": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "labels": {
    "property1": "string",
    "property2": "string"
  },
  "last_error": {
    "detail": "string",
    "kind": "generic",
    "message": "string",
    "provider": "string",
    "retryable": true,
    "status_code": 0
  },
  "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
  "last_reasoning_effort": "string",
  "last_turn_summary": "string",
  "mcp_server_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "owner_name": "string",
  "owner_username": "string",
  "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
  "pin_order": 0,
  "plan_mode": "plan",
  "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
  "shared": true,
  "status": "waiting",
  "title": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "warnings": [
    "string"
  ],
  "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Chat](schemas.md#codersdkchat) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
