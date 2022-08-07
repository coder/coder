# Configuring Port Forwarding

There are two ways to forward ports:

- The Coder CLI port-forward command
- SSH

## The Coder CLI and port-forward

For example:

```console
coder port-forward mycoderworkspacename --tcp 8000:8000
```

For more examples, type `coder port-forward --help`

## SSH

Use the Coder CLI to first [configure SSH](../ides.md#ssh-configuration) on your
local machine. Then run `ssh`. For example:

```console
ssh -L 8000:localhost:8000 coder.mycoderworkspacename 
```

## Accessing the forwarded port

After completing either Port Forwarding method, open a web browser on your local
machine to access the Coder workspace process.

```console
http://localhost:<yourforwardedport>
```
