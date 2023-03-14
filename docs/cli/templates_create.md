<!-- DO NOT EDIT | GENERATED CONTENT -->
# create

 
Create a template from the current directory or as specified by flag


## Usage
```console
create [name]
```


## Options
### --parameter-file
 
| | |
| --- | --- |

Specify a file path with parameter values.
### --variables-file
 
| | |
| --- | --- |

Specify a file path with values for Terraform-managed variables.
### --variable
 
| | |
| --- | --- |

Specify a set of values for Terraform-managed variables.
### --provisioner-tag
 
| | |
| --- | --- |

Specify a set of tags to target provisioner daemons.
### --default-ttl
 
| | |
| --- | --- |
| Default |     <code>24h</code> |



Specify a default TTL for workspaces created from this template.
### --directory, -d
 
| | |
| --- | --- |
| Default |     <code>.</code> |



Specify the directory to create from, use '-' to read tar from stdin
### --test.provisioner
 
| | |
| --- | --- |
| Default |     <code>terraform</code> |



Customize the provisioner backend
### --yes, -y
 
| | |
| --- | --- |
| Environment | <code>$CODER_SKIP_PROMPT</code> |

Bypass prompts