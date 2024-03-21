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

### --confirm

|             |                                            |
| ----------- | ------------------------------------------ |
| Type        | <code>bool</code>                          |
| Environment | <code>$CODER_SUPPORT_BUNDLE_CONFIRM</code> |

By setting this, you confirm that you will treat the resulting support bundle as if it contained sensitive information.

### -O, --output-file

|             |                                                |
| ----------- | ---------------------------------------------- |
| Type        | <code>string</code>                            |
| Environment | <code>$CODER_SUPPORT_BUNDLE_OUTPUT_FILE</code> |

File path for writing the generated support bundle. Defaults to coder-support-$(date +%s).zip.

### --url-override

|             |                                                 |
| ----------- | ----------------------------------------------- |
| Type        | <code>string</code>                             |
| Environment | <code>$CODER_SUPPORT_BUNDLE_URL_OVERRIDE</code> |

Override the URL to your Coder deployment. This may be useful, for example, if you need to troubleshoot a specific Coder replica.
