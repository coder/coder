# landjail Jail Type

> [!NOTE]
> Agent Firewall requires the [AI Governance Add-On](../ai-governance.md).
> As of Coder v2.32, deployments without the add-on will not be able to
> access Agent Firewall.

landjail is Agent Firewall's alternative jail type that uses Landlock V4 for
network isolation.

## Overview

Agent Firewall uses Landlock V4 to enforce network restrictions:

- All `bind` syscalls are forbidden
- All `connect` syscalls are forbidden except to the port that is used by http
  proxy

This provides network isolation without requiring network namespace capabilities
or special Docker permissions.
