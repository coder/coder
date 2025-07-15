<!-- DO NOT EDIT | GENERATED CONTENT -->
# templates archive

Archive unused or failed template versions from a given template(s)

## Usage

```console
coder templates archive [flags] [template-name...] 
```

## Options

### -y, --yes

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Bypass prompts.

### --all

|      |                   |
|------|-------------------|
| Type | <code>bool</code> |

Include all unused template versions. By default, only failed template versions are archived.

### -O, --org

|             |                                  |
|-------------|----------------------------------|
| Type        | <code>string</code>              |
| Environment | <code>$CODER_ORGANIZATION</code> |

Select which organization (uuid or name) to use.
