# Authentication

Long-lived tokens can be generated to perform actions on behalf of your user account:

```console
coder tokens create
```

You can use the created tokens with Coder's REST API by setting the `Coder-Session-Token` HTTP header.

```console
curl 'http://coder-server:8080/api/v2/workspaces' \
  -H 'Coder-Session-Token: *****'
```
