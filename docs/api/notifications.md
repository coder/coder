# Notifications

## Get notifications settings

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/notifications/settings \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /notifications/settings`

### Example responses

> 200 Response

```json
{
  "notifier_paused": true
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                     |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.NotificationsSettings](schemas.md#codersdknotificationssettings) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update notifications settings

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/notifications/settings \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /notifications/settings`

> Body parameter

```json
{
  "notifier_paused": true
}
```

### Parameters

| Name   | In   | Type                                                                       | Required | Description                    |
| ------ | ---- | -------------------------------------------------------------------------- | -------- | ------------------------------ |
| `body` | body | [codersdk.NotificationsSettings](schemas.md#codersdknotificationssettings) | true     | Notifications settings request |

### Example responses

> 200 Response

```json
{
  "notifier_paused": true
}
```

### Responses

| Status | Meaning                                                         | Description  | Schema                                                                     |
| ------ | --------------------------------------------------------------- | ------------ | -------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)         | OK           | [codersdk.NotificationsSettings](schemas.md#codersdknotificationssettings) |
| 304    | [Not Modified](https://tools.ietf.org/html/rfc7232#section-4.1) | Not Modified |                                                                            |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get system notification templates

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/notifications/templates/system \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /notifications/templates/system`

### Example responses

> 200 Response

```json
[
  {
    "actions": "string",
    "body_template": "string",
    "group": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "kind": "string",
    "method": "string",
    "name": "string",
    "title_template": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                            |
| ------ | ------------------------------------------------------- | ----------- | --------------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.NotificationTemplate](schemas.md#codersdknotificationtemplate) |

<h3 id="get-system-notification-templates-responseschema">Response Schema</h3>

Status Code **200**

| Name               | Type         | Required | Restrictions | Description |
| ------------------ | ------------ | -------- | ------------ | ----------- |
| `[array item]`     | array        | false    |              |             |
| `» actions`        | string       | false    |              |             |
| `» body_template`  | string       | false    |              |             |
| `» group`          | string       | false    |              |             |
| `» id`             | string(uuid) | false    |              |             |
| `» kind`           | string       | false    |              |             |
| `» method`         | string       | false    |              |             |
| `» name`           | string       | false    |              |             |
| `» title_template` | string       | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
