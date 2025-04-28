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
|--------|------|--------------------------------------------------------------------|----------|---------------------|
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
|--------|--------------------------------------------------------------|-------------|------------------------------------------------|
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
|--------|--------------------------------------------------------------|-------------|--------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.Response](schemas.md#codersdkresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get organizations

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations`

### Example responses

> 200 Response

```json
[
  {
    "created_at": "2019-08-24T14:15:22Z",
    "description": "string",
    "display_name": "string",
    "icon": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "is_default": true,
    "name": "string",
    "updated_at": "2019-08-24T14:15:22Z"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                            |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.Organization](schemas.md#codersdkorganization) |

<h3 id="get-organizations-responseschema">Response Schema</h3>

Status Code **200**

| Name             | Type              | Required | Restrictions | Description |
|------------------|-------------------|----------|--------------|-------------|
| `[array item]`   | array             | false    |              |             |
| `» created_at`   | string(date-time) | true     |              |             |
| `» description`  | string            | false    |              |             |
| `» display_name` | string            | false    |              |             |
| `» icon`         | string            | false    |              |             |
| `» id`           | string(uuid)      | true     |              |             |
| `» is_default`   | boolean           | true     |              |             |
| `» name`         | string            | false    |              |             |
| `» updated_at`   | string(date-time) | true     |              |             |

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
  "description": "string",
  "display_name": "string",
  "icon": "string",
  "name": "string"
}
```

### Parameters

| Name   | In   | Type                                                                               | Required | Description                 |
|--------|------|------------------------------------------------------------------------------------|----------|-----------------------------|
| `body` | body | [codersdk.CreateOrganizationRequest](schemas.md#codersdkcreateorganizationrequest) | true     | Create organization request |

### Example responses

> 201 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "description": "string",
  "display_name": "string",
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "is_default": true,
  "name": "string",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                   |
|--------|--------------------------------------------------------------|-------------|----------------------------------------------------------|
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
|----------------|------|--------------|----------|-----------------|
| `organization` | path | string(uuid) | true     | Organization ID |

### Example responses

> 200 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "description": "string",
  "display_name": "string",
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "is_default": true,
  "name": "string",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                   |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Organization](schemas.md#codersdkorganization) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete organization

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/organizations/{organization} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /organizations/{organization}`

### Parameters

| Name           | In   | Type   | Required | Description             |
|----------------|------|--------|----------|-------------------------|
| `organization` | path | string | true     | Organization ID or name |

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
|--------|---------------------------------------------------------|-------------|--------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Response](schemas.md#codersdkresponse) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update organization

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/organizations/{organization} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /organizations/{organization}`

> Body parameter

```json
{
  "description": "string",
  "display_name": "string",
  "icon": "string",
  "name": "string"
}
```

### Parameters

| Name           | In   | Type                                                                               | Required | Description                |
|----------------|------|------------------------------------------------------------------------------------|----------|----------------------------|
| `organization` | path | string                                                                             | true     | Organization ID or name    |
| `body`         | body | [codersdk.UpdateOrganizationRequest](schemas.md#codersdkupdateorganizationrequest) | true     | Patch organization request |

### Example responses

> 200 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "description": "string",
  "display_name": "string",
  "icon": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "is_default": true,
  "name": "string",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                   |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.Organization](schemas.md#codersdkorganization) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get provisioner jobs

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/provisionerjobs \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/provisionerjobs`

### Parameters

| Name           | In    | Type         | Required | Description                                                                        |
|----------------|-------|--------------|----------|------------------------------------------------------------------------------------|
| `organization` | path  | string(uuid) | true     | Organization ID                                                                    |
| `limit`        | query | integer      | false    | Page limit                                                                         |
| `ids`          | query | array(uuid)  | false    | Filter results by job IDs                                                          |
| `status`       | query | string       | false    | Filter results by status                                                           |
| `tags`         | query | object       | false    | Provisioner tags to filter by (JSON of the form {'tag1':'value1','tag2':'value2'}) |

#### Enumerated Values

| Parameter | Value       |
|-----------|-------------|
| `status`  | `pending`   |
| `status`  | `running`   |
| `status`  | `succeeded` |
| `status`  | `canceling` |
| `status`  | `canceled`  |
| `status`  | `failed`    |
| `status`  | `unknown`   |
| `status`  | `pending`   |
| `status`  | `running`   |
| `status`  | `succeeded` |
| `status`  | `canceling` |
| `status`  | `canceled`  |
| `status`  | `failed`    |

### Example responses

> 200 Response

```json
[
  {
    "available_workers": [
      "497f6eca-6276-4993-bfeb-53cbbbba6f08"
    ],
    "canceled_at": "2019-08-24T14:15:22Z",
    "completed_at": "2019-08-24T14:15:22Z",
    "created_at": "2019-08-24T14:15:22Z",
    "error": "string",
    "error_code": "REQUIRED_TEMPLATE_VARIABLES",
    "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "input": {
      "error": "string",
      "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
      "workspace_build_id": "badaf2eb-96c5-4050-9f1d-db2d39ca5478"
    },
    "metadata": {
      "template_display_name": "string",
      "template_icon": "string",
      "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
      "template_name": "string",
      "template_version_name": "string",
      "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
      "workspace_name": "string"
    },
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "queue_position": 0,
    "queue_size": 0,
    "started_at": "2019-08-24T14:15:22Z",
    "status": "pending",
    "tags": {
      "property1": "string",
      "property2": "string"
    },
    "type": "template_version_import",
    "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                |
|--------|---------------------------------------------------------|-------------|-----------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.ProvisionerJob](schemas.md#codersdkprovisionerjob) |

<h3 id="get-provisioner-jobs-responseschema">Response Schema</h3>

Status Code **200**

| Name                       | Type                                                                         | Required | Restrictions | Description |
|----------------------------|------------------------------------------------------------------------------|----------|--------------|-------------|
| `[array item]`             | array                                                                        | false    |              |             |
| `» available_workers`      | array                                                                        | false    |              |             |
| `» canceled_at`            | string(date-time)                                                            | false    |              |             |
| `» completed_at`           | string(date-time)                                                            | false    |              |             |
| `» created_at`             | string(date-time)                                                            | false    |              |             |
| `» error`                  | string                                                                       | false    |              |             |
| `» error_code`             | [codersdk.JobErrorCode](schemas.md#codersdkjoberrorcode)                     | false    |              |             |
| `» file_id`                | string(uuid)                                                                 | false    |              |             |
| `» id`                     | string(uuid)                                                                 | false    |              |             |
| `» input`                  | [codersdk.ProvisionerJobInput](schemas.md#codersdkprovisionerjobinput)       | false    |              |             |
| `»» error`                 | string                                                                       | false    |              |             |
| `»» template_version_id`   | string(uuid)                                                                 | false    |              |             |
| `»» workspace_build_id`    | string(uuid)                                                                 | false    |              |             |
| `» metadata`               | [codersdk.ProvisionerJobMetadata](schemas.md#codersdkprovisionerjobmetadata) | false    |              |             |
| `»» template_display_name` | string                                                                       | false    |              |             |
| `»» template_icon`         | string                                                                       | false    |              |             |
| `»» template_id`           | string(uuid)                                                                 | false    |              |             |
| `»» template_name`         | string                                                                       | false    |              |             |
| `»» template_version_name` | string                                                                       | false    |              |             |
| `»» workspace_id`          | string(uuid)                                                                 | false    |              |             |
| `»» workspace_name`        | string                                                                       | false    |              |             |
| `» organization_id`        | string(uuid)                                                                 | false    |              |             |
| `» queue_position`         | integer                                                                      | false    |              |             |
| `» queue_size`             | integer                                                                      | false    |              |             |
| `» started_at`             | string(date-time)                                                            | false    |              |             |
| `» status`                 | [codersdk.ProvisionerJobStatus](schemas.md#codersdkprovisionerjobstatus)     | false    |              |             |
| `» tags`                   | object                                                                       | false    |              |             |
| `»» [any property]`        | string                                                                       | false    |              |             |
| `» type`                   | [codersdk.ProvisionerJobType](schemas.md#codersdkprovisionerjobtype)         | false    |              |             |
| `» worker_id`              | string(uuid)                                                                 | false    |              |             |

#### Enumerated Values

| Property     | Value                         |
|--------------|-------------------------------|
| `error_code` | `REQUIRED_TEMPLATE_VARIABLES` |
| `status`     | `pending`                     |
| `status`     | `running`                     |
| `status`     | `succeeded`                   |
| `status`     | `canceling`                   |
| `status`     | `canceled`                    |
| `status`     | `failed`                      |
| `type`       | `template_version_import`     |
| `type`       | `workspace_build`             |
| `type`       | `template_version_dry_run`    |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get provisioner job

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/provisionerjobs/{job} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/provisionerjobs/{job}`

### Parameters

| Name           | In   | Type         | Required | Description     |
|----------------|------|--------------|----------|-----------------|
| `organization` | path | string(uuid) | true     | Organization ID |
| `job`          | path | string(uuid) | true     | Job ID          |

### Example responses

> 200 Response

```json
{
  "available_workers": [
    "497f6eca-6276-4993-bfeb-53cbbbba6f08"
  ],
  "canceled_at": "2019-08-24T14:15:22Z",
  "completed_at": "2019-08-24T14:15:22Z",
  "created_at": "2019-08-24T14:15:22Z",
  "error": "string",
  "error_code": "REQUIRED_TEMPLATE_VARIABLES",
  "file_id": "8a0cfb4f-ddc9-436d-91bb-75133c583767",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "input": {
    "error": "string",
    "template_version_id": "0ba39c92-1f1b-4c32-aa3e-9925d7713eb1",
    "workspace_build_id": "badaf2eb-96c5-4050-9f1d-db2d39ca5478"
  },
  "metadata": {
    "template_display_name": "string",
    "template_icon": "string",
    "template_id": "c6d67e98-83ea-49f0-8812-e4abae2b68bc",
    "template_name": "string",
    "template_version_name": "string",
    "workspace_id": "0967198e-ec7b-4c6b-b4d3-f71244cadbe9",
    "workspace_name": "string"
  },
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "queue_position": 0,
  "queue_size": 0,
  "started_at": "2019-08-24T14:15:22Z",
  "status": "pending",
  "tags": {
    "property1": "string",
    "property2": "string"
  },
  "type": "template_version_import",
  "worker_id": "ae5fa6f7-c55b-40c1-b40a-b36ac467652b"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                       |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.ProvisionerJob](schemas.md#codersdkprovisionerjob) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
