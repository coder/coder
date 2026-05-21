# Provisioning

## Get provisioner daemons

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/provisionerdaemons \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/organizations/{organization}/provisionerdaemons`

### Parameters

| Name           | In    | Type         | Required | Description                                                                        |
|----------------|-------|--------------|----------|------------------------------------------------------------------------------------|
| `organization` | path  | string(uuid) | true     | Organization ID                                                                    |
| `limit`        | query | integer      | false    | Page limit                                                                         |
| `ids`          | query | array(uuid)  | false    | Filter results by job IDs                                                          |
| `status`       | query | string       | false    | Filter results by status                                                           |
| `tags`         | query | object       | false    | Provisioner tags to filter by (JSON of the form {'tag1':'value1','tag2':'value2'}) |

#### Enumerated Values

| Parameter | Value(s)                                                                        |
|-----------|---------------------------------------------------------------------------------|
| `status`  | `canceled`, `canceling`, `failed`, `pending`, `running`, `succeeded`, `unknown` |

### Example responses

> 200 Response

```json
[
  {
    "api_version": "string",
    "created_at": "2019-08-24T14:15:22Z",
    "current_job": {
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "status": "pending",
      "template_display_name": "string",
      "template_icon": "string",
      "template_name": "string"
    },
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "key_id": "1e779c8a-6786-4c89-b7c3-a6666f5fd6b5",
    "key_name": "string",
    "last_seen_at": "2019-08-24T14:15:22Z",
    "name": "string",
    "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
    "previous_job": {
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "status": "pending",
      "template_display_name": "string",
      "template_icon": "string",
      "template_name": "string"
    },
    "provisioners": [
      "string"
    ],
    "status": "offline",
    "tags": {
      "property1": "string",
      "property2": "string"
    },
    "version": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                      |
|--------|---------------------------------------------------------|-------------|-----------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.ProvisionerDaemon](schemas.md#codersdkprovisionerdaemon) |

<h3 id="get-provisioner-daemons-responseschema">Response Schema</h3>

Status Code **200**

| Name                       | Type                                                                           | Required | Restrictions | Description      |
|----------------------------|--------------------------------------------------------------------------------|----------|--------------|------------------|
| `[array item]`             | array                                                                          | false    |              |                  |
| `» api_version`            | string                                                                         | false    |              |                  |
| `» created_at`             | string(date-time)                                                              | false    |              |                  |
| `» current_job`            | [codersdk.ProvisionerDaemonJob](schemas.md#codersdkprovisionerdaemonjob)       | false    |              |                  |
| `»» id`                    | string(uuid)                                                                   | false    |              |                  |
| `»» status`                | [codersdk.ProvisionerJobStatus](schemas.md#codersdkprovisionerjobstatus)       | false    |              |                  |
| `»» template_display_name` | string                                                                         | false    |              |                  |
| `»» template_icon`         | string                                                                         | false    |              |                  |
| `»» template_name`         | string                                                                         | false    |              |                  |
| `» id`                     | string(uuid)                                                                   | false    |              |                  |
| `» key_id`                 | string(uuid)                                                                   | false    |              |                  |
| `» key_name`               | string                                                                         | false    |              | Optional fields. |
| `» last_seen_at`           | string(date-time)                                                              | false    |              |                  |
| `» name`                   | string                                                                         | false    |              |                  |
| `» organization_id`        | string(uuid)                                                                   | false    |              |                  |
| `» previous_job`           | [codersdk.ProvisionerDaemonJob](schemas.md#codersdkprovisionerdaemonjob)       | false    |              |                  |
| `» provisioners`           | array                                                                          | false    |              |                  |
| `» status`                 | [codersdk.ProvisionerDaemonStatus](schemas.md#codersdkprovisionerdaemonstatus) | false    |              |                  |
| `» tags`                   | object                                                                         | false    |              |                  |
| `»» [any property]`        | string                                                                         | false    |              |                  |
| `» version`                | string                                                                         | false    |              |                  |

#### Enumerated Values

| Property | Value(s)                                                                                        |
|----------|-------------------------------------------------------------------------------------------------|
| `status` | `busy`, `canceled`, `canceling`, `failed`, `idle`, `offline`, `pending`, `running`, `succeeded` |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
