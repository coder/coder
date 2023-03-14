
# config-ssh

 
Add an SSH Host entry for your workspaces &#34;ssh coder.workspace&#34;


## Usage
```console
config-ssh
```

## Description
```console
  - You can use -o (or --ssh-option) so set SSH options to be used for all your 
    workspaces:                                                                 

      $ coder config-ssh -o ForwardAgent=yes 

  - You can use --dry-run (or -n) to see the changes that would be made:        

      $ coder config-ssh --dry-run 
```


## Options
### --ssh-config-file
Specifies the path to an SSH config.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Specifies the path to an SSH config.&lt;/code&gt; |
| Default |     &lt;code&gt;~/.ssh/config&lt;/code&gt; |



### --ssh-option, -o
Specifies additional SSH options to embed in each host stanza.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Specifies additional SSH options to embed in each host stanza.&lt;/code&gt; |

### --dry-run, -n
Perform a trial run with no changes made, showing a diff at the end.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Perform a trial run with no changes made, showing a diff at the end.&lt;/code&gt; |

### --skip-proxy-command
Specifies whether the ProxyCommand option should be skipped. Useful for testing.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Specifies whether the ProxyCommand option should be skipped. Useful for testing.&lt;/code&gt; |

### --use-previous-options
Specifies whether or not to keep options from previous run of config-ssh.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Specifies whether or not to keep options from previous run of config-ssh.&lt;/code&gt; |

### --yes, -y
Bypass prompts
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Bypass prompts&lt;/code&gt; |
