<!-- DO NOT EDIT | GENERATED CONTENT -->

# coder templates

Templates are written in standard Terraform and describe the infrastructure for workspaces

## Usage

```console
coder templates [flags]
```

## Examples

```console
  - Create a template for developers to create workspaces:

      $ coder templates create

  - Make changes to your template, and plan the changes:

      $ coder templates plan my-template

  - Push an update to the template. Your developers can update their workspaces:

      $ coder templates push my-template
```

## Subcommands

| Name                                                | Purpose                                                                        |
| --------------------------------------------------- | ------------------------------------------------------------------------------ |
| [<code>create</code>](./coder_templates_create)     | Create a template from the current directory or as specified by flag           |
| [<code>delete</code>](./coder_templates_delete)     | Delete templates                                                               |
| [<code>edit</code>](./coder_templates_edit)         | Edit the metadata of a template by name.                                       |
| [<code>init</code>](./coder_templates_init)         | Get started with a templated template.                                         |
| [<code>list</code>](./coder_templates_list)         | List all the templates available for the organization                          |
| [<code>plan</code>](./coder_templates_plan)         | Plan a template push from the current directory                                |
| [<code>pull</code>](./coder_templates_pull)         | Download the latest version of a template to a path.                           |
| [<code>push</code>](./coder_templates_push)         | Push a new template version from the current directory or as specified by flag |
| [<code>versions</code>](./coder_templates_versions) | Manage different versions of the specified template                            |
