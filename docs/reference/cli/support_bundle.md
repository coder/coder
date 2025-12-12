<!-- DO NOT EDIT | GENERATED CONTENT -->
# support bundle

Generate a support bundle to troubleshoot issues connecting to a workspace.

## Usage

```console
coder support bundle [flags] <workspace> [<agent>]
```

## Description

```console
This command generates a file containing detailed troubleshooting information about the Coder deployment and workspace connections. You must specify a single workspace (and optionally an agent name).
```

## Options

### -y, --yes

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Run in non-interactive mode. Accepts default values and fails on required inputs.

### -O, --output-file

|             |                                                |
|-------------|------------------------------------------------|
| Type        | <code>string</code>                            |
| Environment | <code>$CODER_SUPPORT_BUNDLE_OUTPUT_FILE</code> |

File path for writing the generated support bundle. Defaults to coder-support-$(date +%s).zip.

### --url-override

|             |                                                 |
|-------------|-------------------------------------------------|
| Type        | <code>string</code>                             |
| Environment | <code>$CODER_SUPPORT_BUNDLE_URL_OVERRIDE</code> |

Override the URL to your Coder deployment. This may be useful, for example, if you need to troubleshoot a specific Coder replica.
