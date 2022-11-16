# Coder

[!["Join us on
Discord"](https://img.shields.io/badge/join-us%20on%20Discord-gray.svg?longCache=true&logo=discord&colorB=green)](https://coder.com/chat?utm_source=github.com/coder/coder&utm_medium=github&utm_campaign=readme.md)
[![codecov](https://codecov.io/gh/coder/coder/branch/main/graph/badge.svg?token=TNLW3OAP6G)](https://codecov.io/gh/coder/coder)
[![Go Reference](https://pkg.go.dev/badge/github.com/coder/coder.svg)](https://pkg.go.dev/github.com/coder/coder)
[![Twitter
Follow](https://img.shields.io/twitter/follow/coderhq?label=%40coderhq&style=social)](https://twitter.com/coderhq)

Software development on your infrastructure. Offload your team's development from local workstations to cloud servers. Onboard developers in minutes. Build, test and compile at the speed of the cloud. Keep your source code and data behind your firewall.

> "By leveraging Terraform, Coder lets developers run any IDE on any compute platform including on-prem, AWS, Azure, GCP, DigitalOcean, Kubernetes, Docker, and more, with workspaces running on Linux, Windows, or Mac." - **Kevin Fishner Chief of Staff at [HashiCorp](https://hashicorp.com/)**


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

## Recommended Reading

- [How our development team shares one giant bare metal machine](https://coder.com/blog/how-our-development-team-shares-one-giant-bare-metal-machine?utm_source=github.com/coder/coder&utm_medium=github&utm_campaign=readme.md)
- [Laptop development is dead: why remote development is the future](https://medium.com/@elliotgraebert/laptop-development-is-dead-why-remote-development-is-the-future-f92ce103fd13)
- [Learn how Palantir improved build times by 78% with coder](https://blog.palantir.com/the-benefits-of-remote-ephemeral-workspaces-1a1251ed6e53).
- [A software development environment is not just a container](https://coder.com/blog/not-a-container?utm_source=github.com/coder/coder&utm_medium=github&utm_campaign=readme.md).
- [What Coder is not](https://coder.com/docs/coder-oss/latest/index#what-coder-is-not?utm_source=github.com/coder/coder&utm_medium=github&utm_campaign=readme.md).

## Getting Started

The easiest way to install Coder is to use our
[install script](https://github.com/coder/coder/blob/main/install.sh) for Linux
and macOS. For Windows, use the latest `..._installer.exe` file from GitHub
Releases.

To install, run:

```bash
curl -L https://coder.com/install.sh | sh
```

You can preview what occurs during the install process:

```bash
curl -L https://coder.com/install.sh | sh -s -- --dry-run
```

You can modify the installation process by including flags. Run the help command for reference:

```bash
curl -L https://coder.com/install.sh | sh -s -- --help
```

> See [install](docs/install) for additional methods.

Once installed, you can start a production deployment<sup>1</sup> with a single command:

```sh
# Automatically sets up an external access URL on *.try.coder.app
coder server

# Requires a PostgreSQL instance (version 13 or higher) and external access URL
coder server --postgres-url <url> --access-url <url>
```

> <sup>1</sup> The embedded database is great for trying out Coder with small deployments, but do consider using an external database for increased assurance and control.

Use `coder --help` to get a complete list of flags and environment variables. Use our [quickstart guide](https://coder.com/docs/coder-oss/latest/quickstart) for a full walkthrough.

## Documentation

Visit our docs [here](https://coder.com/docs/coder-oss).

## Comparison

Please file [an issue](https://github.com/coder/coder/issues/new) if any information is out of date. Also refer to: [What Coder is not](https://coder.com/docs/coder-oss/latest/index#what-coder-is-not).

| Tool                                                        | Type     | Delivery Model     | Cost                          | Environments                                                                                                                                               |
| :---------------------------------------------------------- | :------- | :----------------- | :---------------------------- | :--------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [Coder](https://github.com/coder/coder)                     | Platform | OSS + Self-Managed | Pay your cloud                | All [Terraform](https://www.terraform.io/registry/providers) resources, all clouds, multi-architecture: Linux, Mac, Windows, containers, VMs, amd64, arm64 |
| [code-server](https://github.com/cdr/code-server)           | Web IDE  | OSS + Self-Managed | Pay your cloud                | Linux, Mac, Windows, containers, VMs, amd64, arm64                                                                                                         |
| [Coder (Classic)](https://coder.com/docs)                   | Platform | Self-Managed       | Pay your cloud + license fees | Kubernetes Linux Containers                                                                                                                                |
| [GitHub Codespaces](https://github.com/features/codespaces) | Platform | SaaS               | 2x Azure Compute              | Linux Virtual Machines                                                                                                                                           |

---

_Last updated: 5/27/22_

## Community and Support

Join our community on [Discord](https://coder.com/chat?utm_source=github.com/coder/coder&utm_medium=github&utm_campaign=readme.md) and [Twitter](https://twitter.com/coderhq)!

[Suggest improvements and report problems](https://github.com/coder/coder/issues/new/choose)

## Contributing

If you're using Coder in your organization, please try to add your company name to the [ADOPTERS.md](./ADOPTERS.md). It really helps the project to gain momentum and credibility. It's a small contribution back to the project with a big impact.

Read the [contributing docs](https://coder.com/docs/coder-oss/latest/CONTRIBUTING).

Find our list of contributors [here](https://github.com/coder/coder/graphs/contributors).
