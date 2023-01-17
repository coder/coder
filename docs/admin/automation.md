# Automation

We recommend automating Coder deployments through the CLI. Examples include [updating templates via CI/CD pipelines](../templates/change-management.md).

## Authentication

Coder uses authentication tokens to grant machine users access to the REST API. Follow the [Authentication](../api/authentication.md) page to learn how to generate long-lived tokens.

## CLI

You can use tokens with the CLI by setting the `--token` CLI flag or the `CODER_SESSION_TOKEN`
environment variable.

```console
export CODER_URL=https://coder.example.com
export CODER_SESSION_TOKEN=*****
coder workspaces ls
```

## REST API

You can review the [API reference](../api/index.md) to find the necessary routes and payload. Alternatively, you can enable the [Swagger](https://swagger.io/) endpoint to read the documentation and do requests against the API:

```console
coder server --swagger-enable
```

By default, the local Swagger endpoint is http://localhost:3000/swagger.

## Golang SDK

Coder publishes a public [Golang SDK](https://pkg.go.dev/github.com/coder/coder/codersdk) for Coder. This is consumed by the [CLI package](https://github.com/coder/coder/tree/main/cli).
