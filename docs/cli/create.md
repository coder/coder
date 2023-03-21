<!-- DO NOT EDIT | GENERATED CONTENT -->

# create

Create a workspace

## Usage

```console
coder create [name]
```

## Options

### --template, -t

|             |                                   |
| ----------- | --------------------------------- |
| Environment | <code>$CODER_TEMPLATE_NAME</code> |

Specify a template name.

### --parameter-file

|             |                                    |
| ----------- | ---------------------------------- |
| Environment | <code>$CODER_PARAMETER_FILE</code> |

Specify a file path with parameter values.

### --rich-parameter-file

|             |                                         |
| ----------- | --------------------------------------- |
| Environment | <code>$CODER_RICH_PARAMETER_FILE</code> |

Specify a file path with values for rich parameters defined in the template.

### --start-at

|             |                                        |
| ----------- | -------------------------------------- |
| Environment | <code>$CODER_WORKSPACE_START_AT</code> |

Specify the workspace autostart schedule. Check `coder schedule start --help` for the syntax.

### --stop-after

|             |                                          |
| ----------- | ---------------------------------------- |
| Environment | <code>$CODER_WORKSPACE_STOP_AFTER</code> |

Specify a duration after which the workspace should shut down (e.g. 8h).

### --yes, -y

Bypass prompts.
