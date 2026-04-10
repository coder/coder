<!-- DO NOT EDIT | GENERATED CONTENT -->
# secret

Manage personal secrets

Aliases:

* secrets

## Usage

```console
coder secret
```

## Description

```console
  - Create a secret:

     $ coder secret create openai-key --value "$SECRET_VALUE" --description "Personal OPENAI_API key" --inject-env OPEN_AI_KEY --inject-file "~/.openai-key"

  - Update a secret:

     $ coder secret update openai-key --value "$NEW_SECRET_VALUE" --description "Updated description" --inject-env NEW_ENV_NAME --inject-file "~/.new-path"

  - List your secrets:

     $ coder secret list

  - Show a specific secret:

     $ coder secret list openai-key

  - Delete a secret:

     $ coder secret delete openai-key
```

## Subcommands

| Name                                      | Purpose                           |
|-------------------------------------------|-----------------------------------|
| [<code>create</code>](./secret_create.md) | Create a secret                   |
| [<code>update</code>](./secret_update.md) | Update a secret                   |
| [<code>list</code>](./secret_list.md)     | List secrets, or show one by name |
| [<code>delete</code>](./secret_delete.md) | Delete a secret                   |
