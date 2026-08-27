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
    "created_at": "2019-08-24T14:15:22Z",
    "deleted": true,
    "origin_id": "fa284d86-c703-4b55-825c-a163977fd80a",
    "origin_type": "string",
    "owner_user_id": "65139110-7c3c-4777-b692-80c218be3b9d",
    "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5",
    "username": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                            |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.AIAuditAgent](schemas.md#codersdkaiauditagent) |

<h3 id="list-ai-agent-identities-for-a-sponsor-responseschema">Response Schema</h3>

Status Code **200**

| Name              | Type              | Required | Restrictions | Description                                                                               |
|-------------------|-------------------|----------|--------------|-------------------------------------------------------------------------------------------|
| `[array item]`    | array             | false    |              |                                                                                           |
| `» created_at`    | string(date-time) | false    |              |                                                                                           |
| `» deleted`       | boolean           | false    |              |                                                                                           |
| `» origin_id`     | string(uuid)      | false    |              |                                                                                           |
| `» origin_type`   | string            | false    |              |                                                                                           |
| `» owner_user_id` | string(uuid)      | false    |              |                                                                                           |
| `» user_id`       | string(uuid)      | false    |              | User ID identifies the AI agent's user record. Audit records reference it as ai_agent_id. |
| `» username`      | string            | false    |              |                                                                                           |

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
