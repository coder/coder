# Chats

## Upload a chat file

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/chats/files?organization=497f6eca-6276-4993-bfeb-53cbbbba6f08 \
  -H 'Accept: application/json' \
  -H 'Content-Type: string' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /chats/files`

### Parameters

| Name           | In     | Type         | Required | Description                                                                       |
|----------------|--------|--------------|----------|-----------------------------------------------------------------------------------|
| `Content-Type` | header | string       | true     | Content-Type must be an image type (image/png, image/jpeg, image/gif, image/webp) |
| `organization` | query  | string(uuid) | true     | Organization ID                                                                   |

### Example responses

> 201 Response

```json
{
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08"
}
```

### Responses

| Status | Meaning                                                                    | Description              | Schema                                                                       |
|--------|----------------------------------------------------------------------------|--------------------------|------------------------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)               | Created                  | [codersdk.UploadChatFileResponse](schemas.md#codersdkuploadchatfileresponse) |
| 400    | [Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)           | Bad Request              | [codersdk.Response](schemas.md#codersdkresponse)                             |
| 401    | [Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)            | Unauthorized             | [codersdk.Response](schemas.md#codersdkresponse)                             |
| 413    | [Payload Too Large](https://tools.ietf.org/html/rfc7231#section-6.5.11)    | Request Entity Too Large | [codersdk.Response](schemas.md#codersdkresponse)                             |
| 500    | [Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1) | Internal Server Error    | [codersdk.Response](schemas.md#codersdkresponse)                             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get a chat file

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/chats/files/{file} \
  -H 'Accept: */*' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /chats/files/{file}`

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `file` | path | string(uuid) | true     | File ID     |

### Example responses

> 400 Response

### Responses

| Status | Meaning                                                                    | Description           | Schema                                           |
|--------|----------------------------------------------------------------------------|-----------------------|--------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)                    | OK                    |                                                  |
| 400    | [Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)           | Bad Request           | [codersdk.Response](schemas.md#codersdkresponse) |
| 401    | [Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)            | Unauthorized          | [codersdk.Response](schemas.md#codersdkresponse) |
| 404    | [Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)             | Not Found             | [codersdk.Response](schemas.md#codersdkresponse) |
| 500    | [Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1) | Internal Server Error | [codersdk.Response](schemas.md#codersdkresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Archive a chat

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/chats/{chat}/archive

```

`POST /chats/{chat}/archive`

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

## Watch git changes for a chat

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/chats/{chat}/git/watch \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /chats/{chat}/git/watch`

### Parameters

| Name   | In   | Type         | Required | Description |
|--------|------|--------------|----------|-------------|
| `chat` | path | string(uuid) | true     | Chat ID     |

### Responses

| Status | Meaning                                                                  | Description         | Schema |
|--------|--------------------------------------------------------------------------|---------------------|--------|
| 101    | [Switching Protocols](https://tools.ietf.org/html/rfc7231#section-6.2.2) | Switching Protocols |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Unarchive a chat

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/chats/{chat}/unarchive

```

`POST /chats/{chat}/unarchive`

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |
