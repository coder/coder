# landjail Jail Type

landjail is Agent Boundaries' alternative jail type that uses Landlock V4 for
network isolation.

## Overview

Agent Boundaries uses Landlock V4 to enforce network restrictions:

- All `bind` syscalls are forbidden
- All `connect` syscalls are forbidden except to the port that is used by http
  proxy

This provides network isolation without requiring network namespace capabilities
or special Docker permissions.
