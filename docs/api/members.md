# Members

## Get member roles by organization

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/organizations/{organization}/members/roles \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /organizations/{organization}/members/roles`

### Parameters

| Name           | In   | Type         | Required | Description     |
| -------------- | ---- | ------------ | -------- | --------------- |
| `organization` | path | string(uuid) | true     | Organization ID |

### Example responses

> 200 Response

```json
[
  {
    "assignable": true,
    "display_name": "string",
    "name": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                  |
| ------ | ------------------------------------------------------- | ----------- | ----------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.AssignableRoles](schemas.md#codersdkassignableroles) |

<h3 id="get-member-roles-by-organization-responseschema">Response Schema</h3>

Status Code **200**

| Name             | Type    | Required | Restrictions | Description |
| ---------------- | ------- | -------- | ------------ | ----------- |
| `[array item]`   | array   | false    |              |             |
| `» assignable`   | boolean | false    |              |             |
| `» display_name` | string  | false    |              |             |
| `» name`         | string  | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Assign role to organization member

### Code samples

```shell
# Example request using curl
curl -X PUT http://coder-server:8080/api/v2/organizations/{organization}/members/{user}/roles \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PUT /organizations/{organization}/members/{user}/roles`

> Body parameter

```json
{
  "roles": ["string"]
}
```

### Parameters

| Name           | In   | Type                                                   | Required | Description          |
| -------------- | ---- | ------------------------------------------------------ | -------- | -------------------- |
| `organization` | path | string                                                 | true     | Organization ID      |
| `user`         | path | string                                                 | true     | User ID, name, or me |
| `body`         | body | [codersdk.UpdateRoles](schemas.md#codersdkupdateroles) | true     | Update roles request |

### Example responses

> 200 Response

```json
{
  "created_at": "2019-08-24T14:15:22Z",
  "organization_id": "7c60d51f-b44e-4682-87d6-449835ea4de6",
  "roles": [
    {
      "display_name": "string",
      "name": "string"
    }
  ],
  "updated_at": "2019-08-24T14:15:22Z",
  "user_id": "a169451c-8525-4352-b8ca-070dd449a1a5"
}
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                               |
| ------ | ------------------------------------------------------- | ----------- | -------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | [codersdk.OrganizationMember](schemas.md#codersdkorganizationmember) |

To perform this operation, you must be authenticated. [Learn more](authentication.md).

## Get site member roles

### Code samples

```shell
# Example request using curl
curl -X GET http://coder-server:8080/api/v2/users/roles \
  -H 'Accept: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`GET /users/roles`

### Example responses

> 200 Response

```json
[
  {
    "assignable": true,
    "display_name": "string",
    "name": "string"
  }
]
```

### Responses

| Status | Meaning                                                 | Description | Schema                                                                  |
| ------ | ------------------------------------------------------- | ----------- | ----------------------------------------------------------------------- |
| 200    | [OK](https://tools.ietf.org/html/rfc7231#section-6.3.1) | OK          | array of [codersdk.AssignableRoles](schemas.md#codersdkassignableroles) |

<h3 id="get-site-member-roles-responseschema">Response Schema</h3>

Status Code **200**

| Name             | Type    | Required | Restrictions | Description |
| ---------------- | ------- | -------- | ------------ | ----------- |
| `[array item]`   | array   | false    |              |             |
| `» assignable`   | boolean | false    |              |             |
| `» display_name` | string  | false    |              |             |
| `» name`         | string  | false    |              |             |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
