
# create-admin-user

 
Create a new admin user with the given username, email and password and adds it to every organization.


## Usage
```console
create-admin-user
```


## Options
### --postgres-url
URL of a PostgreSQL database. If empty, the built-in PostgreSQL deployment will be used (Coder must not be already running in this case).
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;URL of a PostgreSQL database. If empty, the built-in PostgreSQL deployment will be used (Coder must not be already running in this case).&lt;/code&gt; |

### --ssh-keygen-algorithm
The algorithm to use for generating ssh keys. Accepted values are &#34;ed25519&#34;, &#34;ecdsa&#34;, or &#34;rsa4096&#34;.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;The algorithm to use for generating ssh keys. Accepted values are &#34;ed25519&#34;, &#34;ecdsa&#34;, or &#34;rsa4096&#34;.&lt;/code&gt; |
| Default |     &lt;code&gt;ed25519&lt;/code&gt; |



### --username
The username of the new user. If not specified, you will be prompted via stdin.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;The username of the new user. If not specified, you will be prompted via stdin.&lt;/code&gt; |

### --email
The email of the new user. If not specified, you will be prompted via stdin.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;The email of the new user. If not specified, you will be prompted via stdin.&lt;/code&gt; |

### --password
The password of the new user. If not specified, you will be prompted via stdin.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;The password of the new user. If not specified, you will be prompted via stdin.&lt;/code&gt; |

### --raw-url
Output the raw connection URL instead of a psql command.
<br/>
| | |
| --- | --- |
| Consumes | &lt;code&gt;Output the raw connection URL instead of a psql command.&lt;/code&gt; |
| Default |     &lt;code&gt;false&lt;/code&gt; |


