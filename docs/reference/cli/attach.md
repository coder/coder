<!-- DO NOT EDIT | GENERATED CONTENT -->
# attach

Create a workspace and attach an external agent to it

## Usage

```console
coder attach [flags] [workspace]
```

## Description

```console
  - Attach an external agent to a workspace:

     $ coder attach my-workspace --template externally-managed-workspace --output text
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
