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

Bypass prompts.

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

### --workspaces-total-cap

|             |                                                         |
|-------------|---------------------------------------------------------|
| Type        | <code>int</code>                                        |
| Environment | <code>$CODER_SUPPORT_BUNDLE_WORKSPACES_TOTAL_CAP</code> |

Maximum number of workspaces to include in the support bundle. Set to 0 or negative value to disable the cap. Defaults to 10.

### --template

|             |                                             |
|-------------|---------------------------------------------|
| Type        | <code>string</code>                         |
| Environment | <code>$CODER_SUPPORT_BUNDLE_TEMPLATE</code> |

Template name to include in the support bundle. Use org_name/template_name if template name is reused across multiple organizations.

### --pprof

|             |                                          |
|-------------|------------------------------------------|
| Type        | <code>bool</code>                        |
| Environment | <code>$CODER_SUPPORT_BUNDLE_PPROF</code> |

Collect pprof profiling data from the Coder server and agent. Requires Coder server version 2.28.0 or newer.
