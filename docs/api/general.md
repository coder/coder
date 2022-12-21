# General

> This page is incomplete, stay tuned.

## API root handler

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/ \
  -H 'Accept: application/json'
```

`GET /`

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
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Response](schemas.md#codersdkresponse) |

undefined

## Build info

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/buildinfo \
  -H 'Accept: application/json'
```

`GET /buildinfo`

### Example responses

> 200 Response

```json
{
  "external_url": "string",
  "version": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                             |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.BuildInfoResponse](schemas.md#codersdkbuildinforesponse) |

undefined

## Report CSP violations

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/csp/reports \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /csp/reports`

> Body parameter

```json
{
  "csp-report": {}
}
```

### Parameters

| Name   | In   | Type                                                 | Required | Description      |
| ------ | ---- | ---------------------------------------------------- | -------- | ---------------- |
| `body` | body | [coderd.cspViolation](schemas.md#coderdcspviolation) | true     | Violation report |

### Responses

| Status | Meaning                                                 | Description | Schema |
| ------ | ------------------------------------------------------- | ----------- | ------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

## Update check

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/updatecheck \
  -H 'Accept: application/json'
```

`GET /updatecheck`

### Example responses

> 200 Response

```json
{
  "current": true,
  "url": "string",
  "version": "string"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                 |
| ------ | ------------------------------------------------------- | ----------- | ---------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.UpdateCheckResponse](schemas.md#codersdkupdatecheckresponse) |

undefined
