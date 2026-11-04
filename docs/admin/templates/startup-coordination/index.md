---
title: Workspace startup coordination
---

We recommend using [declarative script ordering](../extending-templates/script-ordering.md) instead of `coder exp sync` to order whole startup and shutdown scripts.

As a template author, you can use the experimental `coder exp sync` commands to coordinate named units of work within startup scripts.
You can mark a unit complete while its script continues other work.

> [!NOTE]
> The `coder exp sync` commands are experimental and might change without notice.

## Why use this?

Use `coder exp sync` when another script needs only part of a startup script's work to finish.
For example, a script can mark dependency installation complete, then continue building an application.
Scripts that need the installed dependencies can proceed while the build runs.

## Quick start

For an existing `git-clone` unit that checks out a Node.js project at `~/app`, add these commands to a startup script:

```sh
set -e

coder exp sync want install-dependencies git-clone
coder exp sync start install-dependencies
cd ~/app
npm ci
coder exp sync complete install-dependencies

npm run build
```

The script waits for `git-clone` to complete before installing dependencies.
After installation succeeds, it marks `install-dependencies` complete and continues with `npm run build`.
Other scripts that depend on `install-dependencies` can proceed without waiting for the build to finish.

For more information, refer to the [usage documentation](./usage.md), [troubleshooting documentation](./troubleshooting.md), or view our [examples](./example.md).
