<!-- DO NOT EDIT | GENERATED CONTENT -->

# coder dotfiles

Checkout and install a dotfiles repository from a Git URL

## Usage

```console
coder dotfiles [git_repo_url] [flags]
```

## Examples

```console
  - Check out and install a dotfiles repository without prompts:

      $ coder dotfiles --yes git@github.com:example/dotfiles.git
```

## Flags

### --symlink-dir

Specifies the directory for the dotfiles symlink destinations. If empty will use $HOME.
<br/>
| | |
| --- | --- |
| Consumes | <code>$CODER_SYMLINK_DIR</code> |

### --yes, -y

Bypass prompts
<br/>
| | |
| --- | --- |
| Default | <code>false</code> |
