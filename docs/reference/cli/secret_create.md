---
title: secret create
description: Create a secret
---

<!-- DO NOT EDIT | GENERATED CONTENT -->

Create a secret

## Usage

```console
coder secret create [flags] <name>
```

## Description

```console
Provide the secret value with --value or non-interactive stdin (pipe or redirect).
```

## Options

### --value

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Set the secret value. For security reasons, prefer non-interactive stdin (pipe or redirect).

### --description

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Set the secret description.

### --env

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Name of the workspace environment variable that this secret will set.

### --file

|      |                     |
|------|---------------------|
| Type | <code>string</code> |

Workspace file path where this secret will be written. Must start with ~/ or /.

### --enabled

|         |                   |
|---------|-------------------|
| Type    | <code>bool</code> |
| Default | <code>true</code> |

Whether the secret is injected into workspaces. An enabled secret must set --env or --file; pass --enabled=false to store a secret without injecting it.
