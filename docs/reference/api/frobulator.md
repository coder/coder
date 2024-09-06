# Frobulator

## Get frobulators

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/members/{user}/frobulators \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/members/{user}/frobulators`

### Parameters

| Name           | In   | Type   | Required | Description          |
| -------------- | ---- | ------ | -------- | -------------------- |
| `organization` | path | string | true     | Organization ID      |
| `user`         | path | string | true     | User ID, name, or me |

### Example responses

> 200 Response

```json
[
	{
		"id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
		"model_number": "string",
		"org_id": "a40f5d1f-d889-42e9-94ea-b9b33585fc6b",
		"user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
	}
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                        |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Frobulator](schemas.md#codersdkfrobulator) |

<h3 id="get-frobulators-responseschema">Response Schema</h3>

Status Code **200**

| Name             | Type         | Required | Restrictions | Description |
| ---------------- | ------------ | -------- | ------------ | ----------- |
| `[array item]`   | array        | false    |              |             |
| `» id`           | string(uuid) | false    |              |             |
| `» model_number` | string       | false    |              |             |
| `» org_id`       | string(uuid) | false    |              |             |
| `» user_id`      | string(uuid) | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Post frobulator

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/organizations/{organization}/members/{user}/frobulators \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /organizations/{organization}/members/{user}/frobulators`

> Body parameter

```json
{
	"model_number": "string"
}
```

### Parameters

| Name           | In   | Type                                                                           | Required | Description               |
| -------------- | ---- | ------------------------------------------------------------------------------ | -------- | ------------------------- |
| `organization` | path | string                                                                         | true     | Organization ID           |
| `user`         | path | string                                                                         | true     | User ID, name, or me      |
| `body`         | body | [codersdk.InsertFrobulatorRequest](schemas.md#codersdkinsertfrobulatorrequest) | true     | Insert Frobulator request |

### Responses

| Status | Meaning                                                 | Description       | Schema |
| ------ | ------------------------------------------------------- | ----------------- | ------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | New frobulator ID |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete frobulator

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/organizations/{organization}/members/{user}/frobulators/{id} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /organizations/{organization}/members/{user}/frobulators/{id}`

### Parameters

| Name           | In   | Type   | Required | Description          |
| -------------- | ---- | ------ | -------- | -------------------- |
| `organization` | path | string | true     | Organization ID      |
| `user`         | path | string | true     | User ID, name, or me |
| `id`           | path | string | true     | Frobulator ID        |

### Responses

| Status | Meaning                                                         | Description | Schema |
| ------ | --------------------------------------------------------------- | ----------- | ------ |
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
