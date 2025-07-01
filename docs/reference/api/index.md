# API

Get started with the Coder API:

## Quickstart

Generate a token on your Coder deployment by visiting:

````shell
https://coder.example.com/settings/tokens
````

List your workspaces

````shell
# CLI
curl https://coder.example.com/api/v2/workspaces?q=owner:me \
-H "Coder-Session-Token: <your-token>"
````

## Use cases

See some common [use cases](../../reference/index.md#use-cases) for the REST API.

## Sections

<children>
  This page is rendered on https://coder.com/docs/reference/api. Refer to the other documents in the `api/` directory.
</children>
