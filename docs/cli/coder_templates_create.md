<!-- DO NOT EDIT | GENERATED CONTENT -->

# coder templates create

Create a template from the current directory or as specified by flag

## Usage

```console
coder templates create [name] [flags]
```

## Flags

### --default-ttl

Specify a default TTL for workspaces created from this template.
<br/>
| | |
| --- | --- |
| Default | <code>24h0m0s</code> |

### --directory, -d

Specify the directory to create from, use '-' to read tar from stdin
<br/>
| | |
| --- | --- |
| Default | <code>.</code> |

### --parameter-file

Specify a file path with parameter values.
<br/>
| | |
| --- | --- |

### --provisioner-tag

Specify a set of tags to target provisioner daemons.
<br/>
| | |
| --- | --- |
| Default | <code>[]</code> |

### --variable

Specify a set of values for Terraform-managed variables.
<br/>
| | |
| --- | --- |
| Default | <code>[]</code> |

### --variables-file

Specify a file path with values for Terraform-managed variables.
<br/>
| | |
| --- | --- |

### --yes, -y

Bypass prompts
<br/>
| | |
| --- | --- |
| Default | <code>false</code> |
