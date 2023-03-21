<!-- DO NOT EDIT | GENERATED CONTENT -->

# templates edit

Edit the metadata of a template by name.

## Usage

```console
edit <template> [flags]
```

## Options

### --name

|     |     |
| --- | --- |

Edit the template name.

### --display-name

|     |     |
| --- | --- |

Edit the template display name.

### --description

|     |     |
| --- | --- |

Edit the template description.

### --icon

|     |     |
| --- | --- |

Edit the template icon path.

### --default-ttl

|     |     |
| --- | --- |

Edit the template default time before shutdown - workspaces created from this template default to this value.

### --max-ttl

|     |     |
| --- | --- |

Edit the template maximum time before shutdown - workspaces created from this template must shutdown within the given duration after starting. This is an enterprise-only feature.

### --allow-user-cancel-workspace-jobs

|         |                   |
| ------- | ----------------- |
| Default | <code>true</code> |

Allow users to cancel in-progress workspace jobs.

### --yes, -y

|     |     |
| --- | --- |

Bypass prompts.
