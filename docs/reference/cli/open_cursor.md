<!-- DO NOT EDIT | GENERATED CONTENT -->
# open cursor

Open a workspace in Cursor Desktop

## Usage

```console
coder open cursor [flags] <workspace> [<directory in workspace>]
```

## Options

### --generate-token

|             |                                                |
|-------------|------------------------------------------------|
| Type        | <code>bool</code>                              |
| Environment | <code>$CODER_OPEN_CURSOR_GENERATE_TOKEN</code> |

Generate an auth token and include it in the cursor:// URI. This is for automagical configuration of Cursor Desktop and not needed if already configured. This flag does not need to be specified when running this command on a local machine unless automatic open fails.
