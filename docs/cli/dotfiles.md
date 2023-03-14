
# dotfiles

 
Checkout and install a dotfiles repository from a Git URL


## Usage
```console
dotfiles &lt;git_repo_url&gt;
```

## Description
```console
  - Check out and install a dotfiles repository without prompts:                

      $ coder dotfiles --yes git@github.com:example/dotfiles.git 
```


## Options
### --symlink-dir
Specifies the directory for the dotfiles symlink destinations. If empty will use $HOME.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Specifies the directory for the dotfiles symlink destinations. If empty will use $HOME.&lt;/code&gt; |

### --yes, -y
Bypass prompts
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Bypass prompts&lt;/code&gt; |
