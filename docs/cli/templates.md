
# templates

 
Manage templates


## Usage
```console
templates
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
| Name |   Purpose |
| ---- |   ----- |
| [&lt;code&gt;create&lt;/code&gt;](./templates_create) | Create a template from the current directory or as specified by flag |
| [&lt;code&gt;edit&lt;/code&gt;](./templates_edit) | Edit the metadata of a template by name. |
| [&lt;code&gt;init&lt;/code&gt;](./templates_init) | Get started with a templated template. |
| [&lt;code&gt;list&lt;/code&gt;](./templates_list) | List all the templates available for the organization |
| [&lt;code&gt;plan&lt;/code&gt;](./templates_plan) | Plan a template push from the current directory |
| [&lt;code&gt;push&lt;/code&gt;](./templates_push) | Push a new template version from the current directory or as specified by flag |
| [&lt;code&gt;versions&lt;/code&gt;](./templates_versions) | Manage different versions of the specified template |
| [&lt;code&gt;delete&lt;/code&gt;](./templates_delete) | Delete templates |
| [&lt;code&gt;pull&lt;/code&gt;](./templates_pull) | Download the latest version of a template to a path. |
