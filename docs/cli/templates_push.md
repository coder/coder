<!-- DO NOT EDIT | GENERATED CONTENT -->

# push

Push a new template version from the current directory or as specified by flag

## Usage

```console
push [template]
```

## Options

### --test.provisioner, -p

|         |                        |
| ------- | ---------------------- |
| Default | <code>terraform</code> |

Customize the provisioner backend

### --parameter-file

|     |     |
| --- | --- |

Specify a file path with parameter values.

### --variables-file

|     |     |
| --- | --- |

Specify a file path with values for Terraform-managed variables.

### --variable

|     |     |
| --- | --- |

Specify a set of values for Terraform-managed variables.

### --provisioner-tag, -t

|     |     |
| --- | --- |

Specify a set of tags to target provisioner daemons.

### --name

|     |     |
| --- | --- |

Specify a name for the new template version. It will be automatically generated if not provided.

### --always-prompt

|     |     |
| --- | --- |

Always prompt all parameters. Does not pull parameter values from active template version

### --yes, -y

|             |                                 |
| ----------- | ------------------------------- |
| Environment | <code>$CODER_SKIP_PROMPT</code> |

Bypass prompts

### --directory, -d

|         |                |
| ------- | -------------- |
| Default | <code>.</code> |

Specify the directory to create from, use '-' to read tar from stdin
