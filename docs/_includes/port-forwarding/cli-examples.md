Forward the remote TCP port `8080` to local port `8000`:

```console
coder port-forward myworkspace --tcp 8000:8080
```

Forward the remote TCP port `3000` and all ports from `9990` to `9999` to their respective local ports.

```console
coder port-forward myworkspace --tcp 3000,9990-9999
```
