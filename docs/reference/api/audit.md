# Audit

## List AI agent identities for a sponsor

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/ai-audit/agents \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/ai-audit/agents`

### Parameters

| Name      | In    | Type   | Required | Description                                  |
|-----------|-------|--------|----------|----------------------------------------------|
| `sponsor` | query | string | false    | Sponsor user ID, username, or 'me' (default) |

### Example responses

> 200 Response

```json
[
  {
    "creation_site_id": "ef1bb01e-c877-422c-959f-1d403da8b9cb",
    "creation_site_type": "string",
    "creation_time": "2019-08-24T14:15:22Z",
    "display_name": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "owner_id": "8826ee2e-7933-4665-aef2-2393f84a0d05",
    "state": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                            |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.AIAuditAgent](schemas.md#codersdkaiauditagent) |

<h3 id="list-ai-agent-identities-for-a-sponsor-responseschema">Response Schema</h3>

Status Code **200**

| Name                   | Type              | Required | Restrictions | Description                                                                    |
|------------------------|-------------------|----------|--------------|--------------------------------------------------------------------------------|
| `[array item]`         | array             | false    |              |                                                                                |
| `» creation_site_id`   | string(uuid)      | false    |              |                                                                                |
| `» creation_site_type` | string            | false    |              |                                                                                |
| `» creation_time`      | string(date-time) | false    |              |                                                                                |
| `» display_name`       | string            | false    |              | Display name is computed from the creation site and ID; agents store no name.  |
| `» id`                 | string(uuid)      | false    |              | ID is the ai_agent_ledger identity. Audit records reference it as ai_agent_id. |
| `» owner_id`           | string(uuid)      | false    |              |                                                                                |
| `» state`              | string            | false    |              | State is active, dormant, or retired.                                          |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get the AI activity timeline for a sponsor

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/ai-audit/timeline \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/ai-audit/timeline`

### Parameters

| Name          | In    | Type              | Required | Description                                                                               |
|---------------|-------|-------------------|----------|-------------------------------------------------------------------------------------------|
| `sponsor`     | query | string            | false    | Sponsor user ID, username, or 'me' (default)                                              |
| `ai_agent_id` | query | string(uuid)      | false    | Restrict events to one agentic identity                                                   |
| `after_time`  | query | string(date-time) | false    | Exclusive lower bound on occurred_at (RFC3339)                                            |
| `before_time` | query | string(date-time) | false    | Exclusive upper bound on occurred_at (RFC3339); pass the last event's occurred_at to page |
| `types`       | query | string            | false    | Comma-separated event types to include                                                    |
| `limit`       | query | integer           | false    | Page size (default 100, max 1000)                                                         |

### Example responses

> 200 Response

```json
{
  "count": 0,
  "events": [
    {
      "ai_agent_id": "cbaf6aba-437a-4fd2-9d34-7875f81689e6",
      "detail": {
        "property1": null,
        "property2": null
      },
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "occurred_at": "2019-08-24T14:15:22Z",
      "sponsor": {
        "avatar_url": "http://example.com",
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "name": "string",
        "username": "string"
      },
      "summary": "string",
      "type": "sandbox_session_started",
      "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
      "workspace_name": "string"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                         |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AIAuditTimelineResponse](schemas.md#codersdkaiaudittimelineresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get audit logs

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/audit?limit=0 \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/audit`

### Parameters

| Name     | In    | Type    | Required | Description  |
|----------|-------|---------|----------|--------------|
| `q`      | query | string  | false    | Search query |
| `limit`  | query | integer | true     | Page limit   |
| `offset` | query | integer | false    | Page offset  |

### Example responses

> 200 Response

```json
{
  "audit_logs": [
    {
      "action": "create",
      "additional_fields": {},
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
      "on_behalf_of": {
        "avatar_url": "http://example.com",
        "created_at": "2019-08-24T14:15:22Z",
        "email": "user@example.com",
        "has_ai_seat": true,
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "is_service_account": true,
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
        "has_ai_seat": true,
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "is_service_account": true,
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
  "count": 0,
  "count_cap": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                           |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AuditLogResponse](schemas.md#codersdkauditlogresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
