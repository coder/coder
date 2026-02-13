# Changelog

## List changelog entries

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/changelog \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /changelog`

### Example responses

> 200 Response

```json
{
  "entries": [
    {
      "content": "string",
      "date": "string",
      "image_url": "string",
      "summary": "string",
      "title": "string",
      "version": "string"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ListChangelogEntriesResponse](schemas.md#codersdklistchangelogentriesresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get changelog asset

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/changelog/assets/{path} \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /changelog/assets/{path}`

### Parameters

| Name   | In   | Type   | Required | Description |
|--------|------|--------|----------|-------------|
| `path` | path | string | true     | Asset path  |

### Responses

| Status | Meaning                                                 | Description | Schema |
|--------|---------------------------------------------------------|-------------|--------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get unread changelog notification

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/changelog/unread \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /changelog/unread`

### Example responses

> 200 Response

```json
{
  "notification": {
    "actions": [
      {
        "label": "string",
        "url": "string"
      }
    ],
    "content": "string",
    "created_at": "2019-08-24T14:15:22Z",
    "icon": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "read_at": "string",
    "targets": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
    "title": "string",
    "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                                 |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.UnreadChangelogNotificationResponse](schemas.md#codersdkunreadchangelognotificationresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get changelog entry

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/changelog/{version} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /changelog/{version}`

### Parameters

| Name      | In   | Type   | Required | Description |
|-----------|------|--------|----------|-------------|
| `version` | path | string | true     | Version     |

### Example responses

> 200 Response

```json
{
  "content": "string",
  "date": "string",
  "image_url": "string",
  "summary": "string",
  "title": "string",
  "version": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ChangelogEntry](schemas.md#codersdkchangelogentry) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
