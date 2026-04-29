<!-- DO NOT EDIT | GENERATED CONTENT -->
# secret

Manage secrets

Aliases:

* secrets

## Usage

```console
coder secret
```

## Description

```console
  - Create a secret:

     $ printf %s "$MYCLI_API_KEY" | coder secret create api-key --description "API key for workspace tools" --env API_KEY --file "~/.api-key"

  - Update a secret:

     $ echo -n "$NEW_SECRET_VALUE" | coder secret update api-key --description "Rotated API key" --env API_KEY --file "~/.api-key"

  - List your secrets:

     $ coder secret list

  - Show a specific secret:

     $ coder secret list api-key

  - Delete a secret:

     $ coder secret delete api-key
```

## Subcommands

| Name                                      | Purpose                           |
|-------------------------------------------|-----------------------------------|
| [<code>create</code>](./secret_create.md) | Create a secret                   |
| [<code>update</code>](./secret_update.md) | Update a secret                   |
| [<code>list</code>](./secret_list.md)     | List secrets, or show one by name |
| [<code>delete</code>](./secret_delete.md) | Delete a secret                   |
