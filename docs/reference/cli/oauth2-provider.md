---
title: oauth2-provider
description: Manage Coder OAuth2 provider settings
---

<!-- DO NOT EDIT | GENERATED CONTENT -->

Manage Coder OAuth2 provider settings

## Usage

```console
coder oauth2-provider
```

## Description

```console
Administrators can use these commands to change OAuth2 provider settings.
  - Enable dynamic client registration (RFC 7591), allowing OAuth2/MCP clients to
self-register without an admin creating an app first:

     $ coder oauth2-provider dcr enable

  - Disable dynamic client registration. Clients that already registered are
unaffected; only new self-registration attempts are rejected:

     $ coder oauth2-provider dcr disable
```

## Subcommands

| Name                                         | Purpose                                              |
|----------------------------------------------|------------------------------------------------------|
| [<code>dcr</code>](./oauth2-provider_dcr.md) | Manage OAuth2 dynamic client registration (RFC 7591) |
