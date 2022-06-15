# Coder

[!["GitHub
Discussions"](https://img.shields.io/badge/%20GitHub-%20Discussions-gray.svg?longCache=true&logo=github&colorB=purple)](https://github.com/coder/coder/discussions)
[!["Join us on
Discord"](https://img.shields.io/badge/join-us%20on%20Discord-gray.svg?longCache=true&logo=discord&colorB=purple)](https://discord.gg/coder)
[![Twitter
Follow](https://img.shields.io/twitter/follow/CoderHQ?label=%40CoderHQ&style=social)](https://twitter.com/coderhq)
[![codecov](https://codecov.io/gh/coder/coder/branch/main/graph/badge.svg?token=TNLW3OAP6G)](https://codecov.io/gh/coder/coder)

Coder creates remote development machines so your team can develop from anywhere.

<p align="center">
  <img src="./docs/images/hero-image.png">
</p>

**Manage less**

- Ensure your entire team is using the same tools and resources
  - Rollout critical updates to your developers with one command
- Automatically shut down expensive cloud resources
- Keep your source code and data behind your firewall

**Code more**

- Build and test faster
  - Leveraging cloud CPUs, RAM, network speeds, etc.
- Access your environment from any place on any client (even an iPad)
- Onboard instantly then stay up to date continuously

## Getting Started

> **Note**:
> Coder is in an alpha state. [Report issues here](https://github.com/coder/coder/issues/new).

There are a few ways to install Coder: [install script](https://coder.com/docs/coder-oss/latest/install#installsh) (macOS, Linux), [docker-compose](https://coder.com/docs/coder-oss/latest/install#docker-compose), or [manually](https://coder.com/docs/coder-oss/latest/install#manual) via the latest release (macOS, Windows, and Linux).

If you use the install script, you can preview what occurs during the install process:

```sh
curl -fsSL https://coder.com/install.sh | sh -s -- --dry-run
```

To install, run:

```sh
curl -fsSL https://coder.com/install.sh | sh
```

Once installed, you can run a temporary deployment in dev mode (all data is in-memory and destroyed on exit):

```sh
coder server --dev
```

Use `coder --help` to get a complete list of flags and environment variables. Use our [quickstart guide](https://coder.com/docs/coder-oss/latest/quickstart) for a full walkthrough.

## Documentation

Visit our docs [here](https://coder.com/docs/coder-oss).

## Comparison

Please file [an issue](https://github.com/coder/coder/issues/new) if any information is out of date. Also refer to: [What Coder is not](https://coder.com/docs/coder-oss/latest/about#what-coder-is-not).

| Tool                                                        | Type     | Delivery Model     | Cost                          | Environments                                                                                                                                               |
| :---------------------------------------------------------- | :------- | :----------------- | :---------------------------- | :--------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [Coder](https://github.com/coder/coder)                     | Platform | OSS + Self-Managed | Pay your cloud                | All [Terraform](https://www.terraform.io/registry/providers) resources, all clouds, multi-architecture: Linux, Mac, Windows, containers, VMs, amd64, arm64 |
| [code-server](https://github.com/cdr/code-server)           | Web IDE  | OSS + Self-Managed | Pay your cloud                | Linux, Mac, Windows, containers, VMs, amd64, arm64                                                                                                         |
| [Coder (Classic)](https://coder.com/docs)                   | Platform | Self-Managed       | Pay your cloud + license fees | Kubernetes Linux Containers                                                                                                                                |
| [GitHub Codespaces](https://github.com/features/codespaces) | Platform | SaaS               | 2x Azure Compute              | Linux containers                                                                                                                                           |

---

_Last updated: 5/27/22_

## Community and Support

Join our community on [Discord](https://discord.gg/coder) and [Twitter](https://twitter.com/coderhq)!

[Suggest improvements and report problems](https://github.com/coder/coder/issues/new/choose)

## Contributing

Read the [contributing docs](https://coder.com/docs/coder-oss/latest/CONTRIBUTING).

Find our list of contributors [here](./docs/CONTRIBUTORS.md).
