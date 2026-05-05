# Chats

## List chats

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/experimental/chats \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /experimental/chats`

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
      "kind": "string",
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
| `»» kind`                         | string                                                                 | false    |              | Kind classifies the error for consistent client rendering.                                                                                                                                                                                                                 |
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
| `type`        | `context-file`, `file`, `file-reference`, `reasoning`, `skill`, `source`, `text`, `tool-call`, `tool-result` |
| `plan_mode`   | `plan`                                                                                                       |
| `status`      | `completed`, `error`, `paused`, `pending`, `requires_action`, `running`, `waiting`                           |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create chat

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/experimental/chats \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /experimental/chats`

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
        "kind": "string",
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
    "kind": "string",
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

## Upload chat file

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/experimental/chats/files?organization=497f6eca-6276-4993-bfeb-53cbbbba6f08 \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /experimental/chats/files`

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
curl -X GET http://coder-server:8080/experimental/chats/files/{file} \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /experimental/chats/files/{file}`

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

```shell
# Example request using curl
curl -X GET http://coder-server:8080/experimental/chats/models \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /experimental/chats/models`

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

## Watch chat events for a user via WebSockets

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/experimental/chats/watch \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /experimental/chats/watch`

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
      "kind": "string",
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
curl -X GET http://coder-server:8080/experimental/chats/{chat} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /experimental/chats/{chat}`

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
        "kind": "string",
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
    "kind": "string",
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
curl -X PATCH http://coder-server:8080/experimental/chats/{chat} \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /experimental/chats/{chat}`

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

## Get chat diff contents

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/experimental/chats/{chat}/diff \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /experimental/chats/{chat}/diff`

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
curl -X POST http://coder-server:8080/experimental/chats/{chat}/interrupt \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /experimental/chats/{chat}/interrupt`

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
        "kind": "string",
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
    "kind": "string",
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
curl -X GET http://coder-server:8080/experimental/chats/{chat}/messages \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /experimental/chats/{chat}/messages`

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
curl -X POST http://coder-server:8080/experimental/chats/{chat}/messages \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /experimental/chats/{chat}/messages`

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
curl -X PATCH http://coder-server:8080/experimental/chats/{chat}/messages/{message} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /experimental/chats/{chat}/messages/{message}`

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

## Stream chat events via WebSockets

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/experimental/chats/{chat}/stream \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /experimental/chats/{chat}/stream`

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
    "kind": "string",
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
    "kind": "string",
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
curl -X GET http://coder-server:8080/experimental/chats/{chat}/stream/desktop \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /experimental/chats/{chat}/stream/desktop`

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
curl -X GET http://coder-server:8080/experimental/chats/{chat}/stream/git \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /experimental/chats/{chat}/stream/git`

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

```shell
# Example request using curl
curl -X POST http://coder-server:8080/experimental/chats/{chat}/title/regenerate \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /experimental/chats/{chat}/title/regenerate`

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
        "kind": "string",
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
    "kind": "string",
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
