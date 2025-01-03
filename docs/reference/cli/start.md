<!-- DO NOT EDIT | GENERATED CONTENT -->
# start

Start a workspace

## Usage

```console
coder start [flags] <workspace>
```

## Options

### -y, --yes

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Bypass prompts.

### --build-option

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>string-array</code>        |
| Environment | <code>$CODER_BUILD_OPTION</code> |

Build option value in the format "name=value".

### --build-options

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Prompt for one-time build options defined with ephemeral parameters.

### --ephemeral-parameter

|             |                                         |
|-------------|-----------------------------------------|
| Type        | <code>string-array</code>               |
| Environment | <code>$CODER_EPHEMERAL_PARAMETER</code> |

Set the value of ephemeral parameters defined in the template. The format is "name=value".

### --prompt-ephemeral-parameters

|             |                                                 |
|-------------|-------------------------------------------------|
| Type        | <code>bool</code>                               |
| Environment | <code>$CODER_PROMPT_EPHEMERAL_PARAMETERS</code> |

Prompt to set values of ephemeral parameters defined in the template. If a value has been set via --ephemeral-parameter, it will not be prompted for.

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

### --always-prompt

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Always prompt all parameters. Does not pull parameter values from existing workspace.
