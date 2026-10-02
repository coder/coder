---
title: Backstage
---

[Backstage](https://backstage.io) is an open platform for building developer
portals. Coder provides a set of official
[Backstage plugins](https://github.com/coder/backstage-plugins) so teams can
surface and manage Coder workspaces directly from their Backstage instance.

## What's included

The [coder/backstage-plugins](https://github.com/coder/backstage-plugins)
repository includes:

- **backstage-plugin-coder**: A frontend plugin for integrating Coder
  workspaces with Backstage.
- **auth-backend-module-coder-provider**: A backend authentication module for
  Coder OAuth2 integration with Backstage's New Backend System.
- **backstage-plugin-devcontainers-backend**: A backend plugin for
  integrating VS Code Dev Containers with Backstage catalog items (no Coder
  deployment necessary).
- **backstage-plugin-devcontainers-react**: A frontend plugin for detecting
  and working with Dev Container repo data, letting you open repos in VS Code
  with a full Dev Containers setup (no Coder deployment necessary).

## Installation and usage

Installation, configuration, and usage instructions for each plugin are
maintained in the
[coder/backstage-plugins README](https://github.com/coder/backstage-plugins),
along with release and support information.

For questions, bug reports, or feature requests, open an issue in the
[coder/backstage-plugins](https://github.com/coder/backstage-plugins/issues)
repository.
