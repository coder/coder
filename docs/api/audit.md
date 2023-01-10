# Audit

> This page is incomplete, stay tuned.

## Get audit logs

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/audit?q=string \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /audit`

### Parameters

| Name       | In    | Type         | Required | Description  |
| ---------- | ----- | ------------ | -------- | ------------ |
| `q`        | query | string       | true     | Search query |
| `after_id` | query | string(uuid) | false    | After ID     |
| `limit`    | query | integer      | false    | Page limit   |
| `offset`   | query | integer      | false    | Page offset  |

### Example responses

> 200 Response

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
      "id": "string",
      "ip": "string",
      "is_deleted": true,
      "organization_id": "string",
      "request_id": "string",
      "resource_icon": "string",
      "resource_id": "string",
      "resource_link": "string",
      "resource_target": "string",
      "resource_type": "organization",
      "status_code": 0,
      "time": "string",
      "user": {
        "avatar_url": "http://example.com",
        "created_at": "2019-08-24T14:15:22Z",
        "email": "string",
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

### Responses

| Status | Meaning                                                 | Description | Schema                                                           |
| ------ | ------------------------------------------------------- | ----------- | ---------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AuditLogResponse](schemas.md#codersdkauditlogresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Generate fake audit log

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/audit/testgenerate \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /audit/testgenerate`

> Body parameter

```json
{
  "action": "create",
  "resource_id": "string",
  "resource_type": "organization",
  "time": "string"
}
```

### Parameters

| Name   | In   | Type                                                                               | Required | Description       |
| ------ | ---- | ---------------------------------------------------------------------------------- | -------- | ----------------- |
| `body` | body | [codersdk.CreateTestAuditLogRequest](schemas.md#codersdkcreatetestauditlogrequest) | true     | Audit log request |

### Responses

| Status | Meaning                                                         | Description | Schema |
| ------ | --------------------------------------------------------------- | ----------- | ------ |
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
