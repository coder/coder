<!-- DO NOT EDIT | GENERATED CONTENT -->

# coder create

Create a workspace

## Usage

```console
coder create [name] [flags]
```

## Flags

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

### --start-at

Specify the workspace autostart schedule. Check `coder schedule start --help` for the syntax.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_WORKSPACE_START_AT</code> |

### --stop-after

Specify a duration after which the workspace should shut down (e.g. 8h).
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_WORKSPACE_STOP_AFTER</code> |
| Default | <code>8h0m0s</code> |

### --template, -t

Specify a template name.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_TEMPLATE_NAME</code> |

### --yes, -y

Bypass prompts
<br/>
| | |
| --- | --- |
| Default | <code>false</code> |
