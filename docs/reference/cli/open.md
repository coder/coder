<!-- DO NOT EDIT | GENERATED CONTENT -->
# open

Open a workspace in an IDE or workspace app. If no app slug is provided, lists available apps in <workspace>.

## Usage

```console
coder open [flags] <workspace> [<app slug>]
```

## Subcommands

| Name                                    | Purpose                             |
|-----------------------------------------|-------------------------------------|
| [<code>vscode</code>](./open_vscode.md) | Open a workspace in VS Code Desktop |

## Options

### --region

|             |                                 |
|-------------|---------------------------------|
| Type        | <code>string</code>             |
| Environment | <code>$CODER_OPEN_REGION</code> |
| Default     | <code>primary</code>            |

Region to use when opening the app. By default, the app will be opened using the main Coder deployment (a.k.a. "primary"). This has no effect on external application URLs.
