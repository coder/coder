# AI Gateway

## List AI gateway guardrails

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/aibridge/guardrails \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/aibridge/guardrails`

### Example responses

> 200 Response

```json
[
  {
    "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
    "adapter_type": "string",
    "created_at": "2019-08-24T14:15:22Z",
    "display_name": "string",
    "enabled": true,
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "name": "string",
    "updated_at": "2019-08-24T14:15:22Z",
    "versions": [
      {
        "config": [
          0
        ],
        "created_at": "2019-08-24T14:15:22Z",
        "created_by": "ee824cad-d7a6-4f48-87dc-e8461a9201c4",
        "description": "string",
        "guardrail_id": "5ea4ad06-0022-46ca-b5a6-3795e32e6aa8",
        "has_credential": true,
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "version_number": 0
      }
    ]
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                        |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.AIGatewayGuardrail](schemas.md#codersdkaigatewayguardrail) |

<h3 id="list-ai-gateway-guardrails-responseschema">Response Schema</h3>

Status Code **200**

| Name                  | Type              | Required | Restrictions | Description |
|-----------------------|-------------------|----------|--------------|-------------|
| `[array item]`        | array             | false    |              |             |
| `» active_version_id` | string(uuid)      | false    |              |             |
| `» adapter_type`      | string            | false    |              |             |
| `» created_at`        | string(date-time) | false    |              |             |
| `» display_name`      | string            | false    |              |             |
| `» enabled`           | boolean           | false    |              |             |
| `» id`                | string(uuid)      | false    |              |             |
| `» name`              | string            | false    |              |             |
| `» updated_at`        | string(date-time) | false    |              |             |
| `» versions`          | array             | false    |              |             |
| `»» config`           | array             | false    |              |             |
| `»» created_at`       | string(date-time) | false    |              |             |
| `»» created_by`       | string(uuid)      | false    |              |             |
| `»» description`      | string            | false    |              |             |
| `»» guardrail_id`     | string(uuid)      | false    |              |             |
| `»» has_credential`   | boolean           | false    |              |             |
| `»» id`               | string(uuid)      | false    |              |             |
| `»» version_number`   | integer           | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create an AI gateway guardrail

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/aibridge/guardrails \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/v2/aibridge/guardrails`

> Body parameter

```json
{
  "adapter_type": "string",
  "config": [
    0
  ],
  "credential": "string",
  "description": "string",
  "display_name": "string",
  "name": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                           | Required | Description              |
|--------|------|------------------------------------------------------------------------------------------------|----------|--------------------------|
| `body` | body | [codersdk.CreateAIGatewayGuardrailRequest](schemas.md#codersdkcreateaigatewayguardrailrequest) | true     | Create guardrail request |

### Example responses

> 201 Response

```json
{
  "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
  "adapter_type": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "display_name": "string",
  "enabled": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "versions": [
    {
      "config": [
        0
      ],
      "created_at": "2019-08-24T14:15:22Z",
      "created_by": "ee824cad-d7a6-4f48-87dc-e8461a9201c4",
      "description": "string",
      "guardrail_id": "5ea4ad06-0022-46ca-b5a6-3795e32e6aa8",
      "has_credential": true,
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "version_number": 0
    }
  ]
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                               |
|--------|--------------------------------------------------------------|-------------|----------------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.AIGatewayGuardrail](schemas.md#codersdkaigatewayguardrail) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get an AI gateway guardrail

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/aibridge/guardrails/{id} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/aibridge/guardrails/{id}`

### Parameters

| Name | In   | Type         | Required | Description  |
|------|------|--------------|----------|--------------|
| `id` | path | string(uuid) | true     | Guardrail ID |

### Example responses

> 200 Response

```json
{
  "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
  "adapter_type": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "display_name": "string",
  "enabled": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "versions": [
    {
      "config": [
        0
      ],
      "created_at": "2019-08-24T14:15:22Z",
      "created_by": "ee824cad-d7a6-4f48-87dc-e8461a9201c4",
      "description": "string",
      "guardrail_id": "5ea4ad06-0022-46ca-b5a6-3795e32e6aa8",
      "has_credential": true,
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "version_number": 0
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                               |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AIGatewayGuardrail](schemas.md#codersdkaigatewayguardrail) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete an AI gateway guardrail

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/aibridge/guardrails/{id} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /api/v2/aibridge/guardrails/{id}`

### Parameters

| Name | In   | Type         | Required | Description  |
|------|------|--------------|----------|--------------|
| `id` | path | string(uuid) | true     | Guardrail ID |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update an AI gateway guardrail

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/aibridge/guardrails/{id} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /api/v2/aibridge/guardrails/{id}`

> Body parameter

```json
{
  "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
  "display_name": "string",
  "enabled": true,
  "promote": true
}
```

### Parameters

| Name   | In   | Type                                                                                           | Required | Description              |
|--------|------|------------------------------------------------------------------------------------------------|----------|--------------------------|
| `id`   | path | string(uuid)                                                                                   | true     | Guardrail ID             |
| `body` | body | [codersdk.UpdateAIGatewayGuardrailRequest](schemas.md#codersdkupdateaigatewayguardrailrequest) | true     | Update guardrail request |

### Example responses

> 200 Response

```json
{
  "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
  "adapter_type": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "display_name": "string",
  "enabled": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "name": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "versions": [
    {
      "config": [
        0
      ],
      "created_at": "2019-08-24T14:15:22Z",
      "created_by": "ee824cad-d7a6-4f48-87dc-e8461a9201c4",
      "description": "string",
      "guardrail_id": "5ea4ad06-0022-46ca-b5a6-3795e32e6aa8",
      "has_credential": true,
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "version_number": 0
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                               |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AIGatewayGuardrail](schemas.md#codersdkaigatewayguardrail) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create an AI gateway guardrail version

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/aibridge/guardrails/{id}/versions \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/v2/aibridge/guardrails/{id}/versions`

> Body parameter

```json
{
  "activate": true,
  "config": [
    0
  ],
  "credential": "string",
  "description": "string",
  "promote": true
}
```

### Parameters

| Name   | In   | Type                                                                                                         | Required | Description            |
|--------|------|--------------------------------------------------------------------------------------------------------------|----------|------------------------|
| `id`   | path | string(uuid)                                                                                                 | true     | Guardrail ID           |
| `body` | body | [codersdk.CreateAIGatewayGuardrailVersionRequest](schemas.md#codersdkcreateaigatewayguardrailversionrequest) | true     | Create version request |

### Example responses

> 201 Response

```json
{
  "config": [
    0
  ],
  "created_at": "2019-08-24T14:15:22Z",
  "created_by": "ee824cad-d7a6-4f48-87dc-e8461a9201c4",
  "description": "string",
  "guardrail_id": "5ea4ad06-0022-46ca-b5a6-3795e32e6aa8",
  "has_credential": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "version_number": 0
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                                             |
|--------|--------------------------------------------------------------|-------------|------------------------------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.AIGatewayGuardrailVersion](schemas.md#codersdkaigatewayguardrailversion) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List AI gateway pipelines

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/aibridge/pipelines \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/aibridge/pipelines`

### Example responses

> 200 Response

```json
[
  {
    "active_version": {
      "created_at": "2019-08-24T14:15:22Z",
      "guardrails": [
        {
          "enabled": true,
          "fail_mode": "fail_open",
          "guardrail_version_id": "679d05fc-2e89-4d23-9f9d-93315fd86dfd",
          "hook": "pre_auth",
          "network_timeout_ms": 0
        }
      ],
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "pipeline_id": "ec036e81-7903-4e4d-bbfa-ac8516341cf0",
      "policies": [
        {
          "enabled": true,
          "fail_mode": "fail_open",
          "hook": "pre_auth",
          "kind": "annotate",
          "policy_version_id": "7cd41427-f4be-4006-ab17-5ead7f8f8446"
        }
      ],
      "version_number": 0
    },
    "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
    "created_at": "2019-08-24T14:15:22Z",
    "enabled": true,
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "latest_version": {
      "created_at": "2019-08-24T14:15:22Z",
      "guardrails": [
        {
          "enabled": true,
          "fail_mode": "fail_open",
          "guardrail_version_id": "679d05fc-2e89-4d23-9f9d-93315fd86dfd",
          "hook": "pre_auth",
          "network_timeout_ms": 0
        }
      ],
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "pipeline_id": "ec036e81-7903-4e4d-bbfa-ac8516341cf0",
      "policies": [
        {
          "enabled": true,
          "fail_mode": "fail_open",
          "hook": "pre_auth",
          "kind": "annotate",
          "policy_version_id": "7cd41427-f4be-4006-ab17-5ead7f8f8446"
        }
      ],
      "version_number": 0
    },
    "latest_version_id": "8562ca50-d99e-4ec5-9529-9c17b0fd462e",
    "latest_version_number": 0,
    "provider_id": "fe3d49af-4061-436b-ae60-f7044f252a44",
    "updated_at": "2019-08-24T14:15:22Z"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                      |
|--------|---------------------------------------------------------|-------------|-----------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.AIGatewayPipeline](schemas.md#codersdkaigatewaypipeline) |

<h3 id="list-ai-gateway-pipelines-responseschema">Response Schema</h3>

Status Code **200**

| Name                       | Type                                                                             | Required | Restrictions | Description                                                                                                                                                                                                                                                                                                                                                                                                                              |
|----------------------------|----------------------------------------------------------------------------------|----------|--------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `[array item]`             | array                                                                            | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `» active_version`         | [codersdk.AIGatewayPipelineVersion](schemas.md#codersdkaigatewaypipelineversion) | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `»» created_at`            | string(date-time)                                                                | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `»» guardrails`            | array                                                                            | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `»»» enabled`              | boolean                                                                          | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `»»» fail_mode`            | [codersdk.AIGatewayFailMode](schemas.md#codersdkaigatewayfailmode)               | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `»»» guardrail_version_id` | string(uuid)                                                                     | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `»»» hook`                 | [codersdk.AIGatewayHook](schemas.md#codersdkaigatewayhook)                       | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `»»» network_timeout_ms`   | integer                                                                          | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `»» id`                    | string(uuid)                                                                     | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `»» pipeline_id`           | string(uuid)                                                                     | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `»» policies`              | array                                                                            | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `»»» enabled`              | boolean                                                                          | false    |              | Enabled disables this policy within this pipeline without disabling it globally. Disabled members are excluded from the runtime snapshot.                                                                                                                                                                                                                                                                                                |
| `»»» fail_mode`            | [codersdk.AIGatewayFailMode](schemas.md#codersdkaigatewayfailmode)               | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `»»» hook`                 | [codersdk.AIGatewayHook](schemas.md#codersdkaigatewayhook)                       | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `»»» kind`                 | [codersdk.AIGatewayPolicyKind](schemas.md#codersdkaigatewaypolicykind)           | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `»»» policy_version_id`    | string(uuid)                                                                     | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `»» version_number`        | integer                                                                          | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `» active_version_id`      | string(uuid)                                                                     | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `» created_at`             | string(date-time)                                                                | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `» enabled`                | boolean                                                                          | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `» id`                     | string(uuid)                                                                     | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `» latest_version`         | [codersdk.AIGatewayPipelineVersion](schemas.md#codersdkaigatewaypipelineversion) | false    |              | Latest version is the tip version with its full membership (policies and guardrails). Editing a pipeline must base the new version on the tip, not the active version, so staged changes accumulate as one linear draft lineage; basing an edit on the active version would silently drop members added in an unpromoted draft.                                                                                                          |
| `»» created_at`            | string(date-time)                                                                | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `»» guardrails`            | array                                                                            | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `»» id`                    | string(uuid)                                                                     | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `»» pipeline_id`           | string(uuid)                                                                     | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `»» policies`              | array                                                                            | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `»» version_number`        | integer                                                                          | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `» latest_version_id`      | string(uuid)                                                                     | false    |              | Latest version ID / LatestVersionNumber identify the pipeline's tip (most recent) version. Under the explicit two-stage rollout, activating a policy or guardrail mints a new pipeline version on the tip without promoting it, so the tip can be ahead of the active (live) version. When LatestVersionID differs from ActiveVersionID the pipeline has unpromoted changes (drift): the operator can promote the tip to take them live. |
| `» latest_version_number`  | integer                                                                          | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `» provider_id`            | string(uuid)                                                                     | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `» updated_at`             | string(date-time)                                                                | false    |              |                                                                                                                                                                                                                                                                                                                                                                                                                                          |

#### Enumerated Values

| Property    | Value(s)                                   |
|-------------|--------------------------------------------|
| `fail_mode` | `fail_closed`, `fail_open`                 |
| `hook`      | `pre_auth`, `pre_req`, `pre_tool`          |
| `kind`      | `annotate`, `decide`, `route`, `transform` |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create an AI gateway pipeline

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/aibridge/pipelines \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/v2/aibridge/pipelines`

> Body parameter

```json
{
  "enabled": true,
  "guardrails": [
    {
      "enabled": true,
      "fail_mode": "fail_open",
      "guardrail_version_id": "679d05fc-2e89-4d23-9f9d-93315fd86dfd",
      "hook": "pre_auth",
      "network_timeout_ms": 0
    }
  ],
  "policies": [
    {
      "enabled": true,
      "fail_mode": "fail_open",
      "hook": "pre_auth",
      "policy_version_id": "7cd41427-f4be-4006-ab17-5ead7f8f8446"
    }
  ],
  "provider_id": "fe3d49af-4061-436b-ae60-f7044f252a44"
}
```

### Parameters

| Name   | In   | Type                                                                                         | Required | Description             |
|--------|------|----------------------------------------------------------------------------------------------|----------|-------------------------|
| `body` | body | [codersdk.CreateAIGatewayPipelineRequest](schemas.md#codersdkcreateaigatewaypipelinerequest) | true     | Create pipeline request |

### Example responses

> 201 Response

```json
{
  "active_version": {
    "created_at": "2019-08-24T14:15:22Z",
    "guardrails": [
      {
        "enabled": true,
        "fail_mode": "fail_open",
        "guardrail_version_id": "679d05fc-2e89-4d23-9f9d-93315fd86dfd",
        "hook": "pre_auth",
        "network_timeout_ms": 0
      }
    ],
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "pipeline_id": "ec036e81-7903-4e4d-bbfa-ac8516341cf0",
    "policies": [
      {
        "enabled": true,
        "fail_mode": "fail_open",
        "hook": "pre_auth",
        "kind": "annotate",
        "policy_version_id": "7cd41427-f4be-4006-ab17-5ead7f8f8446"
      }
    ],
    "version_number": 0
  },
  "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
  "created_at": "2019-08-24T14:15:22Z",
  "enabled": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "latest_version": {
    "created_at": "2019-08-24T14:15:22Z",
    "guardrails": [
      {
        "enabled": true,
        "fail_mode": "fail_open",
        "guardrail_version_id": "679d05fc-2e89-4d23-9f9d-93315fd86dfd",
        "hook": "pre_auth",
        "network_timeout_ms": 0
      }
    ],
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "pipeline_id": "ec036e81-7903-4e4d-bbfa-ac8516341cf0",
    "policies": [
      {
        "enabled": true,
        "fail_mode": "fail_open",
        "hook": "pre_auth",
        "kind": "annotate",
        "policy_version_id": "7cd41427-f4be-4006-ab17-5ead7f8f8446"
      }
    ],
    "version_number": 0
  },
  "latest_version_id": "8562ca50-d99e-4ec5-9529-9c17b0fd462e",
  "latest_version_number": 0,
  "provider_id": "fe3d49af-4061-436b-ae60-f7044f252a44",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                             |
|--------|--------------------------------------------------------------|-------------|--------------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.AIGatewayPipeline](schemas.md#codersdkaigatewaypipeline) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get an AI gateway pipeline

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/aibridge/pipelines/{id} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/aibridge/pipelines/{id}`

### Parameters

| Name | In   | Type         | Required | Description |
|------|------|--------------|----------|-------------|
| `id` | path | string(uuid) | true     | Pipeline ID |

### Example responses

> 200 Response

```json
{
  "active_version": {
    "created_at": "2019-08-24T14:15:22Z",
    "guardrails": [
      {
        "enabled": true,
        "fail_mode": "fail_open",
        "guardrail_version_id": "679d05fc-2e89-4d23-9f9d-93315fd86dfd",
        "hook": "pre_auth",
        "network_timeout_ms": 0
      }
    ],
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "pipeline_id": "ec036e81-7903-4e4d-bbfa-ac8516341cf0",
    "policies": [
      {
        "enabled": true,
        "fail_mode": "fail_open",
        "hook": "pre_auth",
        "kind": "annotate",
        "policy_version_id": "7cd41427-f4be-4006-ab17-5ead7f8f8446"
      }
    ],
    "version_number": 0
  },
  "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
  "created_at": "2019-08-24T14:15:22Z",
  "enabled": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "latest_version": {
    "created_at": "2019-08-24T14:15:22Z",
    "guardrails": [
      {
        "enabled": true,
        "fail_mode": "fail_open",
        "guardrail_version_id": "679d05fc-2e89-4d23-9f9d-93315fd86dfd",
        "hook": "pre_auth",
        "network_timeout_ms": 0
      }
    ],
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "pipeline_id": "ec036e81-7903-4e4d-bbfa-ac8516341cf0",
    "policies": [
      {
        "enabled": true,
        "fail_mode": "fail_open",
        "hook": "pre_auth",
        "kind": "annotate",
        "policy_version_id": "7cd41427-f4be-4006-ab17-5ead7f8f8446"
      }
    ],
    "version_number": 0
  },
  "latest_version_id": "8562ca50-d99e-4ec5-9529-9c17b0fd462e",
  "latest_version_number": 0,
  "provider_id": "fe3d49af-4061-436b-ae60-f7044f252a44",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                             |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AIGatewayPipeline](schemas.md#codersdkaigatewaypipeline) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete an AI gateway pipeline

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/aibridge/pipelines/{id} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /api/v2/aibridge/pipelines/{id}`

### Parameters

| Name | In   | Type         | Required | Description |
|------|------|--------------|----------|-------------|
| `id` | path | string(uuid) | true     | Pipeline ID |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update an AI gateway pipeline

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/aibridge/pipelines/{id} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /api/v2/aibridge/pipelines/{id}`

> Body parameter

```json
{
  "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
  "enabled": true
}
```

### Parameters

| Name   | In   | Type                                                                                         | Required | Description             |
|--------|------|----------------------------------------------------------------------------------------------|----------|-------------------------|
| `id`   | path | string(uuid)                                                                                 | true     | Pipeline ID             |
| `body` | body | [codersdk.UpdateAIGatewayPipelineRequest](schemas.md#codersdkupdateaigatewaypipelinerequest) | true     | Update pipeline request |

### Example responses

> 200 Response

```json
{
  "active_version": {
    "created_at": "2019-08-24T14:15:22Z",
    "guardrails": [
      {
        "enabled": true,
        "fail_mode": "fail_open",
        "guardrail_version_id": "679d05fc-2e89-4d23-9f9d-93315fd86dfd",
        "hook": "pre_auth",
        "network_timeout_ms": 0
      }
    ],
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "pipeline_id": "ec036e81-7903-4e4d-bbfa-ac8516341cf0",
    "policies": [
      {
        "enabled": true,
        "fail_mode": "fail_open",
        "hook": "pre_auth",
        "kind": "annotate",
        "policy_version_id": "7cd41427-f4be-4006-ab17-5ead7f8f8446"
      }
    ],
    "version_number": 0
  },
  "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
  "created_at": "2019-08-24T14:15:22Z",
  "enabled": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "latest_version": {
    "created_at": "2019-08-24T14:15:22Z",
    "guardrails": [
      {
        "enabled": true,
        "fail_mode": "fail_open",
        "guardrail_version_id": "679d05fc-2e89-4d23-9f9d-93315fd86dfd",
        "hook": "pre_auth",
        "network_timeout_ms": 0
      }
    ],
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "pipeline_id": "ec036e81-7903-4e4d-bbfa-ac8516341cf0",
    "policies": [
      {
        "enabled": true,
        "fail_mode": "fail_open",
        "hook": "pre_auth",
        "kind": "annotate",
        "policy_version_id": "7cd41427-f4be-4006-ab17-5ead7f8f8446"
      }
    ],
    "version_number": 0
  },
  "latest_version_id": "8562ca50-d99e-4ec5-9529-9c17b0fd462e",
  "latest_version_number": 0,
  "provider_id": "fe3d49af-4061-436b-ae60-f7044f252a44",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                             |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AIGatewayPipeline](schemas.md#codersdkaigatewaypipeline) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Enable or disable an AI gateway pipeline member

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/aibridge/pipelines/{id}/members \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /api/v2/aibridge/pipelines/{id}/members`

> Body parameter

```json
{
  "enabled": true,
  "guardrail_version_id": "679d05fc-2e89-4d23-9f9d-93315fd86dfd",
  "hook": "pre_auth",
  "policy_version_id": "7cd41427-f4be-4006-ab17-5ead7f8f8446"
}
```

### Parameters

| Name   | In   | Type                                                                                                     | Required | Description           |
|--------|------|----------------------------------------------------------------------------------------------------------|----------|-----------------------|
| `id`   | path | string(uuid)                                                                                             | true     | Pipeline ID           |
| `body` | body | [codersdk.UpdateAIGatewayPipelineMemberRequest](schemas.md#codersdkupdateaigatewaypipelinememberrequest) | true     | Update member request |

### Example responses

> 200 Response

```json
{
  "active_version": {
    "created_at": "2019-08-24T14:15:22Z",
    "guardrails": [
      {
        "enabled": true,
        "fail_mode": "fail_open",
        "guardrail_version_id": "679d05fc-2e89-4d23-9f9d-93315fd86dfd",
        "hook": "pre_auth",
        "network_timeout_ms": 0
      }
    ],
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "pipeline_id": "ec036e81-7903-4e4d-bbfa-ac8516341cf0",
    "policies": [
      {
        "enabled": true,
        "fail_mode": "fail_open",
        "hook": "pre_auth",
        "kind": "annotate",
        "policy_version_id": "7cd41427-f4be-4006-ab17-5ead7f8f8446"
      }
    ],
    "version_number": 0
  },
  "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
  "created_at": "2019-08-24T14:15:22Z",
  "enabled": true,
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "latest_version": {
    "created_at": "2019-08-24T14:15:22Z",
    "guardrails": [
      {
        "enabled": true,
        "fail_mode": "fail_open",
        "guardrail_version_id": "679d05fc-2e89-4d23-9f9d-93315fd86dfd",
        "hook": "pre_auth",
        "network_timeout_ms": 0
      }
    ],
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "pipeline_id": "ec036e81-7903-4e4d-bbfa-ac8516341cf0",
    "policies": [
      {
        "enabled": true,
        "fail_mode": "fail_open",
        "hook": "pre_auth",
        "kind": "annotate",
        "policy_version_id": "7cd41427-f4be-4006-ab17-5ead7f8f8446"
      }
    ],
    "version_number": 0
  },
  "latest_version_id": "8562ca50-d99e-4ec5-9529-9c17b0fd462e",
  "latest_version_number": 0,
  "provider_id": "fe3d49af-4061-436b-ae60-f7044f252a44",
  "updated_at": "2019-08-24T14:15:22Z"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                             |
|--------|---------------------------------------------------------|-------------|--------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AIGatewayPipeline](schemas.md#codersdkaigatewaypipeline) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List AI gateway pipeline versions

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/aibridge/pipelines/{id}/versions \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/aibridge/pipelines/{id}/versions`

### Parameters

| Name | In   | Type         | Required | Description |
|------|------|--------------|----------|-------------|
| `id` | path | string(uuid) | true     | Pipeline ID |

### Example responses

> 200 Response

```json
[
  {
    "created_at": "2019-08-24T14:15:22Z",
    "guardrails": [
      {
        "enabled": true,
        "fail_mode": "fail_open",
        "guardrail_version_id": "679d05fc-2e89-4d23-9f9d-93315fd86dfd",
        "hook": "pre_auth",
        "network_timeout_ms": 0
      }
    ],
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "pipeline_id": "ec036e81-7903-4e4d-bbfa-ac8516341cf0",
    "policies": [
      {
        "enabled": true,
        "fail_mode": "fail_open",
        "hook": "pre_auth",
        "kind": "annotate",
        "policy_version_id": "7cd41427-f4be-4006-ab17-5ead7f8f8446"
      }
    ],
    "version_number": 0
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                                    |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.AIGatewayPipelineVersion](schemas.md#codersdkaigatewaypipelineversion) |

<h3 id="list-ai-gateway-pipeline-versions-responseschema">Response Schema</h3>

Status Code **200**

| Name                      | Type                                                                   | Required | Restrictions | Description                                                                                                                               |
|---------------------------|------------------------------------------------------------------------|----------|--------------|-------------------------------------------------------------------------------------------------------------------------------------------|
| `[array item]`            | array                                                                  | false    |              |                                                                                                                                           |
| `» created_at`            | string(date-time)                                                      | false    |              |                                                                                                                                           |
| `» guardrails`            | array                                                                  | false    |              |                                                                                                                                           |
| `»» enabled`              | boolean                                                                | false    |              |                                                                                                                                           |
| `»» fail_mode`            | [codersdk.AIGatewayFailMode](schemas.md#codersdkaigatewayfailmode)     | false    |              |                                                                                                                                           |
| `»» guardrail_version_id` | string(uuid)                                                           | false    |              |                                                                                                                                           |
| `»» hook`                 | [codersdk.AIGatewayHook](schemas.md#codersdkaigatewayhook)             | false    |              |                                                                                                                                           |
| `»» network_timeout_ms`   | integer                                                                | false    |              |                                                                                                                                           |
| `» id`                    | string(uuid)                                                           | false    |              |                                                                                                                                           |
| `» pipeline_id`           | string(uuid)                                                           | false    |              |                                                                                                                                           |
| `» policies`              | array                                                                  | false    |              |                                                                                                                                           |
| `»» enabled`              | boolean                                                                | false    |              | Enabled disables this policy within this pipeline without disabling it globally. Disabled members are excluded from the runtime snapshot. |
| `»» fail_mode`            | [codersdk.AIGatewayFailMode](schemas.md#codersdkaigatewayfailmode)     | false    |              |                                                                                                                                           |
| `»» hook`                 | [codersdk.AIGatewayHook](schemas.md#codersdkaigatewayhook)             | false    |              |                                                                                                                                           |
| `»» kind`                 | [codersdk.AIGatewayPolicyKind](schemas.md#codersdkaigatewaypolicykind) | false    |              |                                                                                                                                           |
| `»» policy_version_id`    | string(uuid)                                                           | false    |              |                                                                                                                                           |
| `» version_number`        | integer                                                                | false    |              |                                                                                                                                           |

#### Enumerated Values

| Property    | Value(s)                                   |
|-------------|--------------------------------------------|
| `fail_mode` | `fail_closed`, `fail_open`                 |
| `hook`      | `pre_auth`, `pre_req`, `pre_tool`          |
| `kind`      | `annotate`, `decide`, `route`, `transform` |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create an AI gateway pipeline version

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/aibridge/pipelines/{id}/versions \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/v2/aibridge/pipelines/{id}/versions`

> Body parameter

```json
{
  "activate": true,
  "guardrails": [
    {
      "enabled": true,
      "fail_mode": "fail_open",
      "guardrail_version_id": "679d05fc-2e89-4d23-9f9d-93315fd86dfd",
      "hook": "pre_auth",
      "network_timeout_ms": 0
    }
  ],
  "policies": [
    {
      "enabled": true,
      "fail_mode": "fail_open",
      "hook": "pre_auth",
      "policy_version_id": "7cd41427-f4be-4006-ab17-5ead7f8f8446"
    }
  ]
}
```

### Parameters

| Name   | In   | Type                                                                                                       | Required | Description            |
|--------|------|------------------------------------------------------------------------------------------------------------|----------|------------------------|
| `id`   | path | string(uuid)                                                                                               | true     | Pipeline ID            |
| `body` | body | [codersdk.CreateAIGatewayPipelineVersionRequest](schemas.md#codersdkcreateaigatewaypipelineversionrequest) | true     | Create version request |

### Example responses

> 201 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "guardrails": [
    {
      "enabled": true,
      "fail_mode": "fail_open",
      "guardrail_version_id": "679d05fc-2e89-4d23-9f9d-93315fd86dfd",
      "hook": "pre_auth",
      "network_timeout_ms": 0
    }
  ],
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "pipeline_id": "ec036e81-7903-4e4d-bbfa-ac8516341cf0",
  "policies": [
    {
      "enabled": true,
      "fail_mode": "fail_open",
      "hook": "pre_auth",
      "kind": "annotate",
      "policy_version_id": "7cd41427-f4be-4006-ab17-5ead7f8f8446"
    }
  ],
  "version_number": 0
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                                           |
|--------|--------------------------------------------------------------|-------------|----------------------------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.AIGatewayPipelineVersion](schemas.md#codersdkaigatewaypipelineversion) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## List AI gateway policies

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/aibridge/policies \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/aibridge/policies`

### Example responses

> 200 Response

```json
[
  {
    "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
    "created_at": "2019-08-24T14:15:22Z",
    "display_name": "string",
    "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
    "kind": "annotate",
    "name": "string",
    "updated_at": "2019-08-24T14:15:22Z",
    "versions": [
      {
        "created_at": "2019-08-24T14:15:22Z",
        "created_by": "ee824cad-d7a6-4f48-87dc-e8461a9201c4",
        "description": "string",
        "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
        "input_schema_version": 0,
        "output_schema_version": 0,
        "policy_id": "ee9b03e0-6495-427a-85a5-34444d24ae04",
        "rego": "string",
        "version_number": 0
      }
    ]
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                  |
|--------|---------------------------------------------------------|-------------|-------------------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.AIGatewayPolicy](schemas.md#codersdkaigatewaypolicy) |

<h3 id="list-ai-gateway-policies-responseschema">Response Schema</h3>

Status Code **200**

| Name                       | Type                                                                   | Required | Restrictions | Description |
|----------------------------|------------------------------------------------------------------------|----------|--------------|-------------|
| `[array item]`             | array                                                                  | false    |              |             |
| `» active_version_id`      | string(uuid)                                                           | false    |              |             |
| `» created_at`             | string(date-time)                                                      | false    |              |             |
| `» display_name`           | string                                                                 | false    |              |             |
| `» id`                     | string(uuid)                                                           | false    |              |             |
| `» kind`                   | [codersdk.AIGatewayPolicyKind](schemas.md#codersdkaigatewaypolicykind) | false    |              |             |
| `» name`                   | string                                                                 | false    |              |             |
| `» updated_at`             | string(date-time)                                                      | false    |              |             |
| `» versions`               | array                                                                  | false    |              |             |
| `»» created_at`            | string(date-time)                                                      | false    |              |             |
| `»» created_by`            | string(uuid)                                                           | false    |              |             |
| `»» description`           | string                                                                 | false    |              |             |
| `»» id`                    | string(uuid)                                                           | false    |              |             |
| `»» input_schema_version`  | integer                                                                | false    |              |             |
| `»» output_schema_version` | integer                                                                | false    |              |             |
| `»» policy_id`             | string(uuid)                                                           | false    |              |             |
| `»» rego`                  | string                                                                 | false    |              |             |
| `»» version_number`        | integer                                                                | false    |              |             |

#### Enumerated Values

| Property | Value(s)                                   |
|----------|--------------------------------------------|
| `kind`   | `annotate`, `decide`, `route`, `transform` |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create an AI gateway policy

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/aibridge/policies \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/v2/aibridge/policies`

> Body parameter

```json
{
  "description": "string",
  "display_name": "string",
  "kind": "annotate",
  "name": "string",
  "rego": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                     | Required | Description           |
|--------|------|------------------------------------------------------------------------------------------|----------|-----------------------|
| `body` | body | [codersdk.CreateAIGatewayPolicyRequest](schemas.md#codersdkcreateaigatewaypolicyrequest) | true     | Create policy request |

### Example responses

> 201 Response

```json
{
  "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
  "created_at": "2019-08-24T14:15:22Z",
  "display_name": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "kind": "annotate",
  "name": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "versions": [
    {
      "created_at": "2019-08-24T14:15:22Z",
      "created_by": "ee824cad-d7a6-4f48-87dc-e8461a9201c4",
      "description": "string",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "input_schema_version": 0,
      "output_schema_version": 0,
      "policy_id": "ee9b03e0-6495-427a-85a5-34444d24ae04",
      "rego": "string",
      "version_number": 0
    }
  ]
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                         |
|--------|--------------------------------------------------------------|-------------|----------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.AIGatewayPolicy](schemas.md#codersdkaigatewaypolicy) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get an AI gateway policy

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/aibridge/policies/{id} \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /api/v2/aibridge/policies/{id}`

### Parameters

| Name | In   | Type         | Required | Description |
|------|------|--------------|----------|-------------|
| `id` | path | string(uuid) | true     | Policy ID   |

### Example responses

> 200 Response

```json
{
  "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
  "created_at": "2019-08-24T14:15:22Z",
  "display_name": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "kind": "annotate",
  "name": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "versions": [
    {
      "created_at": "2019-08-24T14:15:22Z",
      "created_by": "ee824cad-d7a6-4f48-87dc-e8461a9201c4",
      "description": "string",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "input_schema_version": 0,
      "output_schema_version": 0,
      "policy_id": "ee9b03e0-6495-427a-85a5-34444d24ae04",
      "rego": "string",
      "version_number": 0
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                         |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AIGatewayPolicy](schemas.md#codersdkaigatewaypolicy) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Delete an AI gateway policy

### Code samples

```shell
# Example request using curl
curl -X DELETE http://coder-server:8080/api/v2/aibridge/policies/{id} \
  -H 'Coder-Session-Token: API_KEY'
```

`DELETE /api/v2/aibridge/policies/{id}`

### Parameters

| Name | In   | Type         | Required | Description |
|------|------|--------------|----------|-------------|
| `id` | path | string(uuid) | true     | Policy ID   |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Update an AI gateway policy

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/aibridge/policies/{id} \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /api/v2/aibridge/policies/{id}`

> Body parameter

```json
{
  "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
  "display_name": "string",
  "promote": true
}
```

### Parameters

| Name   | In   | Type                                                                                     | Required | Description           |
|--------|------|------------------------------------------------------------------------------------------|----------|-----------------------|
| `id`   | path | string(uuid)                                                                             | true     | Policy ID             |
| `body` | body | [codersdk.UpdateAIGatewayPolicyRequest](schemas.md#codersdkupdateaigatewaypolicyrequest) | true     | Update policy request |

### Example responses

> 200 Response

```json
{
  "active_version_id": "eae64611-bd53-4a80-bb77-df1e432c0fbc",
  "created_at": "2019-08-24T14:15:22Z",
  "display_name": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "kind": "annotate",
  "name": "string",
  "updated_at": "2019-08-24T14:15:22Z",
  "versions": [
    {
      "created_at": "2019-08-24T14:15:22Z",
      "created_by": "ee824cad-d7a6-4f48-87dc-e8461a9201c4",
      "description": "string",
      "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
      "input_schema_version": 0,
      "output_schema_version": 0,
      "policy_id": "ee9b03e0-6495-427a-85a5-34444d24ae04",
      "rego": "string",
      "version_number": 0
    }
  ]
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                         |
|--------|---------------------------------------------------------|-------------|----------------------------------------------------------------|
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.AIGatewayPolicy](schemas.md#codersdkaigatewaypolicy) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Create an AI gateway policy version

### Code samples

```shell
# Example request using curl
curl -X POST http://coder-server:8080/api/v2/aibridge/policies/{id}/versions \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`POST /api/v2/aibridge/policies/{id}/versions`

> Body parameter

```json
{
  "activate": true,
  "description": "string",
  "promote": true,
  "rego": "string"
}
```

### Parameters

| Name   | In   | Type                                                                                                   | Required | Description            |
|--------|------|--------------------------------------------------------------------------------------------------------|----------|------------------------|
| `id`   | path | string(uuid)                                                                                           | true     | Policy ID              |
| `body` | body | [codersdk.CreateAIGatewayPolicyVersionRequest](schemas.md#codersdkcreateaigatewaypolicyversionrequest) | true     | Create version request |

### Example responses

> 201 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "created_by": "ee824cad-d7a6-4f48-87dc-e8461a9201c4",
  "description": "string",
  "id": "497f6eca-6276-4993-bfeb-53cbbbba6f08",
  "input_schema_version": 0,
  "output_schema_version": 0,
  "policy_id": "ee9b03e0-6495-427a-85a5-34444d24ae04",
  "rego": "string",
  "version_number": 0
}
```

### Responses

| Status | Meaning                                                      | Description | Schema                                                                       |
|--------|--------------------------------------------------------------|-------------|------------------------------------------------------------------------------|
| 201    | [Created](https://tools.ietf.org/html/rfc7231#section-6.3.2) | Created     | [codersdk.AIGatewayPolicyVersion](schemas.md#codersdkaigatewaypolicyversion) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
