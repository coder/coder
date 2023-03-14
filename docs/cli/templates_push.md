
# push

 
Push a new template version from the current directory or as specified by flag


## Usage
```console
push [template]
```


## Options
### --test.provisioner, -p
Customize the provisioner backend
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Customize the provisioner backend&lt;/code&gt; |
| Default |     &lt;code&gt;terraform&lt;/code&gt; |



### --parameter-file
Specify a file path with parameter values.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Specify a file path with parameter values.&lt;/code&gt; |

### --variables-file
Specify a file path with values for Terraform-managed variables.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Specify a file path with values for Terraform-managed variables.&lt;/code&gt; |

### --variable
Specify a set of values for Terraform-managed variables.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Specify a set of values for Terraform-managed variables.&lt;/code&gt; |

### --provisioner-tag, -t
Specify a set of tags to target provisioner daemons.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Specify a set of tags to target provisioner daemons.&lt;/code&gt; |

### --name
Specify a name for the new template version. It will be automatically generated if not provided.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Specify a name for the new template version. It will be automatically generated if not provided.&lt;/code&gt; |

### --always-prompt
Always prompt all parameters. Does not pull parameter values from active template version
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Always prompt all parameters. Does not pull parameter values from active template version&lt;/code&gt; |

### --yes, -y
Bypass prompts
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Bypass prompts&lt;/code&gt; |

### --directory, -d
Specify the directory to create from, use &#39;-&#39; to read tar from stdin
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Specify the directory to create from, use &#39;-&#39; to read tar from stdin&lt;/code&gt; |
| Default |     &lt;code&gt;.&lt;/code&gt; |


