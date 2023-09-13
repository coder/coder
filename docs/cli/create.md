<!-- DO NOT EDIT | GENERATED CONTENT -->

# create

Create a workspace

## Usage

```console
coder create [flags] [name]
```

## Description

```console
  - Create a workspace for another user (if you have permission):

     $ coder create <username>/<workspace_name>
```

## Options

### --parameter

|             |                                    |
| ----------- | ---------------------------------- |
| Type        | <code>string-array</code>          |
| Environment | <code>$CODER_RICH_PARAMETER</code> |

Rich parameter value in the format "name=value".

### --rich-parameter-file

|             |                                         |
| ----------- | --------------------------------------- |
| Type        | <code>string</code>                     |
| Environment | <code>$CODER_RICH_PARAMETER_FILE</code> |

Specify a file path with values for rich parameters defined in the template.

### --start-at

|             |                                        |
| ----------- | -------------------------------------- |
| Type        | <code>string</code>                    |
| Environment | <code>$CODER_WORKSPACE_START_AT</code> |

Specify the workspace autostart schedule. Check coder schedule start --help for the syntax.

### --stop-after

|             |                                          |
| ----------- | ---------------------------------------- |
| Type        | <code>duration</code>                    |
| Environment | <code>$CODER_WORKSPACE_STOP_AFTER</code> |

Specify a duration after which the workspace should shut down (e.g. 8h).

### -t, --template

|             |                                   |
| ----------- | --------------------------------- |
| Type        | <code>string</code>               |
| Environment | <code>$CODER_TEMPLATE_NAME</code> |

Specify a template name.

### -y, --yes

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Bypass prompts.
