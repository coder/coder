---
name: Sample Template with Script Ordering
description: Declare execution order between coder_script resources and registry modules with coder_script_order
tags: [local, docker, script-ordering]
icon: /icon/docker.png
---

## Overview

This Coder template demonstrates declarative script ordering with the
[`coder_script_order`](https://coder.com/docs/admin/templates/startup-coordination/script-ordering)
data source. Startup scripts run in parallel by default; ordering rules
gate only the scripts that need it, and no script body contains any
coordination logic.

## What it shows

The template runs five units of startup work:

| Script              | Starts                                      |
|---------------------|---------------------------------------------|
| `configure_mirrors` | Immediately                                 |
| `dotfiles`          | Immediately (referenced by no rule)         |
| `module.git_clone`  | After `configure_mirrors` succeeds          |
| `run_agent`         | After the `git_clone` module succeeds       |
| `report`            | After `run_agent` finishes, even on failure |

Key points:

- The `git_clone` registry module is used as-is. Ordering is declared in
  the template with a `module.git_clone` selector, so every script inside
  the module (including nested modules) is matched without modifying it.
- `requires` defaults to `success`: if the clone fails, `run_agent` is
  skipped and logs the reason.
- `requires = "completion"` on the `report` rule means it always runs
  once `run_agent` reaches a terminal state (succeeded, failed, or
  skipped).

Watch the workspace startup logs to see `configure_mirrors` and
`dotfiles` start at the same time, while the gated scripts start as
their dependencies finish.

## Development

Update the template and push it using the following command:

```shell
./scripts/coder-dev.sh templates push examples-script-ordering \
  -d examples/script-ordering \
  -y
```
