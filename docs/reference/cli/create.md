<!-- DO NOT EDIT | GENERATED CONTENT -->
# create

Create a workspace

## Usage

```console
coder create [flags] [workspace]
```

## Description

```console
  - Create a workspace for another user (if you have permission):

     $ coder create <username>/<workspace_name>
```

## Options

### -t, --template

|             |                                   |
|-------------|-----------------------------------|
| Type        | <code>string</code>               |
| Environment | <code>$CODER_TEMPLATE_NAME</code> |

Specify a template name.

### --template-version

|             |                                      |
|-------------|--------------------------------------|
| Type        | <code>string</code>                  |
| Environment | <code>$CODER_TEMPLATE_VERSION</code> |

Specify a template version name.

### --preset

|             |                                 |
|-------------|---------------------------------|
| Type        | <code>string</code>             |
| Environment | <code>$CODER_PRESET_NAME</code> |

Specify the name of a template version preset. Use 'none' to explicitly indicate that no preset should be used.

### --start-at

|             |                                        |
|-------------|----------------------------------------|
| Type        | <code>string</code>                    |
| Environment | <code>$CODER_WORKSPACE_START_AT</code> |

Specify the workspace autostart schedule. Check coder schedule start --help for the syntax.

### --stop-after

|             |                                          |
|-------------|------------------------------------------|
| Type        | <code>duration</code>                    |
| Environment | <code>$CODER_WORKSPACE_STOP_AFTER</code> |

Specify a duration after which the workspace should shut down (e.g. 8h).

### --automatic-updates

|             |                                                 |
|-------------|-------------------------------------------------|
| Type        | <code>string</code>                             |
| Environment | <code>$CODER_WORKSPACE_AUTOMATIC_UPDATES</code> |
| Default     | <code>never</code>                              |

Specify automatic updates setting for the workspace (accepts 'always' or 'never').

### --copy-parameters-from

|             |                                                    |
|-------------|----------------------------------------------------|
| Type        | <code>string</code>                                |
| Environment | <code>$CODER_WORKSPACE_COPY_PARAMETERS_FROM</code> |

Specify the source workspace name to copy parameters from.

### -y, --yes

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Bypass prompts.

### --parameter

|             |                                    |
|-------------|------------------------------------|
| Type        | <code>string-array</code>          |
| Environment | <code>$CODER_RICH_PARAMETER</code> |

Rich parameter value in the format "name=value".

### --rich-parameter-file

|             |                                         |
|-------------|-----------------------------------------|
| Type        | <code>string</code>                     |
| Environment | <code>$CODER_RICH_PARAMETER_FILE</code> |

Specify a file path with values for rich parameters defined in the template. The file should be in YAML format, containing key-value pairs for the parameters.

### --parameter-default

|             |                                            |
|-------------|--------------------------------------------|
| Type        | <code>string-array</code>                  |
| Environment | <code>$CODER_RICH_PARAMETER_DEFAULT</code> |

Rich parameter default values in the format "name=value".

### -O, --org

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>string</code>              |
| Environment | <code>$CODER_ORGANIZATION</code> |

Select which organization (uuid or name) to use.
