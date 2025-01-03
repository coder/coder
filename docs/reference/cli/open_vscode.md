<!-- DO NOT EDIT | GENERATED CONTENT -->
# open vscode

Open a workspace in VS Code Desktop

## Usage

```console
coder open vscode [flags] <workspace> [<directory in workspace>]
```

## Options

### --generate-token

|             |                                                |
|-------------|------------------------------------------------|
| Type        | <code>bool</code>                              |
| Environment | <code>$CODER_OPEN_VSCODE_GENERATE_TOKEN</code> |

Generate an auth token and include it in the vscode:// URI. This is for automagical configuration of VS Code Desktop and not needed if already configured. This flag does not need to be specified when running this command on a local machine unless automatic open fails.
