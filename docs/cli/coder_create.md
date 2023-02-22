<!-- DO NOT EDIT | GENERATED CONTENT -->
# coder create

Create a workspace
## Usage
```console
coder create [name] [flags]
```


## Local Flags
| Name |  Default | Usage |
| ---- |  ------- | ----- |
| --parameter-file |  | <code>Specify a file path with parameter values.<br/>Consumes $CODER_PARAMETER_FILE</code>|
| --rich-parameter-file |  | <code>Specify a file path with values for rich parameters defined in the template.<br/>Consumes $CODER_RICH_PARAMETER_FILE</code>|
| --start-at |  | <code>Specify the workspace autostart schedule. Check `coder schedule start --help` for the syntax.<br/>Consumes $CODER_WORKSPACE_START_AT</code>|
| --stop-after | 8h0m0s | <code>Specify a duration after which the workspace should shut down (e.g. 8h).<br/>Consumes $CODER_WORKSPACE_STOP_AFTER</code>|
| --template, -t |  | <code>Specify a template name.<br/>Consumes $CODER_TEMPLATE_NAME</code>|
| --yes, -y | false | <code>Bypass prompts</code>|