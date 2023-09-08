<!-- DO NOT EDIT | GENERATED CONTENT -->

# templates

Manage templates

Aliases:

- template

## Usage

```console
coder templates
```

## Description

```console
Templates are written in standard Terraform and describe the infrastructure for workspaces
  - Create a template for developers to create workspaces:

     $ coder templates create

  - Make changes to your template, and plan the changes:

     $ coder templates plan my-template

  - Push an update to the template. Your developers can update their workspaces:

     $ coder templates push my-template
```

## Subcommands

| Name                                             | Purpose                                                                        |
| ------------------------------------------------ | ------------------------------------------------------------------------------ |
| [<code>create</code>](./templates_create.md)     | Create a template from the current directory or as specified by flag           |
| [<code>delete</code>](./templates_delete.md)     | Delete templates                                                               |
| [<code>edit</code>](./templates_edit.md)         | Edit the metadata of a template by name.                                       |
| [<code>init</code>](./templates_init.md)         | Get started with a templated template.                                         |
| [<code>list</code>](./templates_list.md)         | List all the templates available for the organization                          |
| [<code>pull</code>](./templates_pull.md)         | Download the latest version of a template to a path.                           |
| [<code>push</code>](./templates_push.md)         | Push a new template version from the current directory or as specified by flag |
| [<code>versions</code>](./templates_versions.md) | Manage different versions of the specified template                            |
