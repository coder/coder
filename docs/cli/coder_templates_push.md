<!-- DO NOT EDIT | GENERATED CONTENT -->

# coder templates push

Push a new template version from the current directory or as specified by flag

## Usage

```console
coder templates push [template] [flags]
```

## Flags

### --always-prompt

Always prompt all parameters. Does not pull parameter values from active template version
<br/>
| | |
| --- | --- |
| Default | <code>false</code> |

### --directory, -d

Specify the directory to create from, use '-' to read tar from stdin
<br/>
| | |
| --- | --- |
| Default | <code>.</code> |

### --name

Specify a name for the new template version. It will be automatically generated if not provided.
<br/>
| | |
| --- | --- |

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
