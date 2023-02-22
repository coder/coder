<!-- DO NOT EDIT | GENERATED CONTENT -->
# coder templates push

Push a new template version from the current directory or as specified by flag
## Usage
```console
coder templates push [template] [flags]
```

## Local Flags
| Name |  Default | Usage |
| ---- |  ------- | ----- |
| --always-prompt | false | <code>Always prompt all parameters. Does not pull parameter values from active template version</code>|
| --directory, -d | <current-directory> | <code>Specify the directory to create from, use '-' to read tar from stdin</code>|
| --name |  | <code>Specify a name for the new template version. It will be automatically generated if not provided.</code>|
| --parameter-file |  | <code>Specify a file path with parameter values.</code>|
| --provisioner-tag | [] | <code>Specify a set of tags to target provisioner daemons.</code>|
| --test.provisioner | terraform | <code>Customize the provisioner backend</code>|
| --variable | [] | <code>Specify a set of values for Terraform-managed variables.</code>|
| --variables-file |  | <code>Specify a file path with values for Terraform-managed variables.</code>|
| --yes, -y | false | <code>Bypass prompts</code>|