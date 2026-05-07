# Chats

## List chats

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats`

Experimental: this endpoint is subject to change.

### Parameters

| Name    | In    | Type   | Required | Description                                                    |
|---------|-------|--------|----------|----------------------------------------------------------------|
| `q`     | query | string | false    | Search query                                                   |
| `label` | query | string | false    | Filter by label as key:value. Repeat for multiple (AND logic). |

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
    "last_injected_context": [
      {
        "args": [
          0
        ],
        "args_delta": "string",
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
        "provider_executed": true,
        "provider_metadata": [
          0
        ],
        "result": [
          0
        ],
        "result_delta": "string",
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
    "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
    "last_turn_summary": "string",
    "mcp_server_ids": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
    "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
    "pin_order": 0,
    "plan_mode": "plan",
    "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
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

| Name                              | Type                                                                   | Required | Restrictions | Description                                                                                                                                                                                                                                                                |
|-----------------------------------|------------------------------------------------------------------------|----------|--------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `[array item]`                    | array                                                                  | false    |              |                                                                                                                                                                                                                                                                            |
| `» agent_id`                      | string(uuid)                                                           | false    |              |                                                                                                                                                                                                                                                                            |
| `» archived`                      | boolean                                                                | false    |              |                                                                                                                                                                                                                                                                            |
| `» build_id`                      | string(uuid)                                                           | false    |              |                                                                                                                                                                                                                                                                            |
| `» children`                      | [codersdk.Chat](schemas.md#codersdkchat)                               | false    |              | Children holds child (subagent) chats nested under this root chat. Always initialized to an empty slice so the JSON field is present as []. Child chats cannot create their own subagents, so nesting depth is capped at 1 and this slice is always empty for child chats. |
| `» client_type`                   | [codersdk.ChatClientType](schemas.md#codersdkchatclienttype)           | false    |              |                                                                                                                                                                                                                                                                            |
| `» created_at`                    | string(date-time)                                                      | false    |              |                                                                                                                                                                                                                                                                            |
| `» diff_status`                   | [codersdk.ChatDiffStatus](schemas.md#codersdkchatdiffstatus)           | false    |              |                                                                                                                                                                                                                                                                            |
| `»» additions`                    | integer                                                                | false    |              |                                                                                                                                                                                                                                                                            |
| `»» approved`                     | boolean                                                                | false    |              |                                                                                                                                                                                                                                                                            |
| `»» author_avatar_url`            | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»» author_login`                 | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»» base_branch`                  | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»» changed_files`                | integer                                                                | false    |              |                                                                                                                                                                                                                                                                            |
| `»» changes_requested`            | boolean                                                                | false    |              |                                                                                                                                                                                                                                                                            |
| `»» chat_id`                      | string(uuid)                                                           | false    |              |                                                                                                                                                                                                                                                                            |
| `»» commits`                      | integer                                                                | false    |              |                                                                                                                                                                                                                                                                            |
| `»» deletions`                    | integer                                                                | false    |              |                                                                                                                                                                                                                                                                            |
| `»» head_branch`                  | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»» pr_number`                    | integer                                                                | false    |              |                                                                                                                                                                                                                                                                            |
| `»» pull_request_draft`           | boolean                                                                | false    |              |                                                                                                                                                                                                                                                                            |
| `»» pull_request_state`           | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»» pull_request_title`           | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»» refreshed_at`                 | string(date-time)                                                      | false    |              |                                                                                                                                                                                                                                                                            |
| `»» reviewer_count`               | integer                                                                | false    |              |                                                                                                                                                                                                                                                                            |
| `»» stale_at`                     | string(date-time)                                                      | false    |              |                                                                                                                                                                                                                                                                            |
| `»» url`                          | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `» files`                         | array                                                                  | false    |              |                                                                                                                                                                                                                                                                            |
| `»» created_at`                   | string(date-time)                                                      | false    |              |                                                                                                                                                                                                                                                                            |
| `»» id`                           | string(uuid)                                                           | false    |              |                                                                                                                                                                                                                                                                            |
| `»» mime_type`                    | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»» name`                         | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»» organization_id`              | string(uuid)                                                           | false    |              |                                                                                                                                                                                                                                                                            |
| `»» owner_id`                     | string(uuid)                                                           | false    |              |                                                                                                                                                                                                                                                                            |
| `» has_unread`                    | boolean                                                                | false    |              | Has unread is true when assistant messages exist beyond the owner's read cursor, which updates on stream connect and disconnect.                                                                                                                                           |
| `» id`                            | string(uuid)                                                           | false    |              |                                                                                                                                                                                                                                                                            |
| `» labels`                        | object                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»» [any property]`               | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `» last_error`                    | [codersdk.ChatError](schemas.md#codersdkchaterror)                     | false    |              |                                                                                                                                                                                                                                                                            |
| `»» detail`                       | string                                                                 | false    |              | Detail is optional provider-specific context shown alongside the normalized error message when available.                                                                                                                                                                  |
| `»» kind`                         | [codersdk.ChatErrorKind](schemas.md#codersdkchaterrorkind)             | false    |              | Kind classifies the error for consistent client rendering.                                                                                                                                                                                                                 |
| `»» message`                      | string                                                                 | false    |              | Message is the normalized, user-facing error message.                                                                                                                                                                                                                      |
| `»» provider`                     | string                                                                 | false    |              | Provider identifies the upstream model provider when known.                                                                                                                                                                                                                |
| `»» retryable`                    | boolean                                                                | false    |              | Retryable reports whether the underlying error is transient.                                                                                                                                                                                                               |
| `»» status_code`                  | integer                                                                | false    |              | Status code is the best-effort upstream HTTP status code.                                                                                                                                                                                                                  |
| `» last_injected_context`         | array                                                                  | false    |              | Last injected context holds the most recently persisted injected context parts (AGENTS.md files and skills). It is updated only when context changes, on first workspace attach or agent change.                                                                           |
| `»» args`                         | array                                                                  | false    |              |                                                                                                                                                                                                                                                                            |
| `»» args_delta`                   | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»» content`                      | string                                                                 | false    |              | The code content from the diff that was commented on.                                                                                                                                                                                                                      |
| `»» context_file_agent_id`        | [uuid.NullUUID](schemas.md#uuidnulluuid)                               | false    |              | Context file agent ID is the workspace agent that provided this context file. Used to detect when the agent changes (e.g. workspace rebuilt) so instruction files can be re-persisted with fresh content.                                                                  |
| `»»» uuid`                        | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»»» valid`                       | boolean                                                                | false    |              | Valid is true if UUID is not NULL                                                                                                                                                                                                                                          |
| `»» context_file_content`         | string                                                                 | false    |              | Context file content holds the file content sent to the LLM. Internal only: stripped before API responses to keep payloads small. The backend reads it when building the prompt via partsToMessageParts.                                                                   |
| `»» context_file_directory`       | string                                                                 | false    |              | Context file directory is the working directory of the workspace agent. Internal only: same purpose as ContextFileOS.                                                                                                                                                      |
| `»» context_file_os`              | string                                                                 | false    |              | Context file os is the operating system of the workspace agent. Internal only: used during prompt expansion so the LLM knows the OS even on turns where InsertSystem is not called.                                                                                        |
| `»» context_file_path`            | string                                                                 | false    |              | Context file path is the absolute path of a file loaded into the LLM context (e.g. an AGENTS.md instruction file).                                                                                                                                                         |
| `»» context_file_skill_meta_file` | string                                                                 | false    |              | Context file skill meta file is the basename of the skill meta file (e.g. "SKILL.md") at the time of persistence. Internal only: restored on subsequent turns so the read_skill tool uses the correct filename even when the agent configured a non-default value.         |
| `»» context_file_truncated`       | boolean                                                                | false    |              | Context file truncated indicates the file exceeded the 64KiB instruction file limit and was truncated.                                                                                                                                                                     |
| `»» created_at`                   | string(date-time)                                                      | false    |              | Created at records when this part was produced. Present on tool-call and tool-result parts so the frontend can compute tool execution duration.                                                                                                                            |
| `»» data`                         | array                                                                  | false    |              |                                                                                                                                                                                                                                                                            |
| `»» end_line`                     | integer                                                                | false    |              |                                                                                                                                                                                                                                                                            |
| `»» file_id`                      | [uuid.NullUUID](schemas.md#uuidnulluuid)                               | false    |              |                                                                                                                                                                                                                                                                            |
| `»»» uuid`                        | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»»» valid`                       | boolean                                                                | false    |              | Valid is true if UUID is not NULL                                                                                                                                                                                                                                          |
| `»» file_name`                    | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»» is_error`                     | boolean                                                                | false    |              |                                                                                                                                                                                                                                                                            |
| `»» is_media`                     | boolean                                                                | false    |              |                                                                                                                                                                                                                                                                            |
| `»» mcp_server_config_id`         | [uuid.NullUUID](schemas.md#uuidnulluuid)                               | false    |              |                                                                                                                                                                                                                                                                            |
| `»»» uuid`                        | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»»» valid`                       | boolean                                                                | false    |              | Valid is true if UUID is not NULL                                                                                                                                                                                                                                          |
| `»» media_type`                   | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»» name`                         | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»» provider_executed`            | boolean                                                                | false    |              | Provider executed indicates the tool call was executed by the provider (e.g. Anthropic computer use).                                                                                                                                                                      |
| `»» provider_metadata`            | array                                                                  | false    |              | Provider metadata holds provider-specific response metadata (e.g. Anthropic cache control hints) as raw JSON. Internal only: stripped by db2sdk before API responses.                                                                                                      |
| `»» result`                       | array                                                                  | false    |              |                                                                                                                                                                                                                                                                            |
| `»» result_delta`                 | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»» signature`                    | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»» skill_description`            | string                                                                 | false    |              | Skill description is the short description from the skill's SKILL.md frontmatter.                                                                                                                                                                                          |
| `»» skill_dir`                    | string                                                                 | false    |              | Skill dir is the absolute path to the skill directory inside the workspace filesystem. Internal only: used by read_skill/read_skill_file tools to locate skill files.                                                                                                      |
| `»» skill_name`                   | string                                                                 | false    |              | Skill name is the kebab-case name of a discovered skill from the workspace's .agents/skills/ directory.                                                                                                                                                                    |
| `»» source_id`                    | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»» start_line`                   | integer                                                                | false    |              |                                                                                                                                                                                                                                                                            |
| `»» text`                         | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»» title`                        | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»» tool_call_id`                 | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»» tool_name`                    | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `»» type`                         | [codersdk.ChatMessagePartType](schemas.md#codersdkchatmessageparttype) | false    |              |                                                                                                                                                                                                                                                                            |
| `»» url`                          | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `» last_model_config_id`          | string(uuid)                                                           | false    |              |                                                                                                                                                                                                                                                                            |
| `» last_turn_summary`             | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `» mcp_server_ids`                | array                                                                  | false    |              |                                                                                                                                                                                                                                                                            |
| `» organization_id`               | string(uuid)                                                           | false    |              |                                                                                                                                                                                                                                                                            |
| `» owner_id`                      | string(uuid)                                                           | false    |              |                                                                                                                                                                                                                                                                            |
| `» parent_chat_id`                | string(uuid)                                                           | false    |              |                                                                                                                                                                                                                                                                            |
| `» pin_order`                     | integer                                                                | false    |              |                                                                                                                                                                                                                                                                            |
| `» plan_mode`                     | [codersdk.ChatPlanMode](schemas.md#codersdkchatplanmode)               | false    |              |                                                                                                                                                                                                                                                                            |
| `» root_chat_id`                  | string(uuid)                                                           | false    |              |                                                                                                                                                                                                                                                                            |
| `» status`                        | [codersdk.ChatStatus](schemas.md#codersdkchatstatus)                   | false    |              |                                                                                                                                                                                                                                                                            |
| `» title`                         | string                                                                 | false    |              |                                                                                                                                                                                                                                                                            |
| `» updated_at`                    | string(date-time)                                                      | false    |              |                                                                                                                                                                                                                                                                            |
| `» warnings`                      | array                                                                  | false    |              |                                                                                                                                                                                                                                                                            |
| `» workspace_id`                  | string(uuid)                                                           | false    |              |                                                                                                                                                                                                                                                                            |

#### Enumerated Values

| Property      | Value(s)                                                                                                     |
|---------------|--------------------------------------------------------------------------------------------------------------|
| `client_type` | `api`, `ui`                                                                                                  |
| `kind`        | `auth`, `config`, `generic`, `overloaded`, `rate_limit`, `startup_timeout`, `timeout`, `usage_limit`         |
| `type`        | `context-file`, `file`, `file-reference`, `reasoning`, `skill`, `source`, `text`, `tool-call`, `tool-result` |
| `plan_mode`   | `plan`                                                                                                       |
| `status`      | `completed`, `error`, `paused`, `pending`, `requires_action`, `running`, `waiting`                           |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create chat

### Code samples

```shell
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
      "last_injected_context": [
        {
          "args": [
            0
          ],
          "args_delta": "string",
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
          "provider_executed": true,
          "provider_metadata": [
            0
          ],
          "result": [
            0
          ],
          "result_delta": "string",
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
      "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
      "last_turn_summary": "string",
      "mcp_server_ids": [
        "497f6eca-6276-4993-bfeb-53cbbbba6f08"
      ],
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
      "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
      "pin_order": 0,
      "plan_mode": "plan",
      "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
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
  "last_injected_context": [
    {
      "args": [
        0
      ],
      "args_delta": "string",
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
      "provider_executed": true,
      "provider_metadata": [
        0
      ],
      "result": [
        0
      ],
      "result_delta": "string",
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
  "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
  "last_turn_summary": "string",
  "mcp_server_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
  "pin_order": 0,
  "plan_mode": "plan",
  "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
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

## Get chat advisor config

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/config/advisor \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/config/advisor`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
{
  "enabled": true,
  "max_output_tokens": 0,
  "max_uses_per_run": 0,
  "model_config_id": "f5fb4d91-62ca-4377-9ee6-5d43ba00d205"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                     |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AdvisorConfig](schemas.md#codersdkadvisorconfig) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update chat advisor config

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/experimental/chats/config/advisor \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /api/experimental/chats/config/advisor`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "enabled": true,
  "max_output_tokens": 0,
  "max_uses_per_run": 0,
  "model_config_id": "f5fb4d91-62ca-4377-9ee6-5d43ba00d205"
}
```

### Parameters

| Name   | In   | Type                                                                                 | Required | Description  |
|--------|------|--------------------------------------------------------------------------------------|----------|--------------|
| `body` | body | [codersdk.UpdateAdvisorConfigRequest](schemas.md#codersdkupdateadvisorconfigrequest) | true     | Request body |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get chat auto archive days

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/config/auto-archive-days \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/config/auto-archive-days`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
{
  "auto_archive_days": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                 |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatAutoArchiveDaysResponse](schemas.md#codersdkchatautoarchivedaysresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update chat auto archive days

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/experimental/chats/config/auto-archive-days \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /api/experimental/chats/config/auto-archive-days`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "auto_archive_days": 0
}
```

### Parameters

| Name   | In   | Type                                                                                             | Required | Description  |
|--------|------|--------------------------------------------------------------------------------------------------|----------|--------------|
| `body` | body | [codersdk.UpdateChatAutoArchiveDaysRequest](schemas.md#codersdkupdatechatautoarchivedaysrequest) | true     | Request body |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get chat computer use provider

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/config/computer-use-provider \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/config/computer-use-provider`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
{
  "provider": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                         |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatComputerUseProviderResponse](schemas.md#codersdkchatcomputeruseproviderresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update chat computer use provider

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/experimental/chats/config/computer-use-provider \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /api/experimental/chats/config/computer-use-provider`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "provider": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                                     | Required | Description  |
|--------|------|----------------------------------------------------------------------------------------------------------|----------|--------------|
| `body` | body | [codersdk.UpdateChatComputerUseProviderRequest](schemas.md#codersdkupdatechatcomputeruseproviderrequest) | true     | Request body |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get chat debug logging

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/config/debug-logging \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/config/debug-logging`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
{
  "allow_users": true,
  "forced_by_deployment": true
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                     |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatDebugLoggingAdminSettings](schemas.md#codersdkchatdebugloggingadminsettings) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update chat debug logging

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/experimental/chats/config/debug-logging \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /api/experimental/chats/config/debug-logging`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "allow_users": true
}
```

### Parameters

| Name   | In   | Type                                                                                                           | Required | Description  |
|--------|------|----------------------------------------------------------------------------------------------------------------|----------|--------------|
| `body` | body | [codersdk.UpdateChatDebugLoggingAllowUsersRequest](schemas.md#codersdkupdatechatdebugloggingallowusersrequest) | true     | Request body |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get chat debug retention days

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/config/debug-retention-days \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/config/debug-retention-days`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
{
  "debug_retention_days": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                       |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatDebugRetentionDaysResponse](schemas.md#codersdkchatdebugretentiondaysresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update chat debug retention days

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/experimental/chats/config/debug-retention-days \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /api/experimental/chats/config/debug-retention-days`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "debug_retention_days": 0
}
```

### Parameters

| Name   | In   | Type                                                                                                   | Required | Description  |
|--------|------|--------------------------------------------------------------------------------------------------------|----------|--------------|
| `body` | body | [codersdk.UpdateChatDebugRetentionDaysRequest](schemas.md#codersdkupdatechatdebugretentiondaysrequest) | true     | Request body |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get chat desktop enabled

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/config/desktop-enabled \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/config/desktop-enabled`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
{
  "enable_desktop": true
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                               |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatDesktopEnabledResponse](schemas.md#codersdkchatdesktopenabledresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update chat desktop enabled

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/experimental/chats/config/desktop-enabled \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /api/experimental/chats/config/desktop-enabled`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "enable_desktop": true
}
```

### Parameters

| Name   | In   | Type                                                                                           | Required | Description  |
|--------|------|------------------------------------------------------------------------------------------------|----------|--------------|
| `body` | body | [codersdk.UpdateChatDesktopEnabledRequest](schemas.md#codersdkupdatechatdesktopenabledrequest) | true     | Request body |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get chat model override

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/config/model-override/{context} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/config/model-override/{context}`

Experimental: this endpoint is subject to change.

### Parameters

| Name      | In   | Type   | Required | Description      |
|-----------|------|--------|----------|------------------|
| `context` | path | string | true     | Override context |

### Example responses

> 200 Response

```json
{
  "context": "general",
  "is_malformed": true,
  "model_config_id": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                             |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatModelOverrideResponse](schemas.md#codersdkchatmodeloverrideresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update chat model override

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/experimental/chats/config/model-override/{context} \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /api/experimental/chats/config/model-override/{context}`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "model_config_id": "string"
}
```

### Parameters

| Name      | In   | Type                                                                                         | Required | Description      |
|-----------|------|----------------------------------------------------------------------------------------------|----------|------------------|
| `context` | path | string                                                                                       | true     | Override context |
| `body`    | body | [codersdk.UpdateChatModelOverrideRequest](schemas.md#codersdkupdatechatmodeloverriderequest) | true     | Request body     |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get chat personal model overrides admin settings

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/config/personal-model-overrides \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/config/personal-model-overrides`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
{
  "allow_users": true
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                                         |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatPersonalModelOverridesAdminSettings](schemas.md#codersdkchatpersonalmodeloverridesadminsettings) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update chat personal model overrides admin settings

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/experimental/chats/config/personal-model-overrides \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /api/experimental/chats/config/personal-model-overrides`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "allow_users": true
}
```

### Parameters

| Name   | In   | Type                                                                                                                                     | Required | Description  |
|--------|------|------------------------------------------------------------------------------------------------------------------------------------------|----------|--------------|
| `body` | body | [codersdk.UpdateChatPersonalModelOverridesAdminSettingsRequest](schemas.md#codersdkupdatechatpersonalmodeloverridesadminsettingsrequest) | true     | Request body |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get chat plan mode instructions

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/config/plan-mode-instructions \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/config/plan-mode-instructions`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
{
  "plan_mode_instructions": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                           |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatPlanModeInstructionsResponse](schemas.md#codersdkchatplanmodeinstructionsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update chat plan mode instructions

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/experimental/chats/config/plan-mode-instructions \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /api/experimental/chats/config/plan-mode-instructions`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "plan_mode_instructions": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                                       | Required | Description  |
|--------|------|------------------------------------------------------------------------------------------------------------|----------|--------------|
| `body` | body | [codersdk.UpdateChatPlanModeInstructionsRequest](schemas.md#codersdkupdatechatplanmodeinstructionsrequest) | true     | Request body |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get chat system prompt

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/config/system-prompt \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/config/system-prompt`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
{
  "default_system_prompt": "string",
  "include_default_system_prompt": true,
  "system_prompt": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                           |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatSystemPromptResponse](schemas.md#codersdkchatsystempromptresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update chat system prompt

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/experimental/chats/config/system-prompt \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /api/experimental/chats/config/system-prompt`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "include_default_system_prompt": true,
  "system_prompt": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                       | Required | Description  |
|--------|------|--------------------------------------------------------------------------------------------|----------|--------------|
| `body` | body | [codersdk.UpdateChatSystemPromptRequest](schemas.md#codersdkupdatechatsystempromptrequest) | true     | Request body |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get chat template allowlist

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/config/template-allowlist \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/config/template-allowlist`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
{
  "template_ids": [
    "string"
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                     |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatTemplateAllowlist](schemas.md#codersdkchattemplateallowlist) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update chat template allowlist

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/experimental/chats/config/template-allowlist \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /api/experimental/chats/config/template-allowlist`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "template_ids": [
    "string"
  ]
}
```

### Parameters

| Name   | In   | Type                                                                       | Required | Description  |
|--------|------|----------------------------------------------------------------------------|----------|--------------|
| `body` | body | [codersdk.ChatTemplateAllowlist](schemas.md#codersdkchattemplateallowlist) | true     | Request body |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get user chat debug logging

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/config/user-debug-logging \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/config/user-debug-logging`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
{
  "debug_logging_enabled": true,
  "forced_by_deployment": true,
  "user_toggle_allowed": true
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.UserChatDebugLoggingSettings](schemas.md#codersdkuserchatdebugloggingsettings) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update user chat debug logging

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/experimental/chats/config/user-debug-logging \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /api/experimental/chats/config/user-debug-logging`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "debug_logging_enabled": true
}
```

### Parameters

| Name   | In   | Type                                                                                               | Required | Description  |
|--------|------|----------------------------------------------------------------------------------------------------|----------|--------------|
| `body` | body | [codersdk.UpdateUserChatDebugLoggingRequest](schemas.md#codersdkupdateuserchatdebugloggingrequest) | true     | Request body |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get user chat personal model overrides

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/config/user-personal-model-overrides \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/config/user-personal-model-overrides`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
{
  "deployment_defaults": {
    "explore": {
      "context": "general",
      "is_malformed": true,
      "model_config_id": "string"
    },
    "general": {
      "context": "general",
      "is_malformed": true,
      "model_config_id": "string"
    }
  },
  "enabled": true,
  "explore": {
    "context": "root",
    "is_malformed": true,
    "is_set": true,
    "mode": "deployment_default",
    "model_config_id": "string"
  },
  "general": {
    "context": "root",
    "is_malformed": true,
    "is_set": true,
    "mode": "deployment_default",
    "model_config_id": "string"
  },
  "root": {
    "context": "root",
    "is_malformed": true,
    "is_set": true,
    "mode": "deployment_default",
    "model_config_id": "string"
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                                       |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.UserChatPersonalModelOverridesResponse](schemas.md#codersdkuserchatpersonalmodeloverridesresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update user chat personal model override

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/experimental/chats/config/user-personal-model-overrides/{context} \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /api/experimental/chats/config/user-personal-model-overrides/{context}`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "mode": "deployment_default",
  "model_config_id": "string"
}
```

### Parameters

| Name      | In   | Type                                                                                                                 | Required | Description      |
|-----------|------|----------------------------------------------------------------------------------------------------------------------|----------|------------------|
| `context` | path | string                                                                                                               | true     | Override context |
| `body`    | body | [codersdk.UpdateUserChatPersonalModelOverrideRequest](schemas.md#codersdkupdateuserchatpersonalmodeloverriderequest) | true     | Request body     |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get user chat custom prompt

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/config/user-prompt \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/config/user-prompt`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
{
  "custom_prompt": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                   |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.UserChatCustomPrompt](schemas.md#codersdkuserchatcustomprompt) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update user chat custom prompt

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/experimental/chats/config/user-prompt \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /api/experimental/chats/config/user-prompt`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "custom_prompt": "string"
}
```

### Parameters

| Name   | In   | Type                                                                     | Required | Description  |
|--------|------|--------------------------------------------------------------------------|----------|--------------|
| `body` | body | [codersdk.UserChatCustomPrompt](schemas.md#codersdkuserchatcustomprompt) | true     | Request body |

### Example responses

> 200 Response

```json
{
  "custom_prompt": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                   |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.UserChatCustomPrompt](schemas.md#codersdkuserchatcustomprompt) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get chat workspace TTL

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/config/workspace-ttl \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/config/workspace-ttl`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
{
  "workspace_ttl_ms": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                           |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatWorkspaceTTLResponse](schemas.md#codersdkchatworkspacettlresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update chat workspace TTL

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/experimental/chats/config/workspace-ttl \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /api/experimental/chats/config/workspace-ttl`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "workspace_ttl_ms": 0
}
```

### Parameters

| Name   | In   | Type                                                                                       | Required | Description  |
|--------|------|--------------------------------------------------------------------------------------------|----------|--------------|
| `body` | body | [codersdk.UpdateChatWorkspaceTTLRequest](schemas.md#codersdkupdatechatworkspacettlrequest) | true     | Request body |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Upload chat file

### Code samples

```shell
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

```shell
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

## List chat model configs

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/model-configs \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/model-configs`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
[
  {
    "compression_threshold": 0,
    "context_limit": 0,
    "created_at": "2019-08-24T14:15:22Z",
    "display_name": "string",
    "enabled": true,
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "is_default": true,
    "model": "string",
    "model_config": {
      "cost": {
        "cache_read_price_per_million_tokens": 0,
        "cache_write_price_per_million_tokens": 0,
        "input_price_per_million_tokens": 0,
        "output_price_per_million_tokens": 0
      },
      "frequency_penalty": 0,
      "max_output_tokens": 0,
      "presence_penalty": 0,
      "provider_options": {
        "anthropic": {
          "allowed_domains": [
            "string"
          ],
          "blocked_domains": [
            "string"
          ],
          "disable_parallel_tool_use": true,
          "effort": "string",
          "send_reasoning": true,
          "thinking": {
            "budget_tokens": 0
          },
          "web_search_enabled": true
        },
        "google": {
          "cached_content": "string",
          "safety_settings": [
            {
              "category": "string",
              "threshold": "string"
            }
          ],
          "thinking_config": {
            "include_thoughts": true,
            "thinking_budget": 0
          },
          "threshold": "string",
          "web_search_enabled": true
        },
        "openai": {
          "allowed_domains": [
            "string"
          ],
          "include": [
            "string"
          ],
          "instructions": "string",
          "log_probs": true,
          "logit_bias": {
            "property1": 0,
            "property2": 0
          },
          "max_completion_tokens": 0,
          "max_tool_calls": 0,
          "metadata": {
            "property1": null,
            "property2": null
          },
          "parallel_tool_calls": true,
          "prediction": {
            "property1": null,
            "property2": null
          },
          "prompt_cache_key": "string",
          "reasoning_effort": "string",
          "reasoning_summary": "string",
          "safety_identifier": "string",
          "search_context_size": "string",
          "service_tier": "string",
          "store": true,
          "strict_json_schema": true,
          "structured_outputs": true,
          "text_verbosity": "string",
          "top_log_probs": 0,
          "user": "string",
          "web_search_enabled": true
        },
        "openaicompat": {
          "reasoning_effort": "string",
          "user": "string"
        },
        "openrouter": {
          "extra_body": {
            "property1": null,
            "property2": null
          },
          "include_usage": true,
          "log_probs": true,
          "logit_bias": {
            "property1": 0,
            "property2": 0
          },
          "parallel_tool_calls": true,
          "provider": {
            "allow_fallbacks": true,
            "data_collection": "string",
            "ignore": [
              "string"
            ],
            "only": [
              "string"
            ],
            "order": [
              "string"
            ],
            "quantizations": [
              "string"
            ],
            "require_parameters": true,
            "sort": "string"
          },
          "reasoning": {
            "effort": "string",
            "enabled": true,
            "exclude": true,
            "max_tokens": 0
          },
          "user": "string"
        },
        "vercel": {
          "extra_body": {
            "property1": null,
            "property2": null
          },
          "logit_bias": {
            "property1": 0,
            "property2": 0
          },
          "logprobs": true,
          "parallel_tool_calls": true,
          "providerOptions": {
            "models": [
              "string"
            ],
            "order": [
              "string"
            ]
          },
          "reasoning": {
            "effort": "string",
            "enabled": true,
            "exclude": true,
            "max_tokens": 0
          },
          "top_logprobs": 0,
          "user": "string"
        }
      },
      "temperature": 0,
      "top_k": 0,
      "top_p": 0
    },
    "provider": "string",
    "updated_at": "2019-08-24T14:15:22Z"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                  |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.ChatModelConfig](schemas.md#codersdkchatmodelconfig) |

<h3 id="list-chat-model-configs-responseschema">Response Schema</h3>

Status Code **200**

| Name                                       | Type                                                                                                       | Required | Restrictions | Description |
|--------------------------------------------|------------------------------------------------------------------------------------------------------------|----------|--------------|-------------|
| `[array item]`                             | array                                                                                                      | false    |              |             |
| `» compression_threshold`                  | integer                                                                                                    | false    |              |             |
| `» context_limit`                          | integer                                                                                                    | false    |              |             |
| `» created_at`                             | string(date-time)                                                                                          | false    |              |             |
| `» display_name`                           | string                                                                                                     | false    |              |             |
| `» enabled`                                | boolean                                                                                                    | false    |              |             |
| `» id`                                     | string(uuid)                                                                                               | false    |              |             |
| `» is_default`                             | boolean                                                                                                    | false    |              |             |
| `» model`                                  | string                                                                                                     | false    |              |             |
| `» model_config`                           | [codersdk.ChatModelCallConfig](schemas.md#codersdkchatmodelcallconfig)                                     | false    |              |             |
| `»» cost`                                  | [codersdk.ModelCostConfig](schemas.md#codersdkmodelcostconfig)                                             | false    |              |             |
| `»»» cache_read_price_per_million_tokens`  | number                                                                                                     | false    |              |             |
| `»»» cache_write_price_per_million_tokens` | number                                                                                                     | false    |              |             |
| `»»» input_price_per_million_tokens`       | number                                                                                                     | false    |              |             |
| `»»» output_price_per_million_tokens`      | number                                                                                                     | false    |              |             |
| `»» frequency_penalty`                     | number                                                                                                     | false    |              |             |
| `»» max_output_tokens`                     | integer                                                                                                    | false    |              |             |
| `»» presence_penalty`                      | number                                                                                                     | false    |              |             |
| `»» provider_options`                      | [codersdk.ChatModelProviderOptions](schemas.md#codersdkchatmodelprovideroptions)                           | false    |              |             |
| `»»» anthropic`                            | [codersdk.ChatModelAnthropicProviderOptions](schemas.md#codersdkchatmodelanthropicprovideroptions)         | false    |              |             |
| `»»»» allowed_domains`                     | array                                                                                                      | false    |              |             |
| `»»»» blocked_domains`                     | array                                                                                                      | false    |              |             |
| `»»»» disable_parallel_tool_use`           | boolean                                                                                                    | false    |              |             |
| `»»»» effort`                              | string                                                                                                     | false    |              |             |
| `»»»» send_reasoning`                      | boolean                                                                                                    | false    |              |             |
| `»»»» thinking`                            | [codersdk.ChatModelAnthropicThinkingOptions](schemas.md#codersdkchatmodelanthropicthinkingoptions)         | false    |              |             |
| `»»»»» budget_tokens`                      | integer                                                                                                    | false    |              |             |
| `»»»» web_search_enabled`                  | boolean                                                                                                    | false    |              |             |
| `»»» google`                               | [codersdk.ChatModelGoogleProviderOptions](schemas.md#codersdkchatmodelgoogleprovideroptions)               | false    |              |             |
| `»»»» cached_content`                      | string                                                                                                     | false    |              |             |
| `»»»» safety_settings`                     | array                                                                                                      | false    |              |             |
| `»»»»» category`                           | string                                                                                                     | false    |              |             |
| `»»»»» threshold`                          | string                                                                                                     | false    |              |             |
| `»»»» thinking_config`                     | [codersdk.ChatModelGoogleThinkingConfig](schemas.md#codersdkchatmodelgooglethinkingconfig)                 | false    |              |             |
| `»»»»» include_thoughts`                   | boolean                                                                                                    | false    |              |             |
| `»»»»» thinking_budget`                    | integer                                                                                                    | false    |              |             |
| `»»»» threshold`                           | string                                                                                                     | false    |              |             |
| `»»»» web_search_enabled`                  | boolean                                                                                                    | false    |              |             |
| `»»» openai`                               | [codersdk.ChatModelOpenAIProviderOptions](schemas.md#codersdkchatmodelopenaiprovideroptions)               | false    |              |             |
| `»»»» allowed_domains`                     | array                                                                                                      | false    |              |             |
| `»»»» include`                             | array                                                                                                      | false    |              |             |
| `»»»» instructions`                        | string                                                                                                     | false    |              |             |
| `»»»» log_probs`                           | boolean                                                                                                    | false    |              |             |
| `»»»» logit_bias`                          | object                                                                                                     | false    |              |             |
| `»»»»» [any property]`                     | integer(int64)                                                                                             | false    |              |             |
| `»»»» max_completion_tokens`               | integer                                                                                                    | false    |              |             |
| `»»»» max_tool_calls`                      | integer                                                                                                    | false    |              |             |
| `»»»» metadata`                            | object                                                                                                     | false    |              |             |
| `»»»»» [any property]`                     | any                                                                                                        | false    |              |             |
| `»»»» parallel_tool_calls`                 | boolean                                                                                                    | false    |              |             |
| `»»»» prediction`                          | object                                                                                                     | false    |              |             |
| `»»»»» [any property]`                     | any                                                                                                        | false    |              |             |
| `»»»» prompt_cache_key`                    | string                                                                                                     | false    |              |             |
| `»»»» reasoning_effort`                    | string                                                                                                     | false    |              |             |
| `»»»» reasoning_summary`                   | string                                                                                                     | false    |              |             |
| `»»»» safety_identifier`                   | string                                                                                                     | false    |              |             |
| `»»»» search_context_size`                 | string                                                                                                     | false    |              |             |
| `»»»» service_tier`                        | string                                                                                                     | false    |              |             |
| `»»»» store`                               | boolean                                                                                                    | false    |              |             |
| `»»»» strict_json_schema`                  | boolean                                                                                                    | false    |              |             |
| `»»»» structured_outputs`                  | boolean                                                                                                    | false    |              |             |
| `»»»» text_verbosity`                      | string                                                                                                     | false    |              |             |
| `»»»» top_log_probs`                       | integer                                                                                                    | false    |              |             |
| `»»»» user`                                | string                                                                                                     | false    |              |             |
| `»»»» web_search_enabled`                  | boolean                                                                                                    | false    |              |             |
| `»»» openaicompat`                         | [codersdk.ChatModelOpenAICompatProviderOptions](schemas.md#codersdkchatmodelopenaicompatprovideroptions)   | false    |              |             |
| `»»»» reasoning_effort`                    | string                                                                                                     | false    |              |             |
| `»»»» user`                                | string                                                                                                     | false    |              |             |
| `»»» openrouter`                           | [codersdk.ChatModelOpenRouterProviderOptions](schemas.md#codersdkchatmodelopenrouterprovideroptions)       | false    |              |             |
| `»»»» extra_body`                          | object                                                                                                     | false    |              |             |
| `»»»»» [any property]`                     | any                                                                                                        | false    |              |             |
| `»»»» include_usage`                       | boolean                                                                                                    | false    |              |             |
| `»»»» log_probs`                           | boolean                                                                                                    | false    |              |             |
| `»»»» logit_bias`                          | object                                                                                                     | false    |              |             |
| `»»»»» [any property]`                     | integer(int64)                                                                                             | false    |              |             |
| `»»»» parallel_tool_calls`                 | boolean                                                                                                    | false    |              |             |
| `»»»» provider`                            | [codersdk.ChatModelOpenRouterProvider](schemas.md#codersdkchatmodelopenrouterprovider)                     | false    |              |             |
| `»»»»» allow_fallbacks`                    | boolean                                                                                                    | false    |              |             |
| `»»»»» data_collection`                    | string                                                                                                     | false    |              |             |
| `»»»»» ignore`                             | array                                                                                                      | false    |              |             |
| `»»»»» only`                               | array                                                                                                      | false    |              |             |
| `»»»»» order`                              | array                                                                                                      | false    |              |             |
| `»»»»» quantizations`                      | array                                                                                                      | false    |              |             |
| `»»»»» require_parameters`                 | boolean                                                                                                    | false    |              |             |
| `»»»»» sort`                               | string                                                                                                     | false    |              |             |
| `»»»» reasoning`                           | [codersdk.ChatModelReasoningOptions](schemas.md#codersdkchatmodelreasoningoptions)                         | false    |              |             |
| `»»»»» effort`                             | string                                                                                                     | false    |              |             |
| `»»»»» enabled`                            | boolean                                                                                                    | false    |              |             |
| `»»»»» exclude`                            | boolean                                                                                                    | false    |              |             |
| `»»»»» max_tokens`                         | integer                                                                                                    | false    |              |             |
| `»»»» user`                                | string                                                                                                     | false    |              |             |
| `»»» vercel`                               | [codersdk.ChatModelVercelProviderOptions](schemas.md#codersdkchatmodelvercelprovideroptions)               | false    |              |             |
| `»»»» extra_body`                          | object                                                                                                     | false    |              |             |
| `»»»»» [any property]`                     | any                                                                                                        | false    |              |             |
| `»»»» logit_bias`                          | object                                                                                                     | false    |              |             |
| `»»»»» [any property]`                     | integer(int64)                                                                                             | false    |              |             |
| `»»»» logprobs`                            | boolean                                                                                                    | false    |              |             |
| `»»»» parallel_tool_calls`                 | boolean                                                                                                    | false    |              |             |
| `»»»» providerOptions`                     | [codersdk.ChatModelVercelGatewayProviderOptions](schemas.md#codersdkchatmodelvercelgatewayprovideroptions) | false    |              |             |
| `»»»»» models`                             | array                                                                                                      | false    |              |             |
| `»»»»» order`                              | array                                                                                                      | false    |              |             |
| `»»»» reasoning`                           | [codersdk.ChatModelReasoningOptions](schemas.md#codersdkchatmodelreasoningoptions)                         | false    |              |             |
| `»»»» top_logprobs`                        | integer                                                                                                    | false    |              |             |
| `»»»» user`                                | string                                                                                                     | false    |              |             |
| `»» temperature`                           | number                                                                                                     | false    |              |             |
| `»» top_k`                                 | integer                                                                                                    | false    |              |             |
| `»» top_p`                                 | number                                                                                                     | false    |              |             |
| `» provider`                               | string                                                                                                     | false    |              |             |
| `» updated_at`                             | string(date-time)                                                                                          | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create chat model config

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/experimental/chats/model-configs \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/experimental/chats/model-configs`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "compression_threshold": 0,
  "context_limit": 0,
  "display_name": "string",
  "enabled": true,
  "is_default": true,
  "model": "string",
  "model_config": {
    "cost": {
      "cache_read_price_per_million_tokens": 0,
      "cache_write_price_per_million_tokens": 0,
      "input_price_per_million_tokens": 0,
      "output_price_per_million_tokens": 0
    },
    "frequency_penalty": 0,
    "max_output_tokens": 0,
    "presence_penalty": 0,
    "provider_options": {
      "anthropic": {
        "allowed_domains": [
          "string"
        ],
        "blocked_domains": [
          "string"
        ],
        "disable_parallel_tool_use": true,
        "effort": "string",
        "send_reasoning": true,
        "thinking": {
          "budget_tokens": 0
        },
        "web_search_enabled": true
      },
      "google": {
        "cached_content": "string",
        "safety_settings": [
          {
            "category": "string",
            "threshold": "string"
          }
        ],
        "thinking_config": {
          "include_thoughts": true,
          "thinking_budget": 0
        },
        "threshold": "string",
        "web_search_enabled": true
      },
      "openai": {
        "allowed_domains": [
          "string"
        ],
        "include": [
          "string"
        ],
        "instructions": "string",
        "log_probs": true,
        "logit_bias": {
          "property1": 0,
          "property2": 0
        },
        "max_completion_tokens": 0,
        "max_tool_calls": 0,
        "metadata": {
          "property1": null,
          "property2": null
        },
        "parallel_tool_calls": true,
        "prediction": {
          "property1": null,
          "property2": null
        },
        "prompt_cache_key": "string",
        "reasoning_effort": "string",
        "reasoning_summary": "string",
        "safety_identifier": "string",
        "search_context_size": "string",
        "service_tier": "string",
        "store": true,
        "strict_json_schema": true,
        "structured_outputs": true,
        "text_verbosity": "string",
        "top_log_probs": 0,
        "user": "string",
        "web_search_enabled": true
      },
      "openaicompat": {
        "reasoning_effort": "string",
        "user": "string"
      },
      "openrouter": {
        "extra_body": {
          "property1": null,
          "property2": null
        },
        "include_usage": true,
        "log_probs": true,
        "logit_bias": {
          "property1": 0,
          "property2": 0
        },
        "parallel_tool_calls": true,
        "provider": {
          "allow_fallbacks": true,
          "data_collection": "string",
          "ignore": [
            "string"
          ],
          "only": [
            "string"
          ],
          "order": [
            "string"
          ],
          "quantizations": [
            "string"
          ],
          "require_parameters": true,
          "sort": "string"
        },
        "reasoning": {
          "effort": "string",
          "enabled": true,
          "exclude": true,
          "max_tokens": 0
        },
        "user": "string"
      },
      "vercel": {
        "extra_body": {
          "property1": null,
          "property2": null
        },
        "logit_bias": {
          "property1": 0,
          "property2": 0
        },
        "logprobs": true,
        "parallel_tool_calls": true,
        "providerOptions": {
          "models": [
            "string"
          ],
          "order": [
            "string"
          ]
        },
        "reasoning": {
          "effort": "string",
          "enabled": true,
          "exclude": true,
          "max_tokens": 0
        },
        "top_logprobs": 0,
        "user": "string"
      }
    },
    "temperature": 0,
    "top_k": 0,
    "top_p": 0
  },
  "provider": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                     | Required | Description  |
|--------|------|------------------------------------------------------------------------------------------|----------|--------------|
| `body` | body | [codersdk.CreateChatModelConfigRequest](schemas.md#codersdkcreatechatmodelconfigrequest) | true     | Request body |

### Example responses

> 201 Response

```json
{
  "compression_threshold": 0,
  "context_limit": 0,
  "created_at": "2019-08-24T14:15:22Z",
  "display_name": "string",
  "enabled": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "is_default": true,
  "model": "string",
  "model_config": {
    "cost": {
      "cache_read_price_per_million_tokens": 0,
      "cache_write_price_per_million_tokens": 0,
      "input_price_per_million_tokens": 0,
      "output_price_per_million_tokens": 0
    },
    "frequency_penalty": 0,
    "max_output_tokens": 0,
    "presence_penalty": 0,
    "provider_options": {
      "anthropic": {
        "allowed_domains": [
          "string"
        ],
        "blocked_domains": [
          "string"
        ],
        "disable_parallel_tool_use": true,
        "effort": "string",
        "send_reasoning": true,
        "thinking": {
          "budget_tokens": 0
        },
        "web_search_enabled": true
      },
      "google": {
        "cached_content": "string",
        "safety_settings": [
          {
            "category": "string",
            "threshold": "string"
          }
        ],
        "thinking_config": {
          "include_thoughts": true,
          "thinking_budget": 0
        },
        "threshold": "string",
        "web_search_enabled": true
      },
      "openai": {
        "allowed_domains": [
          "string"
        ],
        "include": [
          "string"
        ],
        "instructions": "string",
        "log_probs": true,
        "logit_bias": {
          "property1": 0,
          "property2": 0
        },
        "max_completion_tokens": 0,
        "max_tool_calls": 0,
        "metadata": {
          "property1": null,
          "property2": null
        },
        "parallel_tool_calls": true,
        "prediction": {
          "property1": null,
          "property2": null
        },
        "prompt_cache_key": "string",
        "reasoning_effort": "string",
        "reasoning_summary": "string",
        "safety_identifier": "string",
        "search_context_size": "string",
        "service_tier": "string",
        "store": true,
        "strict_json_schema": true,
        "structured_outputs": true,
        "text_verbosity": "string",
        "top_log_probs": 0,
        "user": "string",
        "web_search_enabled": true
      },
      "openaicompat": {
        "reasoning_effort": "string",
        "user": "string"
      },
      "openrouter": {
        "extra_body": {
          "property1": null,
          "property2": null
        },
        "include_usage": true,
        "log_probs": true,
        "logit_bias": {
          "property1": 0,
          "property2": 0
        },
        "parallel_tool_calls": true,
        "provider": {
          "allow_fallbacks": true,
          "data_collection": "string",
          "ignore": [
            "string"
          ],
          "only": [
            "string"
          ],
          "order": [
            "string"
          ],
          "quantizations": [
            "string"
          ],
          "require_parameters": true,
          "sort": "string"
        },
        "reasoning": {
          "effort": "string",
          "enabled": true,
          "exclude": true,
          "max_tokens": 0
        },
        "user": "string"
      },
      "vercel": {
        "extra_body": {
          "property1": null,
          "property2": null
        },
        "logit_bias": {
          "property1": 0,
          "property2": 0
        },
        "logprobs": true,
        "parallel_tool_calls": true,
        "providerOptions": {
          "models": [
            "string"
          ],
          "order": [
            "string"
          ]
        },
        "reasoning": {
          "effort": "string",
          "enabled": true,
          "exclude": true,
          "max_tokens": 0
        },
        "top_logprobs": 0,
        "user": "string"
      }
    },
    "temperature": 0,
    "top_k": 0,
    "top_p": 0
  },
  "provider": "string",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                         |
|--------|--------------------------------------------------------------|-------------|----------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.ChatModelConfig](schemas.md#codersdkchatmodelconfig) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete chat model config

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/experimental/chats/model-configs/{modelConfig} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /api/experimental/chats/model-configs/{modelConfig}`

Experimental: this endpoint is subject to change.

### Parameters

| Name          | In   | Type         | Required | Description     |
|---------------|------|--------------|----------|-----------------|
| `modelConfig` | path | string(uuid) | true     | Model config ID |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update chat model config

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/experimental/chats/model-configs/{modelConfig} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /api/experimental/chats/model-configs/{modelConfig}`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "compression_threshold": 0,
  "context_limit": 0,
  "display_name": "string",
  "enabled": true,
  "is_default": true,
  "model": "string",
  "model_config": {
    "cost": {
      "cache_read_price_per_million_tokens": 0,
      "cache_write_price_per_million_tokens": 0,
      "input_price_per_million_tokens": 0,
      "output_price_per_million_tokens": 0
    },
    "frequency_penalty": 0,
    "max_output_tokens": 0,
    "presence_penalty": 0,
    "provider_options": {
      "anthropic": {
        "allowed_domains": [
          "string"
        ],
        "blocked_domains": [
          "string"
        ],
        "disable_parallel_tool_use": true,
        "effort": "string",
        "send_reasoning": true,
        "thinking": {
          "budget_tokens": 0
        },
        "web_search_enabled": true
      },
      "google": {
        "cached_content": "string",
        "safety_settings": [
          {
            "category": "string",
            "threshold": "string"
          }
        ],
        "thinking_config": {
          "include_thoughts": true,
          "thinking_budget": 0
        },
        "threshold": "string",
        "web_search_enabled": true
      },
      "openai": {
        "allowed_domains": [
          "string"
        ],
        "include": [
          "string"
        ],
        "instructions": "string",
        "log_probs": true,
        "logit_bias": {
          "property1": 0,
          "property2": 0
        },
        "max_completion_tokens": 0,
        "max_tool_calls": 0,
        "metadata": {
          "property1": null,
          "property2": null
        },
        "parallel_tool_calls": true,
        "prediction": {
          "property1": null,
          "property2": null
        },
        "prompt_cache_key": "string",
        "reasoning_effort": "string",
        "reasoning_summary": "string",
        "safety_identifier": "string",
        "search_context_size": "string",
        "service_tier": "string",
        "store": true,
        "strict_json_schema": true,
        "structured_outputs": true,
        "text_verbosity": "string",
        "top_log_probs": 0,
        "user": "string",
        "web_search_enabled": true
      },
      "openaicompat": {
        "reasoning_effort": "string",
        "user": "string"
      },
      "openrouter": {
        "extra_body": {
          "property1": null,
          "property2": null
        },
        "include_usage": true,
        "log_probs": true,
        "logit_bias": {
          "property1": 0,
          "property2": 0
        },
        "parallel_tool_calls": true,
        "provider": {
          "allow_fallbacks": true,
          "data_collection": "string",
          "ignore": [
            "string"
          ],
          "only": [
            "string"
          ],
          "order": [
            "string"
          ],
          "quantizations": [
            "string"
          ],
          "require_parameters": true,
          "sort": "string"
        },
        "reasoning": {
          "effort": "string",
          "enabled": true,
          "exclude": true,
          "max_tokens": 0
        },
        "user": "string"
      },
      "vercel": {
        "extra_body": {
          "property1": null,
          "property2": null
        },
        "logit_bias": {
          "property1": 0,
          "property2": 0
        },
        "logprobs": true,
        "parallel_tool_calls": true,
        "providerOptions": {
          "models": [
            "string"
          ],
          "order": [
            "string"
          ]
        },
        "reasoning": {
          "effort": "string",
          "enabled": true,
          "exclude": true,
          "max_tokens": 0
        },
        "top_logprobs": 0,
        "user": "string"
      }
    },
    "temperature": 0,
    "top_k": 0,
    "top_p": 0
  },
  "provider": "string"
}
```

### Parameters

| Name          | In   | Type                                                                                     | Required | Description     |
|---------------|------|------------------------------------------------------------------------------------------|----------|-----------------|
| `modelConfig` | path | string(uuid)                                                                             | true     | Model config ID |
| `body`        | body | [codersdk.UpdateChatModelConfigRequest](schemas.md#codersdkupdatechatmodelconfigrequest) | true     | Request body    |

### Example responses

> 200 Response

```json
{
  "compression_threshold": 0,
  "context_limit": 0,
  "created_at": "2019-08-24T14:15:22Z",
  "display_name": "string",
  "enabled": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "is_default": true,
  "model": "string",
  "model_config": {
    "cost": {
      "cache_read_price_per_million_tokens": 0,
      "cache_write_price_per_million_tokens": 0,
      "input_price_per_million_tokens": 0,
      "output_price_per_million_tokens": 0
    },
    "frequency_penalty": 0,
    "max_output_tokens": 0,
    "presence_penalty": 0,
    "provider_options": {
      "anthropic": {
        "allowed_domains": [
          "string"
        ],
        "blocked_domains": [
          "string"
        ],
        "disable_parallel_tool_use": true,
        "effort": "string",
        "send_reasoning": true,
        "thinking": {
          "budget_tokens": 0
        },
        "web_search_enabled": true
      },
      "google": {
        "cached_content": "string",
        "safety_settings": [
          {
            "category": "string",
            "threshold": "string"
          }
        ],
        "thinking_config": {
          "include_thoughts": true,
          "thinking_budget": 0
        },
        "threshold": "string",
        "web_search_enabled": true
      },
      "openai": {
        "allowed_domains": [
          "string"
        ],
        "include": [
          "string"
        ],
        "instructions": "string",
        "log_probs": true,
        "logit_bias": {
          "property1": 0,
          "property2": 0
        },
        "max_completion_tokens": 0,
        "max_tool_calls": 0,
        "metadata": {
          "property1": null,
          "property2": null
        },
        "parallel_tool_calls": true,
        "prediction": {
          "property1": null,
          "property2": null
        },
        "prompt_cache_key": "string",
        "reasoning_effort": "string",
        "reasoning_summary": "string",
        "safety_identifier": "string",
        "search_context_size": "string",
        "service_tier": "string",
        "store": true,
        "strict_json_schema": true,
        "structured_outputs": true,
        "text_verbosity": "string",
        "top_log_probs": 0,
        "user": "string",
        "web_search_enabled": true
      },
      "openaicompat": {
        "reasoning_effort": "string",
        "user": "string"
      },
      "openrouter": {
        "extra_body": {
          "property1": null,
          "property2": null
        },
        "include_usage": true,
        "log_probs": true,
        "logit_bias": {
          "property1": 0,
          "property2": 0
        },
        "parallel_tool_calls": true,
        "provider": {
          "allow_fallbacks": true,
          "data_collection": "string",
          "ignore": [
            "string"
          ],
          "only": [
            "string"
          ],
          "order": [
            "string"
          ],
          "quantizations": [
            "string"
          ],
          "require_parameters": true,
          "sort": "string"
        },
        "reasoning": {
          "effort": "string",
          "enabled": true,
          "exclude": true,
          "max_tokens": 0
        },
        "user": "string"
      },
      "vercel": {
        "extra_body": {
          "property1": null,
          "property2": null
        },
        "logit_bias": {
          "property1": 0,
          "property2": 0
        },
        "logprobs": true,
        "parallel_tool_calls": true,
        "providerOptions": {
          "models": [
            "string"
          ],
          "order": [
            "string"
          ]
        },
        "reasoning": {
          "effort": "string",
          "enabled": true,
          "exclude": true,
          "max_tokens": 0
        },
        "top_logprobs": 0,
        "user": "string"
      }
    },
    "temperature": 0,
    "top_k": 0,
    "top_p": 0
  },
  "provider": "string",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                         |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatModelConfig](schemas.md#codersdkchatmodelconfig) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List chat models

### Code samples

```shell
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
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                               |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatModelsResponse](schemas.md#codersdkchatmodelsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List chat providers

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/providers \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/providers`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
[
  {
    "allow_central_api_key_fallback": true,
    "allow_user_api_key": true,
    "base_url": "string",
    "central_api_key_enabled": true,
    "created_at": "2019-08-24T14:15:22Z",
    "display_name": "string",
    "enabled": true,
    "has_api_key": true,
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "provider": "string",
    "source": "database",
    "updated_at": "2019-08-24T14:15:22Z"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                        |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.ChatProviderConfig](schemas.md#codersdkchatproviderconfig) |

<h3 id="list-chat-providers-responseschema">Response Schema</h3>

Status Code **200**

| Name                               | Type                                                                             | Required | Restrictions | Description |
|------------------------------------|----------------------------------------------------------------------------------|----------|--------------|-------------|
| `[array item]`                     | array                                                                            | false    |              |             |
| `» allow_central_api_key_fallback` | boolean                                                                          | false    |              |             |
| `» allow_user_api_key`             | boolean                                                                          | false    |              |             |
| `» base_url`                       | string                                                                           | false    |              |             |
| `» central_api_key_enabled`        | boolean                                                                          | false    |              |             |
| `» created_at`                     | string(date-time)                                                                | false    |              |             |
| `» display_name`                   | string                                                                           | false    |              |             |
| `» enabled`                        | boolean                                                                          | false    |              |             |
| `» has_api_key`                    | boolean                                                                          | false    |              |             |
| `» id`                             | string(uuid)                                                                     | false    |              |             |
| `» provider`                       | string                                                                           | false    |              |             |
| `» source`                         | [codersdk.ChatProviderConfigSource](schemas.md#codersdkchatproviderconfigsource) | false    |              |             |
| `» updated_at`                     | string(date-time)                                                                | false    |              |             |

#### Enumerated Values

| Property | Value(s)                              |
|----------|---------------------------------------|
| `source` | `database`, `env_preset`, `supported` |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create chat provider

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/experimental/chats/providers \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/experimental/chats/providers`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "allow_central_api_key_fallback": true,
  "allow_user_api_key": true,
  "api_key": "string",
  "base_url": "string",
  "central_api_key_enabled": true,
  "display_name": "string",
  "enabled": true,
  "provider": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                           | Required | Description  |
|--------|------|------------------------------------------------------------------------------------------------|----------|--------------|
| `body` | body | [codersdk.CreateChatProviderConfigRequest](schemas.md#codersdkcreatechatproviderconfigrequest) | true     | Request body |

### Example responses

> 201 Response

```json
{
  "allow_central_api_key_fallback": true,
  "allow_user_api_key": true,
  "base_url": "string",
  "central_api_key_enabled": true,
  "created_at": "2019-08-24T14:15:22Z",
  "display_name": "string",
  "enabled": true,
  "has_api_key": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "provider": "string",
  "source": "database",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                               |
|--------|--------------------------------------------------------------|-------------|----------------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.ChatProviderConfig](schemas.md#codersdkchatproviderconfig) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete chat provider

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/experimental/chats/providers/{providerConfig} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /api/experimental/chats/providers/{providerConfig}`

Experimental: this endpoint is subject to change.

### Parameters

| Name             | In   | Type         | Required | Description        |
|------------------|------|--------------|----------|--------------------|
| `providerConfig` | path | string(uuid) | true     | Provider config ID |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update chat provider

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/experimental/chats/providers/{providerConfig} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /api/experimental/chats/providers/{providerConfig}`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "allow_central_api_key_fallback": true,
  "allow_user_api_key": true,
  "api_key": "string",
  "base_url": "string",
  "central_api_key_enabled": true,
  "display_name": "string",
  "enabled": true
}
```

### Parameters

| Name             | In   | Type                                                                                           | Required | Description        |
|------------------|------|------------------------------------------------------------------------------------------------|----------|--------------------|
| `providerConfig` | path | string(uuid)                                                                                   | true     | Provider config ID |
| `body`           | body | [codersdk.UpdateChatProviderConfigRequest](schemas.md#codersdkupdatechatproviderconfigrequest) | true     | Request body       |

### Example responses

> 200 Response

```json
{
  "allow_central_api_key_fallback": true,
  "allow_user_api_key": true,
  "base_url": "string",
  "central_api_key_enabled": true,
  "created_at": "2019-08-24T14:15:22Z",
  "display_name": "string",
  "enabled": true,
  "has_api_key": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "provider": "string",
  "source": "database",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                               |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatProviderConfig](schemas.md#codersdkchatproviderconfig) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List user chat provider configs

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/user-provider-configs \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/user-provider-configs`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
[
  {
    "display_name": "string",
    "has_central_api_key_fallback": true,
    "has_user_api_key": true,
    "provider": "string",
    "provider_id": "fe3d49af-4061-436b-ae60-f7044f252a44"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                |
|--------|---------------------------------------------------------|-------------|---------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.UserChatProviderConfig](schemas.md#codersdkuserchatproviderconfig) |

<h3 id="list-user-chat-provider-configs-responseschema">Response Schema</h3>

Status Code **200**

| Name                             | Type         | Required | Restrictions | Description |
|----------------------------------|--------------|----------|--------------|-------------|
| `[array item]`                   | array        | false    |              |             |
| `» display_name`                 | string       | false    |              |             |
| `» has_central_api_key_fallback` | boolean      | false    |              |             |
| `» has_user_api_key`             | boolean      | false    |              |             |
| `» provider`                     | string       | false    |              |             |
| `» provider_id`                  | string(uuid) | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Upsert user chat provider key

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/experimental/chats/user-provider-configs/{providerConfig} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /api/experimental/chats/user-provider-configs/{providerConfig}`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "api_key": "string"
}
```

### Parameters

| Name             | In   | Type                                                                                             | Required | Description        |
|------------------|------|--------------------------------------------------------------------------------------------------|----------|--------------------|
| `providerConfig` | path | string(uuid)                                                                                     | true     | Provider config ID |
| `body`           | body | [codersdk.CreateUserChatProviderKeyRequest](schemas.md#codersdkcreateuserchatproviderkeyrequest) | true     | Request body       |

### Example responses

> 200 Response

```json
{
  "display_name": "string",
  "has_central_api_key_fallback": true,
  "has_user_api_key": true,
  "provider": "string",
  "provider_id": "fe3d49af-4061-436b-ae60-f7044f252a44"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                       |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.UserChatProviderConfig](schemas.md#codersdkuserchatproviderconfig) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete user chat provider key

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/experimental/chats/user-provider-configs/{providerConfig} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /api/experimental/chats/user-provider-configs/{providerConfig}`

Experimental: this endpoint is subject to change.

### Parameters

| Name             | In   | Type         | Required | Description        |
|------------------|------|--------------|----------|--------------------|
| `providerConfig` | path | string(uuid) | true     | Provider config ID |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Watch chat events for a user via WebSockets

### Code samples

```shell
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
    "last_injected_context": [
      {
        "args": [
          0
        ],
        "args_delta": "string",
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
        "provider_executed": true,
        "provider_metadata": [
          0
        ],
        "result": [
          0
        ],
        "result_delta": "string",
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
    "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
    "last_turn_summary": "string",
    "mcp_server_ids": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
    "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
    "pin_order": 0,
    "plan_mode": "plan",
    "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
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

```shell
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
      "last_injected_context": [
        {
          "args": [
            0
          ],
          "args_delta": "string",
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
          "provider_executed": true,
          "provider_metadata": [
            0
          ],
          "result": [
            0
          ],
          "result_delta": "string",
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
      "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
      "last_turn_summary": "string",
      "mcp_server_ids": [
        "497f6eca-6276-4993-bfeb-53cbbbba6f08"
      ],
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
      "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
      "pin_order": 0,
      "plan_mode": "plan",
      "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
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
  "last_injected_context": [
    {
      "args": [
        0
      ],
      "args_delta": "string",
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
      "provider_executed": true,
      "provider_metadata": [
        0
      ],
      "result": [
        0
      ],
      "result_delta": "string",
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
  "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
  "last_turn_summary": "string",
  "mcp_server_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
  "pin_order": 0,
  "plan_mode": "plan",
  "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
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

```shell
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

## Get chat debug runs

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/{chat}/debug/runs \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/{chat}/debug/runs`

Experimental: this endpoint is subject to change.

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `chat` | path | string(uuid) | true     | Chat ID     |

### Example responses

> 200 Response

```json
[
  {
    "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
    "finished_at": "2019-08-24T14:15:22Z",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "kind": "chat_turn",
    "model": "string",
    "provider": "string",
    "started_at": "2019-08-24T14:15:22Z",
    "status": "in_progress",
    "summary": {
      "property1": null,
      "property2": null
    },
    "updated_at": "2019-08-24T14:15:22Z"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                          |
|--------|---------------------------------------------------------|-------------|---------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.ChatDebugRunSummary](schemas.md#codersdkchatdebugrunsummary) |

<h3 id="get-chat-debug-runs-responseschema">Response Schema</h3>

Status Code **200**

| Name                | Type                                                             | Required | Restrictions | Description |
|---------------------|------------------------------------------------------------------|----------|--------------|-------------|
| `[array item]`      | array                                                            | false    |              |             |
| `» chat_id`         | string(uuid)                                                     | false    |              |             |
| `» finished_at`     | string(date-time)                                                | false    |              |             |
| `» id`              | string(uuid)                                                     | false    |              |             |
| `» kind`            | [codersdk.ChatDebugRunKind](schemas.md#codersdkchatdebugrunkind) | false    |              |             |
| `» model`           | string                                                           | false    |              |             |
| `» provider`        | string                                                           | false    |              |             |
| `» started_at`      | string(date-time)                                                | false    |              |             |
| `» status`          | [codersdk.ChatDebugStatus](schemas.md#codersdkchatdebugstatus)   | false    |              |             |
| `» summary`         | object                                                           | false    |              |             |
| `»» [any property]` | any                                                              | false    |              |             |
| `» updated_at`      | string(date-time)                                                | false    |              |             |

#### Enumerated Values

| Property | Value(s)                                                  |
|----------|-----------------------------------------------------------|
| `kind`   | `chat_turn`, `compaction`, `quickgen`, `title_generation` |
| `status` | `completed`, `error`, `in_progress`, `interrupted`        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get chat debug run

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/chats/{chat}/debug/runs/{debugRun} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/chats/{chat}/debug/runs/{debugRun}`

Experimental: this endpoint is subject to change.

### Parameters

| Name       | In   | Type         | Required | Description  |
|------------|------|--------------|----------|--------------|
| `chat`     | path | string(uuid) | true     | Chat ID      |
| `debugRun` | path | string(uuid) | true     | Debug run ID |

### Example responses

> 200 Response

```json
{
  "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
  "finished_at": "2019-08-24T14:15:22Z",
  "history_tip_message_id": 0,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "kind": "chat_turn",
  "model": "string",
  "model_config_id": "f5fb4d91-62ca-4377-9ee6-5d43ba00d205",
  "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
  "provider": "string",
  "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
  "started_at": "2019-08-24T14:15:22Z",
  "status": "in_progress",
  "steps": [
    {
      "assistant_message_id": 0,
      "attempts": [
        {
          "property1": null,
          "property2": null
        }
      ],
      "chat_id": "efc9fe20-a1e5-4a8c-9c48-f1b30c1e4f86",
      "error": {
        "property1": null,
        "property2": null
      },
      "finished_at": "2019-08-24T14:15:22Z",
      "history_tip_message_id": 0,
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "metadata": {
        "property1": null,
        "property2": null
      },
      "normalized_request": {
        "property1": null,
        "property2": null
      },
      "normalized_response": {
        "property1": null,
        "property2": null
      },
      "operation": "stream",
      "run_id": "dded282c-8ebd-44cf-8ba5-9a234973d1ec",
      "started_at": "2019-08-24T14:15:22Z",
      "status": "in_progress",
      "step_number": 0,
      "updated_at": "2019-08-24T14:15:22Z",
      "usage": {
        "property1": null,
        "property2": null
      }
    }
  ],
  "summary": {
    "property1": null,
    "property2": null
  },
  "trigger_message_id": 0,
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                   |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChatDebugRun](schemas.md#codersdkchatdebugrun) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get chat diff contents

### Code samples

```shell
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

```shell
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
      "last_injected_context": [
        {
          "args": [
            0
          ],
          "args_delta": "string",
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
          "provider_executed": true,
          "provider_metadata": [
            0
          ],
          "result": [
            0
          ],
          "result_delta": "string",
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
      "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
      "last_turn_summary": "string",
      "mcp_server_ids": [
        "497f6eca-6276-4993-bfeb-53cbbbba6f08"
      ],
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
      "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
      "pin_order": 0,
      "plan_mode": "plan",
      "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
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
  "last_injected_context": [
    {
      "args": [
        0
      ],
      "args_delta": "string",
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
      "provider_executed": true,
      "provider_metadata": [
        0
      ],
      "result": [
        0
      ],
      "result_delta": "string",
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
  "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
  "last_turn_summary": "string",
  "mcp_server_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
  "pin_order": 0,
  "plan_mode": "plan",
  "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
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

```shell
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
          "provider_executed": true,
          "provider_metadata": [
            0
          ],
          "result": [
            0
          ],
          "result_delta": "string",
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
          "provider_executed": true,
          "provider_metadata": [
            0
          ],
          "result": [
            0
          ],
          "result_delta": "string",
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

```shell
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
  "plan_mode": "plan"
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
        "provider_executed": true,
        "provider_metadata": [
          0
        ],
        "result": [
          0
        ],
        "result_delta": "string",
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
        "provider_executed": true,
        "provider_metadata": [
          0
        ],
        "result": [
          0
        ],
        "result_delta": "string",
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

```shell
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
  ]
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
        "provider_executed": true,
        "provider_metadata": [
          0
        ],
        "result": [
          0
        ],
        "result_delta": "string",
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

## Delete chat queued message

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/experimental/chats/{chat}/queue/{queuedMessage} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /api/experimental/chats/{chat}/queue/{queuedMessage}`

Experimental: this endpoint is subject to change.

### Parameters

| Name            | In   | Type         | Required | Description       |
|-----------------|------|--------------|----------|-------------------|
| `chat`          | path | string(uuid) | true     | Chat ID           |
| `queuedMessage` | path | integer      | true     | Queued message ID |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Promote chat queued message

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/experimental/chats/{chat}/queue/{queuedMessage}/promote \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/experimental/chats/{chat}/queue/{queuedMessage}/promote`

Experimental: this endpoint is subject to change.

### Parameters

| Name            | In   | Type         | Required | Description       |
|-----------------|------|--------------|----------|-------------------|
| `chat`          | path | string(uuid) | true     | Chat ID           |
| `queuedMessage` | path | integer      | true     | Queued message ID |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Stream chat events via WebSockets

### Code samples

```shell
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
        "provider_executed": true,
        "provider_metadata": [
          0
        ],
        "result": [
          0
        ],
        "result_delta": "string",
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
    "part": {
      "args": [
        0
      ],
      "args_delta": "string",
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
      "provider_executed": true,
      "provider_metadata": [
        0
      ],
      "result": [
        0
      ],
      "result_delta": "string",
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
    "role": "system"
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
          "provider_executed": true,
          "provider_metadata": [
            0
          ],
          "result": [
            0
          ],
          "result_delta": "string",
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

```shell
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

```shell
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

## Propose chat title

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/experimental/chats/{chat}/title/propose \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/experimental/chats/{chat}/title/propose`

Experimental: this endpoint is subject to change.

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `chat` | path | string(uuid) | true     | Chat ID     |

### Example responses

> 200 Response

```json
{
  "title": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                           |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ProposeChatTitleResponse](schemas.md#codersdkproposechattitleresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Regenerate chat title

### Code samples

```shell
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
      "last_injected_context": [
        {
          "args": [
            0
          ],
          "args_delta": "string",
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
          "provider_executed": true,
          "provider_metadata": [
            0
          ],
          "result": [
            0
          ],
          "result_delta": "string",
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
      "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
      "last_turn_summary": "string",
      "mcp_server_ids": [
        "497f6eca-6276-4993-bfeb-53cbbbba6f08"
      ],
      "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
      "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
      "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
      "pin_order": 0,
      "plan_mode": "plan",
      "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
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
  "last_injected_context": [
    {
      "args": [
        0
      ],
      "args_delta": "string",
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
      "provider_executed": true,
      "provider_metadata": [
        0
      ],
      "result": [
        0
      ],
      "result_delta": "string",
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
  "last_model_config_id": "30ebb95f-c255-4759-9429-89aa4ec1554c",
  "last_turn_summary": "string",
  "mcp_server_ids": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
  "parent_chat_id": "c3609ee6-3b11-4a93-b9ae-e4fabcc99359",
  "pin_order": 0,
  "plan_mode": "plan",
  "root_chat_id": "2898031c-fdce-4e3e-8c53-4481dd42fcd7",
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

## Submit chat tool results

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/experimental/chats/{chat}/tool-results \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/experimental/chats/{chat}/tool-results`

Experimental: this endpoint is subject to change.

> Body parameter

```json
{
  "results": [
    {
      "is_error": true,
      "output": [
        0
      ],
      "tool_call_id": "string"
    }
  ]
}
```

### Parameters

| Name   | In   | Type                                                                             | Required | Description  |
|--------|------|----------------------------------------------------------------------------------|----------|--------------|
| `chat` | path | string(uuid)                                                                     | true     | Chat ID      |
| `body` | body | [codersdk.SubmitToolResultsRequest](schemas.md#codersdksubmittoolresultsrequest) | true     | Request body |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
