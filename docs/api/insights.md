# Insights

## Get deployment DAUs

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/insights/daus?tz_offset=0 \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /insights/daus`

### Parameters

| Name        | In    | Type    | Required | Description                |
| ----------- | ----- | ------- | -------- | -------------------------- |
| `tz_offset` | query | integer | true     | Time-zone offset (e.g. -2) |

### Example responses

> 200 Response

```json
{
  "entries": [
    {
      "amount": 0,
      "date": "string"
    }
  ],
  "tz_hour_offset": 0
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                   |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.DAUsResponse](schemas.md#codersdkdausresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get insights about templates

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/insights/templates?start_time=2019-08-24T14%3A15%3A22Z&end_time=2019-08-24T14%3A15%3A22Z&interval=week \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /insights/templates`

### Parameters

| Name           | In    | Type              | Required | Description  |
| -------------- | ----- | ----------------- | -------- | ------------ |
| `start_time`   | query | string(date-time) | true     | Start time   |
| `end_time`     | query | string(date-time) | true     | End time     |
| `interval`     | query | string            | true     | Interval     |
| `template_ids` | query | array[string]     | false    | Template IDs |

#### Enumerated Values

| Parameter  | Value  |
| ---------- | ------ |
| `interval` | `week` |
| `interval` | `day`  |

### Example responses

> 200 Response

```json
{
  "interval_reports": [
    {
      "active_users": 14,
      "end_time": "2019-08-24T14:15:22Z",
      "interval": "week",
      "start_time": "2019-08-24T14:15:22Z",
      "template_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"]
    }
  ],
  "report": {
    "active_users": 22,
    "apps_usage": [
      {
        "display_name": "Visual Studio Code",
        "icon": "string",
        "seconds": 80500,
        "slug": "vscode",
        "template_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
        "times_used": 2,
        "type": "builtin"
      }
    ],
    "end_time": "2019-08-24T14:15:22Z",
    "parameters_usage": [
      {
        "description": "string",
        "display_name": "string",
        "name": "string",
        "options": [
          {
            "description": "string",
            "icon": "string",
            "name": "string",
            "value": "string"
          }
        ],
        "template_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
        "type": "string",
        "values": [
          {
            "count": 0,
            "value": "string"
          }
        ]
      }
    ],
    "start_time": "2019-08-24T14:15:22Z",
    "template_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"]
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                           |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.TemplateInsightsResponse](schemas.md#codersdktemplateinsightsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get insights about user activity

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/insights/user-activity?start_time=2019-08-24T14%3A15%3A22Z&end_time=2019-08-24T14%3A15%3A22Z \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /insights/user-activity`

### Parameters

| Name           | In    | Type              | Required | Description  |
| -------------- | ----- | ----------------- | -------- | ------------ |
| `start_time`   | query | string(date-time) | true     | Start time   |
| `end_time`     | query | string(date-time) | true     | End time     |
| `template_ids` | query | array[string]     | false    | Template IDs |

### Example responses

> 200 Response

```json
{
  "report": {
    "end_time": "2019-08-24T14:15:22Z",
    "start_time": "2019-08-24T14:15:22Z",
    "template_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
    "users": [
      {
        "avatar_url": "http://example.com",
        "seconds": 80500,
        "template_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
        "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5",
        "username": "string"
      }
    ]
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                   |
| ------ | ------------------------------------------------------- | ----------- | ---------------------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.UserActivityInsightsResponse](schemas.md#codersdkuseractivityinsightsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get insights about user latency

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/insights/user-latency?start_time=2019-08-24T14%3A15%3A22Z&end_time=2019-08-24T14%3A15%3A22Z \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /insights/user-latency`

### Parameters

| Name           | In    | Type              | Required | Description  |
| -------------- | ----- | ----------------- | -------- | ------------ |
| `start_time`   | query | string(date-time) | true     | Start time   |
| `end_time`     | query | string(date-time) | true     | End time     |
| `template_ids` | query | array[string]     | false    | Template IDs |

### Example responses

> 200 Response

```json
{
  "report": {
    "end_time": "2019-08-24T14:15:22Z",
    "start_time": "2019-08-24T14:15:22Z",
    "template_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
    "users": [
      {
        "avatar_url": "http://example.com",
        "latency_ms": {
          "p50": 31.312,
          "p95": 119.832
        },
        "template_ids": ["497f6eca-6276-4993-bfeb-53cbbbba6f08"],
        "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5",
        "username": "string"
      }
    ]
  }
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                 |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.UserLatencyInsightsResponse](schemas.md#codersdkuserlatencyinsightsresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
