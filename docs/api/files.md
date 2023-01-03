# Files

> This page is incomplete, stay tuned.

## Upload file

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/files \
  -H 'Accept: application/json' \
  -H 'Content-Type: application/x-tar' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /files`

<<<<<<< HEAD
Notice: Swagger 2.0 doesn't support file upload with a `content-type` different than `application/x-www-form-urlencoded`.

=======
>>>>>>> main
> Body parameter

```yaml
file: string
```

### Parameters

<<<<<<< HEAD
| Name           | In     | Type           | Required | Description                              |
| -------------- | ------ | -------------- | -------- | ---------------------------------------- |
| `Content-Type` | header | string         | true     | Content-Type must be `application/x-tar` |
| `body`         | body   | object         | true     |                                          |
| `» file`       | body   | string(binary) | true     | File to be uploaded                      |
=======
| Name           | In     | Type   | Required | Description                              |
| -------------- | ------ | ------ | -------- | ---------------------------------------- |
| `Content-Type` | header | string | true     | Content-Type must be `application/x-tar` |
| `body`         | body   | object | true     |                                          |
| `» file`       | body   | binary | true     | File to be uploaded                      |
>>>>>>> main

### Example responses

> 201 Response

```json
{
<<<<<<< HEAD
  "hash": "string"
=======
  "hash": "19686d84-b10d-4f90-b18e-84fd3fa038fd"
>>>>>>> main
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                       |
| ------ | ------------------------------------------------------------ | ----------- | ------------------------------------------------------------ |
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.UploadResponse](schemas.md#codersdkuploadresponse) |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.

## Get file by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/files/{fileID} \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /files/{fileID}`

### Parameters

| Name     | In   | Type         | Required | Description |
| -------- | ---- | ------------ | -------- | ----------- |
| `fileID` | path | string(uuid) | true     | File ID     |

### Responses

| Status | Meaning                                                 | Description | Schema |
| ------ | ------------------------------------------------------- | ----------- | ------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          |        |

To perform this operation, you must be authenticated by means of one of the following methods: **CoderSessionToken**.
