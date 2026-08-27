# Audit

## Get AI agent audit trail timeline

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/ai-audit/timeline \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /ai-audit/timeline`

### Parameters

| Name          | In    | Type              | Required | Description                                                          |
|---------------|-------|-------------------|----------|----------------------------------------------------------------------|
| `owner`       | query | string            | false    | Owner user ID, username, or 'me' (default). Current-owner semantics. |
| `after_time`  | query | string(date-time) | false    | Lower time bound, exclusive (RFC3339)                                |
| `before_time` | query | string(date-time) | false    | Upper time bound, exclusive (RFC3339)                                |
| `ai_agent_id` | query | string(uuid)      | false    | Filter to one AI agent                                               |
| `types`       | query | string            | false    | Comma-separated event types                                          |
| `limit`       | query | integer           | false    | Page size (default 100, max 1000)                                    |

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
      "id": "string",
      "occurred_at": "2019-08-24T14:15:22Z",
      "owner": {
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "type": "string",
        "username": "string"
      },
      "recorded_at": "2019-08-24T14:15:22Z",
      "summary": "string",
      "type": "ai_agent_lifecycle",
      "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                   |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AIAuditTrailResponse](schemas.md#codersdkaiaudittrailresponse) |

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
