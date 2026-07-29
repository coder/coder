# Experimental

## List AI model prices

### Code samples

```sh
# Example request using curl
curl -X GET http://coder-server:8080/api/experimental/ai/model-prices \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/experimental/ai/model-prices`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
[
  {
    "cache_read_price": 0,
    "cache_write_price": 0,
    "created_at": "2019-08-24T14:15:22Z",
    "input_price": 0,
    "model": "string",
    "output_price": 0,
    "provider": "string",
    "updated_at": "2019-08-24T14:15:22Z"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                            |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.AIModelPrice](schemas.md#codersdkaimodelprice) |

<h3 id="list-ai-model-prices-responseschema">Response Schema</h3>

Status Code **200**

| Name                  | Type              | Required | Restrictions | Description |
|-----------------------|-------------------|----------|--------------|-------------|
| `[array item]`        | array             | false    |              |             |
| `» cache_read_price`  | integer           | false    |              |             |
| `» cache_write_price` | integer           | false    |              |             |
| `» created_at`        | string(date-time) | false    |              |             |
| `» input_price`       | integer           | false    |              |             |
| `» model`             | string            | false    |              |             |
| `» output_price`      | integer           | false    |              |             |
| `» provider`          | string            | false    |              |             |
| `» updated_at`        | string(date-time) | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Upsert AI model prices

### Code samples

```sh
# Example request using curl
curl -X PUT http://coder-server:8080/api/experimental/ai/model-prices \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /api/experimental/ai/model-prices`

Experimental: this endpoint is subject to change.

### Example responses

> 200 Response

```json
[
  {
    "cache_read_price": 0,
    "cache_write_price": 0,
    "created_at": "2019-08-24T14:15:22Z",
    "input_price": 0,
    "model": "string",
    "output_price": 0,
    "provider": "string",
    "updated_at": "2019-08-24T14:15:22Z"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                            |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.AIModelPrice](schemas.md#codersdkaimodelprice) |

<h3 id="upsert-ai-model-prices-responseschema">Response Schema</h3>

Status Code **200**

| Name                  | Type              | Required | Restrictions | Description |
|-----------------------|-------------------|----------|--------------|-------------|
| `[array item]`        | array             | false    |              |             |
| `» cache_read_price`  | integer           | false    |              |             |
| `» cache_write_price` | integer           | false    |              |             |
| `» created_at`        | string(date-time) | false    |              |             |
| `» input_price`       | integer           | false    |              |             |
| `» model`             | string            | false    |              |             |
| `» output_price`      | integer           | false    |              |             |
| `» provider`          | string            | false    |              |             |
| `» updated_at`        | string(date-time) | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
