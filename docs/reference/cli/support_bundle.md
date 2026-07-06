<!-- DO NOT EDIT | GENERATED CONTENT -->
# support bundle

Generate a support bundle to troubleshoot issues connecting to a workspace.

## Usage

```console
coder support bundle [flags] [<workspace>] [<agent>]
```

## Description

```console
This command generates a file containing detailed troubleshooting information about the Coder deployment and workspace connections. You may specify a single workspace (and optionally an agent name). When run inside a workspace, the workspace and agent are inferred from the environment if not provided.
```

## Options

### -y, --yes

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Bypass confirmation prompts.

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

### --workspace-log-path

|             |                                                       |
|-------------|-------------------------------------------------------|
| Type        | <code>string-array</code>                             |
| Environment | <code>$CODER_SUPPORT_BUNDLE_WORKSPACE_LOG_PATH</code> |

Log file path or glob to collect from inside the remote workspace, resolved against the agent user's home directory. Files local to the machine running this command are not collected. Can be specified multiple times.

### --pprof

|             |                                          |
|-------------|------------------------------------------|
| Type        | <code>bool</code>                        |
| Environment | <code>$CODER_SUPPORT_BUNDLE_PPROF</code> |

Collect pprof profiling data from the Coder server and agent. Requires Coder server version 2.28.0 or newer.
