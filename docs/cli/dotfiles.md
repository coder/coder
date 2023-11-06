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

### -b, --branch

|      |                     |
| ---- | ------------------- |
| Type | <code>string</code> |

Specifies which branch to clone. If empty, will default to cloning the default branch or using the existing branch in the cloned repo on disk.

### --repo-dir

|             |                                       |
| ----------- | ------------------------------------- |
| Type        | <code>string</code>                   |
| Environment | <code>$CODER_DOTFILES_REPO_DIR</code> |
| Default     | <code>dotfiles</code>                 |

Specifies the directory for the dotfiles repository, relative to global config directory.

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
