# Default

## Update workspace ACL

### Code samples

```shell
# Example request using curl
curl -X PATCH http://coder-server:8080/api/v2/workspaces/{workspace}/acl \
  -H 'Content-Type: application/json' \
  -H 'Coder-Session-Token: API_KEY'
```

`PATCH /workspaces/{workspace}/acl`

> Body parameter

```json
{
  "group_roles": {
    "property1": "admin",
    "property2": "admin"
  },
  "user_roles": {
    "property1": "admin",
    "property2": "admin"
  }
}
```

### Parameters

| Name        | In   | Type                                                                 | Required | Description                  |
|-------------|------|----------------------------------------------------------------------|----------|------------------------------|
| `workspace` | path | string(uuid)                                                         | true     | Workspace ID                 |
| `body`      | body | [codersdk.UpdateWorkspaceACL](schemas.md#codersdkupdateworkspaceacl) | true     | Update workspace ACL request |

### Responses

| Status | Meaning                                                         | Description | Schema |
|--------|-----------------------------------------------------------------|-------------|--------|
| 204    | [No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5) | No Content  |        |

To perform this operation, you must be authenticated. [Learn more](authentication.md).
