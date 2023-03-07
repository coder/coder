<!-- DO NOT EDIT | GENERATED CONTENT -->

# coder update

Will update and start a given workspace if it is out of date. Use --always-prompt to change the parameter values of the workspace.

## Usage

```console
coder update <workspace> [flags]
```

## Flags

### --always-prompt

Always prompt all parameters. Does not pull parameter values from existing workspace
<br/>
| | |
| --- | --- |
| Default | <code>false</code> |

### --parameter-file

Specify a file path with parameter values.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_PARAMETER_FILE</code> |

### --rich-parameter-file

Specify a file path with values for rich parameters defined in the template.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_RICH_PARAMETER_FILE</code> |
