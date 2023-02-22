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


## Local Flags
| Name |  Default | Usage | Environment | 
| ---- |  ------- | ----- | -------- |
| --symlink-dir | |<code>Specifies the directory for the dotfiles symlink destinations. If empty will use $HOME.</code> | <code>$CODER_SYMLINK_DIR</code>  |
| --yes, -y |false |<code>Bypass prompts</code> | |