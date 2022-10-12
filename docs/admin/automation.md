# Automation

We recommend automating Coder deployments through the CLI. Examples include [updating templates via CI/CD pipelines](../templates/change-management.md).

## Tokens

Long-lived tokens can be generated to perform actions on behalf of your user account:

```sh
coder tokens create
```

## CLI

You can use tokens with the CLI by setting the `--token` CLI flag or the `CODER_SESSION_TOKEN`
environment variable.

```sh
export CODER_URL=https://coder.example.com
export CODER_SESSION_TOKEN=*****
coder workspaces ls
```

## REST API

You can use tokens with the Coder's REST API using the `Coder-Session-Token` HTTP header.

```sh
curl 'https://dev.coder.com/api/v2/workspaces' \
  -H 'Coder-Session-Token: *****'
```

> At this time, we do not publish an API reference. However, [codersdk](https://github.com/coder/coder/tree/main/codersdk) can be grepped to find the necessary routes and payloads.

## Golang SDK

Coder publishes a public [Golang SDK](https://pkg.go.dev/github.com/coder/coder@main/codersdk) for Coder. This is consumed by the [CLI package](https://github.com/coder/coder/tree/main/cli).
