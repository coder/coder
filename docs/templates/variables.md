# Terraform template-wide variables

In Coder, Terraform templates offer extensive flexibility through template-wide
variables. These variables, managed by template authors, facilitate the
construction of customizable templates. Unlike parameters, which are primarily
for workspace customization, template variables remain under the control of the
template author, ensuring workspace users cannot modify them.

```hcl
variable "CLOUD_API_KEY" {
  type        = string
  description = "API key for the service"
  default     = "1234567890"
  sensitive   = true
}
```

Given that variables are a
[fundamental concept in Terraform](https://developer.hashicorp.com/terraform/language/values/variables),
Coder endeavors to fully support them. Native support includes `string`,
`number`, and `bool` formats. However, other types such as `list(string)` or
`map(any)` will default to being treated as strings.

## Default value

Upon adding a template variable, it's mandatory to provide a value during the
first push. At this stage, the template administrator faces two choices:

1. _No `default` property_: opt not to define a default property. Instead,
   utilize the `--var name=value` command-line argument during the push to
   supply the variable's value.
2. _Define `default` property_: set a default property for the template
   variable. If the administrator doesn't input a value via CLI, Coder
   automatically uses this default during the push.

After the initial push, variables are stored in the database table, associated
with the specific template version. They can be conveniently managed via
_Template Settings_ without requiring an extra push.

### Resolved values vs. default values

It's crucial to note that Coder templates operate based on resolved values
during a push, rather than default values. This ensures that default values do
not inadvertently override the configured variable settings during the push
process.

This approach caters to users who prefer to avoid accidental overrides of their
variable settings with default values during pushes, thereby enhancing control
and predictability.

If you encounter a situation where you need to override template settings for
variables, you can employ a straightforward solution:

1. Create a `terraform.tfvars` file in in the template directory:

```hcl
coder_image = newimage:tag
```

2. Push the new template revision using Coder CLI:

```
coder templates push my-template -y # no need to use --var
```

This file serves as a mechanism to override the template settings for variables.
It can be stored in the repository for easy access and reference. Coder CLI
automatically detects it and loads variable values.

## Input options

When working with Terraform configurations in Coder, you have several options
for providing values to variables using the Coder CLI:

1. _Manual input in CLI_: You can manually input values for Terraform variables
   directly in the CLI during the deployment process.
2. _Command-line argument_: Utilize the `--var name=value` command-line argument
   to specify variable values inline as key-value pairs.
3. _Variables file selection_: Alternatively, you can use a variables file
   selected via the `--variables-file values.yml` command-line argument. This
   approach is particularly useful when dealing with multiple variables or to
   avoid manual input of numerous values. Variables files can be versioned for
   better traceability and management, and it enhances reproducibility.

Here's an example of a YAML-formatted variables file, `values.yml`:

```yaml
region: us-east-1
bucket_name: magic
zone_types: '{"us-east-1":"US East", "eu-west-1": "EU West"}'
cpu: 1
```

In this sample file:

- `region`, `bucket_name`, `zone_types`, and `cpu` are Terraform variable names.
- Corresponding values are provided for each variable.
- The `zone_types` variable demonstrates how to provide a JSON-formatted string
  as a value in YAML.

## Terraform .tfvars files

In Terraform, `.tfvars` files provide a convenient means to define variable
values for a project in a reusable manner. These files, ending with either
`.tfvars` or `.tfvars.json`, streamline the process of setting numerous
variables.

By utilizing `.tfvars` files, you can efficiently manage and organize variable
values for your Terraform projects. This approach offers several advantages:

- Clarity and consistency: Centralize variable definitions in dedicated files,
  enhancing clarity, instead of input values on template push.
- Ease of maintenance: Modify variable values in a single location under version
  control, simplifying maintenance and updates.

Coder automatically loads variable definition files following a specific order,
providing flexibility and control over variable configuration. The loading
sequence is as follows:

1. `terraform.tfvars`: This file contains variable values and is loaded first.
2. `terraform.tfvars.json`: If present, this JSON-formatted file is loaded after
   `terraform.tfvars`.
3. `\*.auto.tfvars`: Files matching this pattern are loaded next, ordered
   alphabetically.
4. `\*.auto.tfvars.json`: JSON-formatted files matching this pattern are loaded
   last.
