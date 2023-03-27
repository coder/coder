<!-- DO NOT EDIT | GENERATED CONTENT -->

# dotfiles

Personalize your workspace by applying a canonical dotfiles repository

## Usage

```console
coder dotfiles [flags] <git_repo_url>
```

## Description

```console
  - Check out and install a dotfiles repository without prompts:

      $ coder dotfiles --yes git@github.com:example/dotfiles.git
```

## Options

### --symlink-dir

|             |                                 |
| ----------- | ------------------------------- |
| Type        | <code>string</code>             |
| Environment | <code>$CODER_SYMLINK_DIR</code> |

Specifies the directory for the dotfiles symlink destinations. If empty, will use $HOME.

### -y, --yes

|      |                   |
| ---- | ----------------- |
| Type | <code>bool</code> |

Bypass prompts.
