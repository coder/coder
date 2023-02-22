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
| Name |   Purpose |
| ---- |   ----- |
| <code>create</code> | Create a template from the current directory or as specified by flag |
| <code>delete</code> | Delete templates |
| <code>edit</code> | Edit the metadata of a template by name. |
| <code>init</code> | Get started with a templated template. |
| <code>list</code> | List all the templates available for the organization |
| <code>plan</code> | Plan a template push from the current directory |
| <code>pull</code> | Download the latest version of a template to a path. |
| <code>push</code> | Push a new template version from the current directory or as specified by flag |
| <code>versions</code> | Manage different versions of the specified template |
