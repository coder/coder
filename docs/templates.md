# Templates

Templates define the infrastructure underlying workspaces. Each Coder deployment
can have multiple templates for different workloads, such as ones for front-end
development, Windows development, and so on.

Coder manages templates, including sharing them and rolling out updates
to everybody. Users can also manually update their workspaces.

## Manage templates

Coder provides production-ready [sample templates](https://github.com/coder/coder/tree/main/examples/templates),
but you can modify the templates with Terraform.

```sh
# start from an example
coder templates init

# optional: modify the template
vim <template-name>/main.tf

# add the template to Coder deployment
coder templates <create/update> <template-name>
```

## Parameters

Templates often contain _parameters_. In Coder, there are two types of parameters:

- **Admin parameters** are set when a template is created/updated. These values
  are often cloud secrets, such as a `ServiceAccount` token, and are annotated
  with `sensitive = true` in the template code.
- **User parameters** are set when a user creates a workspace. They are unique
  to each workspace, often personalization settings such as "preferred region"
  or "workspace image".

## Change Management

We recommend source controlling your templates as you would other code.

CI is as simple as running `coder templates update` with the appropriate
credentials.

---

Next: [Workspaces](./workspaces.md)

Next: [Authentication & Secrets](./authentication.md)
