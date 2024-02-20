# Organizations

## Add new license

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/licenses \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /licenses`

> Body parameter

```json
{
  "license": "string"
}
```

### Parameters

| Name   | In   | Type                                                               | Required | Description         |
| ------ | ---- | ------------------------------------------------------------------ | -------- | ------------------- |
| `body` | body | [codersdk.AddLicenseRequest](schemas.md#codersdkaddlicenserequest) | true     | Add license request |

### Example responses

> 201 Response

```json
{
  "claims": {},
  "id": 0,
  "uploaded_at": "2019-08-24T14:15:22Z",
  "uuid": "095be615-a8ad-4c33-8e9c-c7612fbf6c9f"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                         |
| ------ | ------------------------------------------------------------ | ----------- | ---------------------------------------------- |
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.License](schemas.md#codersdklicense) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update license entitlements

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/licenses/refresh-entitlements \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /licenses/refresh-entitlements`

### Example responses

> 201 Response

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

| Status | Meaning                                                      | Description | Schema                                           |
| ------ | ------------------------------------------------------------ | ----------- | ------------------------------------------------ |
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.Response](schemas.md#codersdkresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

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
  "is_default": true,
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
  "is_default": true,
  "name": "string",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                   |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Organization](schemas.md#codersdkorganization) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
