<!-- DO NOT EDIT | GENERATED CONTENT -->
# templates

 
Manage templates


## Usage
```console
coder templates
```

## Description
```console
Templates are written in standard Terraform and describe the infrastructure for workspaces
  - Create a template for developers to create workspaces.:                     

      $ coder templates create 

  - Make changes to your template, and plan the changes.:                       

      $ coder templates plan my-template 

  - Push an update to the template. Your developers can update their            
    workspaces.:                                                                

      $ coder templates push my-template 
```

## Subcommands
| Name |   Purpose |
| ---- |   ----- |
| [<code>create</code>](./templates_create) | Create a template from the current directory or as specified by flag |
| [<code>edit</code>](./templates_edit) | Edit the metadata of a template by name. |
| [<code>init</code>](./templates_init) | Get started with a templated template. |
| [<code>list</code>](./templates_list) | List all the templates available for the organization |
| [<code>plan</code>](./templates_plan) | Plan a template push from the current directory |
| [<code>push</code>](./templates_push) | Push a new template version from the current directory or as specified by flag |
| [<code>versions</code>](./templates_versions) | Manage different versions of the specified template |
| [<code>delete</code>](./templates_delete) | Delete templates |
| [<code>pull</code>](./templates_pull) | Download the latest version of a template to a path. |
