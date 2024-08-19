# Frobulator

## Get all frobulators

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/frobulators \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /frobulators`

### Example responses

> 200 Response

```json
[
	{
		"id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
		"model_number": "string",
		"user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
	}
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                        |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Frobulator](schemas.md#codersdkfrobulator) |

<h3 id="get-all-frobulators-responseschema">Response Schema</h3>

Status Code **200**

| Name             | Type         | Required | Restrictions | Description |
| ---------------- | ------------ | -------- | ------------ | ----------- |
| `[array item]`   | array        | false    |              |             |
| `» id`           | string(uuid) | false    |              |             |
| `» model_number` | string       | false    |              |             |
| `» user_id`      | string(uuid) | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get user frobulators

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/frobulators/{user} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /frobulators/{user}`

### Parameters

| Name   | In   | Type   | Required | Description          |
| ------ | ---- | ------ | -------- | -------------------- |
| `user` | path | string | true     | User ID, name, or me |

### Example responses

> 200 Response

```json
[
	{
		"id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
		"model_number": "string",
		"user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
	}
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                        |
| ------ | ------------------------------------------------------- | ----------- | ------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Frobulator](schemas.md#codersdkfrobulator) |

<h3 id="get-user-frobulators-responseschema">Response Schema</h3>

Status Code **200**

| Name             | Type         | Required | Restrictions | Description |
| ---------------- | ------------ | -------- | ------------ | ----------- |
| `[array item]`   | array        | false    |              |             |
| `» id`           | string(uuid) | false    |              |             |
| `» model_number` | string       | false    |              |             |
| `» user_id`      | string(uuid) | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Post frobulator

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/frobulators/{user} \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /frobulators/{user}`

> Body parameter

```json
{
	"model_number": "string"
}
```

### Parameters

| Name   | In   | Type                                                                           | Required | Description               |
| ------ | ---- | ------------------------------------------------------------------------------ | -------- | ------------------------- |
| `user` | path | string                                                                         | true     | User ID, name, or me      |
| `body` | body | [codersdk.InsertFrobulatorRequest](schemas.md#codersdkinsertfrobulatorrequest) | true     | Insert Frobulator request |

### Responses

| Status | Meaning                                                 | Description       | Schema |
| ------ | ------------------------------------------------------- | ----------------- | ------ |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | New frobulator ID |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
