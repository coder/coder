# Organizations

> This page is incomplete, stay tuned.

## Create organization

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/organizations \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /organizations`

> Body parameter

```json
{
  "name": "string"
}
```

### Parameters

| Name   | In   | Type                                                                               | Required | Description                 |
| ------ | ---- | ---------------------------------------------------------------------------------- | -------- | --------------------------- |
| `body` | body | [codersdk.CreateOrganizationRequest](schemas.md#codersdkcreateorganizationrequest) | true     | Create organization request |

### Example responses

> 201 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                   |
| ------ | ------------------------------------------------------------ | ----------- | -------------------------------------------------------- |
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.Organization](schemas.md#codersdkorganization) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get organization by ID

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}`

### Parameters

| Name           | In   | Type         | Required | Description     |
| -------------- | ---- | ------------ | -------- | --------------- |
| `organization` | path | string(uuid) | true     | Organization ID |

### Example responses

> 200 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                   |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Organization](schemas.md#codersdkorganization) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
