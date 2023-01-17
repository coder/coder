# Parameters

## Get parameters

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/parameters/{scope}/{id} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /parameters/{scope}/{id}`

### Parameters

| Name    | In   | Type         | Required | Description |
| ------- | ---- | ------------ | -------- | ----------- |
| `scope` | path | string       | true     | Scope       |
| `id`    | path | string(uuid) | true     | ID          |

#### Enumerated Values

| Parameter | Value        |
| --------- | ------------ |
| `scope`   | `template`   |
| `scope`   | `workspace`  |
| `scope`   | `import_job` |

### Example responses

> 200 Response

```json
[
  {
    "created_at": "2019-08-24T14:15:22Z",
    "destination_scheme": "none",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "name": "string",
    "scope": "template",
    "scope_id": "5d3fe357-12dd-4f62-b004-6d1fb3b8454f",
    "source_scheme": "none",
    "updated_at": "2019-08-24T14:15:22Z"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                      |
| ------ | ------------------------------------------------------- | ----------- | ----------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Parameter](schemas.md#codersdkparameter) |

<h3 id="get-parameters-responseschema">Response Schema</h3>

Status Code **200**

| Name                   | Type                                                                                 | Required | Restrictions | Description                                       |
| ---------------------- | ------------------------------------------------------------------------------------ | -------- | ------------ | ------------------------------------------------- |
| `[array item]`         | array                                                                                | false    |              | [Parameter represents a set value for the scope.] |
| `» created_at`         | string(date-time)                                                                    | false    |              |                                                   |
| `» destination_scheme` | [codersdk.ParameterDestinationScheme](schemas.md#codersdkparameterdestinationscheme) | false    |              |                                                   |
| `» id`                 | string(uuid)                                                                         | false    |              |                                                   |
| `» name`               | string                                                                               | false    |              |                                                   |
| `» scope`              | [codersdk.ParameterScope](schemas.md#codersdkparameterscope)                         | false    |              |                                                   |
| `» scope_id`           | string(uuid)                                                                         | false    |              |                                                   |
| `» source_scheme`      | [codersdk.ParameterSourceScheme](schemas.md#codersdkparametersourcescheme)           | false    |              |                                                   |
| `» updated_at`         | string(date-time)                                                                    | false    |              |                                                   |

#### Enumerated Values

| Property             | Value                  |
| -------------------- | ---------------------- |
| `destination_scheme` | `none`                 |
| `destination_scheme` | `environment_variable` |
| `destination_scheme` | `provisioner_variable` |
| `scope`              | `template`             |
| `scope`              | `workspace`            |
| `scope`              | `import_job`           |
| `source_scheme`      | `none`                 |
| `source_scheme`      | `data`                 |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create parameter

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/parameters/{scope}/{id} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /parameters/{scope}/{id}`

> Body parameter

```json
{
  "copy_from_parameter": "000e07d6-021d-446c-be14-48a9c20bca0b",
  "destination_scheme": "none",
  "name": "string",
  "source_scheme": "none",
  "source_value": "string"
}
```

### Parameters

| Name    | In   | Type                                                                         | Required | Description       |
| ------- | ---- | ---------------------------------------------------------------------------- | -------- | ----------------- |
| `scope` | path | string                                                                       | true     | Scope             |
| `id`    | path | string(uuid)                                                                 | true     | ID                |
| `body`  | body | [codersdk.CreateParameterRequest](schemas.md#codersdkcreateparameterrequest) | true     | Parameter request |

#### Enumerated Values

| Parameter | Value        |
| --------- | ------------ |
| `scope`   | `template`   |
| `scope`   | `workspace`  |
| `scope`   | `import_job` |

### Example responses

> 201 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "destination_scheme": "none",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "scope": "template",
  "scope_id": "5d3fe357-12dd-4f62-b004-6d1fb3b8454f",
  "source_scheme": "none",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                             |
| ------ | ------------------------------------------------------------ | ----------- | -------------------------------------------------- |
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.Parameter](schemas.md#codersdkparameter) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete parameter

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/parameters/{scope}/{id}/{name} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /parameters/{scope}/{id}/{name}`

### Parameters

| Name    | In   | Type         | Required | Description |
| ------- | ---- | ------------ | -------- | ----------- |
| `scope` | path | string       | true     | Scope       |
| `id`    | path | string(uuid) | true     | ID          |
| `name`  | path | string       | true     | Name        |

#### Enumerated Values

| Parameter | Value        |
| --------- | ------------ |
| `scope`   | `template`   |
| `scope`   | `workspace`  |
| `scope`   | `import_job` |

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

To perform this operation, you must be authenticated. [Learn more](authentication.md).
