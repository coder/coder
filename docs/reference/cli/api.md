<!-- DO NOT EDIT | GENERATED CONTENT -->
# api

Make requests to the Coder API

## Usage

```console
coder api [flags] <api-path>
```

## Description

```console
Make an authenticated API request using your current Coder CLI token.

Examples:
  coder api workspacebuilds/my-build/logs
This will perform a GET request to /api/v2/workspacebuilds/my-build/logs on the connected Coder server.

  coder api users/me
This will perform a GET request to /api/v2/users/me on the connected Coder server.

Consult the API documentation for more information - https://coder.com/docs/reference/api.

```

## Options

### --color

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>auto\|always\|never</code> |
| Environment | <code>$CODER_COLOR</code>        |
| Default     | <code>auto</code>                |

Output colorization: auto, always, never.
