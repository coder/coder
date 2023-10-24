---
name: Develop in Linux on Exoscale VM
description: Get started with Linux development on Exoscale.
maintainer_github: WhizUs
tags: [cloud, exoscale]
icon: /icon/exoscale.png
---

# exoscale-linux

To get started, run `coder templates init`. When prompted, select this template.
Follow the on-screen instructions to proceed.

## Authentication

### using env vars

You can set 2 environment variables at your coder server to connect to the exoscale api:

- `EXOSCALE_API_KEY`
- `EXOSCALE_API_SECRET`

You can create a key/secret pair in your [exoscale portal](https://portal.exoscale.com/)

### using terraform variables

- In the coder GUI you can go to the template settings -> variables and set key and secret there
- use a `.creds.yaml` in combination with the `--variables-file` flag
  - `coder template create  --variables-file .creds.yaml`

## code-server

`code-server` is installed via the [`code-serer`](https://registry.coder.com/modules/code-server) module. The `coder_app` resource is defined to access `code-server` through
the dashboard UI over `localhost:13337`.
