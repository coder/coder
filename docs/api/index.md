Get started with the Coder API:

## Quickstart

Generate a token on your Coder deployment by visiting:

```sh
https://coder.example.com/settings/tokens
```

List your workspaces

```sh
# CLI
curl https://coder.example.com/api/v2/workspaces?q=owner:me \
-H "Coder-Session-Token: <your-token>"
```

## Sections

<children>
  This page is rendered on https://coder.com/docs/coder-oss/api. Refer to the other documents in the `api/` directory.
</children>
