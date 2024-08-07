<!-- DO NOT EDIT | GENERATED CONTENT -->

# external-auth access-token

Print auth for an external provider

## Usage

```console
coder external-auth access-token [flags] <provider>
```

## Description

```console
Print an access-token for an external auth provider. The access-token will be validated and sent to stdout with exit code 0. If a valid access-token cannot be obtained, the URL to authenticate will be sent to stdout with exit code 1
  - Ensure that the user is authenticated with GitHub before cloning.:

     $ #!/usr/bin/env sh

OUTPUT=$(coder external-auth access-token github)
if [ $? -eq 0 ]; then
  echo "Authenticated with GitHub"
else
  echo "Please authenticate with GitHub:"
  echo $OUTPUT
fi


  - Obtain an extra property of an access token for additional metadata.:

     $ coder external-auth access-token slack --extra "authed_user.id"
```

## Options

### --extra

|      |                     |
| ---- | ------------------- |
| Type | <code>string</code> |

Extract a field from the "extra" properties of the OAuth token.
