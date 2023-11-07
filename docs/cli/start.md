<!-- DO NOT EDIT | GENERATED CONTENT -->

# start

Start a workspace

## Usage

```console
coder start [flags] <workspace>
```

## Options

### --always-prompt

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Always prompt all parameters. Does not pull parameter values from existing workspace.

### --build-option

|             |                                  |
| ----------- | -------------------------------- |
| Type        | <code>string-array</code>        |
| Environment | <code>$CODER_BUILD_OPTION</code> |

Build option value in the format "name=value".

### --build-options

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Prompt for one-time build options defined with ephemeral parameters.

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

### -y, --yes

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Bypass prompts.
