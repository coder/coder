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

     $ coder secret create api-key --value "$SECRET_VALUE" --description "API key for workspace tools" --inject-env API_KEY --inject-file "~/.api-key"

  - Update a secret:

     $ coder secret update api-key --value "$NEW_SECRET_VALUE" --description "Rotated API key" --inject-env API_KEY --inject-file "~/.api-key"

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
