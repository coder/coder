<!-- DO NOT EDIT | GENERATED CONTENT -->
# dotfiles

 
Checkout and install a dotfiles repository from a Git URL


## Usage
```console
coder dotfiles <git_repo_url>
```

## Description
```console
  - Check out and install a dotfiles repository without prompts.:               

      $ coder dotfiles --yes git@github.com:example/dotfiles.git 
```


## Options
### --symlink-dir
 
| | |
| --- | --- |
| Environment | <code>$CODER_SYMLINK_DIR</code> |

Specifies the directory for the dotfiles symlink destinations. If empty, will use $HOME.
### --yes, -y
 
| | |
| --- | --- |

Bypass prompts.