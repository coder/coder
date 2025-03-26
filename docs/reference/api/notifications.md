# Notifications

## Get notification dispatch methods

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/notifications/dispatch-methods \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /notifications/dispatch-methods`

### Example responses

> 200 Response

```json
[
  {
    "available": [
      "string"
    ],
    "default": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                          |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.NotificationMethodsResponse](schemas.md#codersdknotificationmethodsresponse) |

<h3 id="get-notification-dispatch-methods-responseschema">Response Schema</h3>

Status Code **200**

| Name           | Type   | Required | Restrictions | Description |
|----------------|--------|----------|--------------|-------------|
| `[array item]` | array  | false    |              |             |
| `» available`  | array  | false    |              |             |
| `» default`    | string | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List inbox notifications

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/notifications/inbox \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /notifications/inbox`

### Parameters

| Name              | In    | Type         | Required | Description                                                                                                     |
|-------------------|-------|--------------|----------|-----------------------------------------------------------------------------------------------------------------|
| `targets`         | query | string       | false    | Comma-separated list of target IDs to filter notifications                                                      |
| `templates`       | query | string       | false    | Comma-separated list of template IDs to filter notifications                                                    |
| `read_status`     | query | string       | false    | Filter notifications by read status. Possible values: read, unread, all                                         |
| `starting_before` | query | string(uuid) | false    | ID of the last notification from the current page. Notifications returned will be older than the associated one |

### Example responses

> 200 Response

```json
{
  "notifications": [
    {
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
  ],
  "unread_count": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                       |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ListInboxNotificationsResponse](schemas.md#codersdklistinboxnotificationsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Mark all unread notifications as read

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/notifications/inbox/mark-all-as-read \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /notifications/inbox/mark-all-as-read`

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Watch for new inbox notifications

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/notifications/inbox/watch \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /notifications/inbox/watch`

### Parameters

| Name          | In    | Type   | Required | Description                                                             |
|---------------|-------|--------|----------|-------------------------------------------------------------------------|
| `targets`     | query | string | false    | Comma-separated list of target IDs to filter notifications              |
| `templates`   | query | string | false    | Comma-separated list of template IDs to filter notifications            |
| `read_status` | query | string | false    | Filter notifications by read status. Possible values: read, unread, all |
| `format`      | query | string | false    | Define the output format for notifications title and body.              |

#### Enumerated Values

| Parameter | Value       |
|-----------|-------------|
| `format`  | `plaintext` |
| `format`  | `markdown`  |

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
  },
  "unread_count": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                   |
|--------|---------------------------------------------------------|-------------|------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.GetInboxNotificationResponse](schemas.md#codersdkgetinboxnotificationresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update read status of a notification

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/notifications/inbox/{id}/read-status \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /notifications/inbox/{id}/read-status`

### Parameters

| Name | In   | Type   | Required | Description            |
|------|------|--------|----------|------------------------|
| `id` | path | string | true     | id of the notification |

### Example responses

> 200 Response

```json
{
  "detail": "string",
  "message": "string",
  "validations": [
    {
      "detail": "string",
      "field": "string"
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                           |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Response](schemas.md#codersdkresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

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
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------------|
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
|--------|------|----------------------------------------------------------------------------|----------|--------------------------------|
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
|--------|-----------------------------------------------------------------|--------------|----------------------------------------------------------------------------|
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
    "enabled_by_default": true,
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
|--------|---------------------------------------------------------|-------------|-----------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.NotificationTemplate](schemas.md#codersdknotificationtemplate) |

<h3 id="get-system-notification-templates-responseschema">Response Schema</h3>

Status Code **200**

| Name                   | Type         | Required | Restrictions | Description |
|------------------------|--------------|----------|--------------|-------------|
| `[array item]`         | array        | false    |              |             |
| `» actions`            | string       | false    |              |             |
| `» body_template`      | string       | false    |              |             |
| `» enabled_by_default` | boolean      | false    |              |             |
| `» group`              | string       | false    |              |             |
| `» id`                 | string(uuid) | false    |              |             |
| `» kind`               | string       | false    |              |             |
| `» method`             | string       | false    |              |             |
| `» name`               | string       | false    |              |             |
| `» title_template`     | string       | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Send a test notification

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/notifications/test \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /notifications/test`

### Responses

| Status | Meaning                                                 | Description | Schema |
|--------|---------------------------------------------------------|-------------|--------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get user notification preferences

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/{user}/notifications/preferences \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/{user}/notifications/preferences`

### Parameters

| Name   | In   | Type   | Required | Description          |
|--------|------|--------|----------|----------------------|
| `user` | path | string | true     | User ID, name, or me |

### Example responses

> 200 Response

```json
[
  {
    "disabled": true,
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "updated_at": "2019-08-24T14:15:22Z"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                |
|--------|---------------------------------------------------------|-------------|---------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.NotificationPreference](schemas.md#codersdknotificationpreference) |

<h3 id="get-user-notification-preferences-responseschema">Response Schema</h3>

Status Code **200**

| Name           | Type              | Required | Restrictions | Description |
|----------------|-------------------|----------|--------------|-------------|
| `[array item]` | array             | false    |              |             |
| `» disabled`   | boolean           | false    |              |             |
| `» id`         | string(uuid)      | false    |              |             |
| `» updated_at` | string(date-time) | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update user notification preferences

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/users/{user}/notifications/preferences \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /users/{user}/notifications/preferences`

> Body parameter

```json
{
  "push_subscription": "string",
  "template_disabled_map": {
    "property1": true,
    "property2": true
  }
}
```

### Parameters

| Name   | In   | Type                                                                                               | Required | Description          |
|--------|------|----------------------------------------------------------------------------------------------------|----------|----------------------|
| `user` | path | string                                                                                             | true     | User ID, name, or me |
| `body` | body | [codersdk.UpdateUserNotificationPreferences](schemas.md#codersdkupdateusernotificationpreferences) | true     | Preferences          |

### Example responses

> 200 Response

```json
[
  {
    "disabled": true,
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "updated_at": "2019-08-24T14:15:22Z"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                |
|--------|---------------------------------------------------------|-------------|---------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.NotificationPreference](schemas.md#codersdknotificationpreference) |

<h3 id="update-user-notification-preferences-responseschema">Response Schema</h3>

Status Code **200**

| Name           | Type              | Required | Restrictions | Description |
|----------------|-------------------|----------|--------------|-------------|
| `[array item]` | array             | false    |              |             |
| `» disabled`   | boolean           | false    |              |             |
| `» id`         | string(uuid)      | false    |              |             |
| `» updated_at` | string(date-time) | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create user webpush notification subscription

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/users/{user}/notifications/push/subscription \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /users/{user}/notifications/push/subscription`

> Body parameter

```json
{
  "auth_key": "string",
  "endpoint": "string",
  "p256dh_key": "string"
}
```

### Parameters

| Name   | In   | Type                                                                   | Required | Description          |
|--------|------|------------------------------------------------------------------------|----------|----------------------|
| `user` | path | string                                                                 | true     | User ID, name, or me |
| `body` | body | [codersdk.WebpushSubscription](schemas.md#codersdkwebpushsubscription) | true     | Webpush subscription |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete user webpush notification subscription

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/users/{user}/notifications/push/subscription \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /users/{user}/notifications/push/subscription`

> Body parameter

```json
{
  "endpoint": "string"
}
```

### Parameters

| Name   | In   | Type                                                                               | Required | Description                    |
|--------|------|------------------------------------------------------------------------------------|----------|--------------------------------|
| `user` | path | string                                                                             | true     | User ID, name, or me           |
| `body` | body | [codersdk.DeleteWebpushSubscription](schemas.md#codersdkdeletewebpushsubscription) | true     | Push notification subscription |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Send a test push notification

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/users/{user}/notifications/push/test \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /users/{user}/notifications/push/test`

### Parameters

| Name   | In   | Type   | Required | Description          |
|--------|------|--------|----------|----------------------|
| `user` | path | string | true     | User ID, name, or me |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
