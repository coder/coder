<!-- DO NOT EDIT | GENERATED CONTENT -->
# coder templates create


Create a template from the current directory or as specified by flag

## Usage
```console
coder templates create [name] [flags]
```


## Local Flags
| Name |  Default | Usage | Environment | 
| ---- |  ------- | ----- | -------- |
| --default-ttl |24h0m0s |<code>Specify a default TTL for workspaces created from this template.</code> | |
| --directory, -d |/home/coder/coder |<code>Specify the directory to create from, use '-' to read tar from stdin</code> | |
| --parameter-file | |<code>Specify a file path with parameter values.</code> | |
| --provisioner-tag |[] |<code>Specify a set of tags to target provisioner daemons.</code> | |
| --variable |[] |<code>Specify a set of values for Terraform-managed variables.</code> | |
| --variables-file | |<code>Specify a file path with values for Terraform-managed variables.</code> | |
| --yes, -y |false |<code>Bypass prompts</code> | |