# API & Session Tokens

Users can generate tokens to make API requests on behalf of themselves.

## Short-Lived Tokens (Sessions)

The [Coder CLI](../../install/cli.md) and
[Backstage Plugin](https://github.com/coder/backstage-plugins) use short-lived
token to authenticate. To generate a short-lived session token on behalf of your
account, visit the following URL: `https://coder.example.com/cli-auth`

### Session Durations

By default, sessions last 24 hours and are automatically refreshed. You can
configure
[`CODER_SESSION_DURATION`](../../reference/cli/server.md#--session-duration) to
change the duration and
[`CODER_DISABLE_SESSION_EXPIRY_REFRESH`](../../reference/cli/server.md#--disable-session-expiry-refresh)
to configure this behavior.

## Long-Lived Tokens (API Tokens)

Users can create long lived tokens. We refer to these as "API tokens" in the
product.

### Generate a long-lived API token on behalf of yourself

<div class="tabs">

#### UI

Visit your account settings in the top right of the dashboard or by navigating
to `https://coder.example.com/settings/account`

Navigate to the tokens page in the sidebar and create a new token:

![Create an API token](../../images/admin/users/create-token.png)

#### CLI

Use the following command:

```sh
coder tokens create --name=my-token --lifetime=720h
```

See the help docs for
[`coder tokens create`](../../reference/cli/tokens_create.md) for more info.

</div>

### Generate a long-lived API token on behalf of another user

You must have the `Owner` role to generate a token for another user.

As of Coder v2.17+, you can use the CLI or API to create long-lived tokens on
behalf of other users. Use the API for earlier versions of Coder.

<div class="tabs">

#### CLI

```sh
coder tokens create --name my-token --user <username>
```

See the full CLI reference for
[`coder tokens create`](../../reference/cli/tokens_create.md)

#### API

Use our API reference for more information on how to
[create token API key](../../reference/api/users.md#create-token-api-key)

</div>

### Set max token length

You can use the
[`CODER_MAX_TOKEN_LIFETIME`](https://coder.com/docs/reference/cli/server#--max-token-lifetime)
server flag to set the maximum duration for long-lived tokens in your
deployment.

## API Key Scopes

API key scopes allow you to limit the permissions of a token to specific operations. By default, tokens are created with the `all` scope, granting full access to all actions the user can perform. For improved security, you can create tokens with limited scopes that restrict access to only the operations needed.

Scopes follow the format `resource:action`, where `resource` is the type of object (like `workspace`, `template`, or `user`) and `action` is the operation (like `read`, `create`, `update`, or `delete`). You can also use wildcards like `workspace:*` to grant all permissions for a specific resource type.

### Creating tokens with scopes

You can specify scopes when creating a token using the `--scope` flag:

```sh
# Create a token that can only read workspaces
coder tokens create --name readonly-token --scope workspace:read

# Create a token with multiple scopes
coder tokens create --name limited-token --scope workspace:read --scope template:read
```

Common scope examples include:

- `workspace:read` - View workspace information
- `workspace:*` - Full workspace access (create, read, update, delete)
- `template:read` - View template information
- `api_key:read` - View API keys (useful for automation)
- `application_connect` - Connect to workspace applications

For a complete list of available scopes, see the API reference documentation.

