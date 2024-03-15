<!-- DO NOT EDIT | GENERATED CONTENT -->

# templates pull

Download the active, latest, or specified version of a template to a path.

## Usage

```console
coder templates pull [flags] <name> [destination]
```

## Options

### --tar

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Output the template as a tar archive to stdout.

### --zip

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Output the template as a zip archive to stdout.

### --version

|      |                     |
| ---- | ------------------- |
| Type | <code>string</code> |

The name of the template version to pull. Use 'active' to pull the active version, 'latest' to pull the latest version, or the name of the template version to pull.

### -y, --yes

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Bypass prompts.
