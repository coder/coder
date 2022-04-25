# Coder

[!["GitHub Discussions"](https://img.shields.io/badge/%20GitHub-%20Discussions-gray.svg?longCache=true&logo=github&colorB=purple)](https://github.com/coder/coder/discussions) [!["Join us on Slack"](https://img.shields.io/badge/join-us%20on%20slack-gray.svg?longCache=true&logo=slack&colorB=brightgreen)](https://coder.com/community) [![Twitter Follow](https://img.shields.io/twitter/follow/CoderHQ?label=%40CoderHQ&style=social)](https://twitter.com/coderhq) [![codecov](https://codecov.io/gh/coder/coder/branch/main/graph/badge.svg?token=TNLW3OAP6G)](https://codecov.io/gh/coder/coder)

Provision remote development environments with Terraform.

## Highlights

- Automate development environments for Linux, Windows, and MacOS in your cloud
- Start writing code with a single command
- Use one of many [examples](./examples) to get started

## Installing Coder

Install [the latest release](https://github.com/coder/coder/releases) on a system with
at least 2 CPU cores and 2 GB RAM.

To tinker, start with dev-mode (all data is in-memory, and is destroyed on exit):

```bash
$ coder server --dev
```

To run a production deployment with PostgreSQL:

```bash
$ CODER_PG_CONNECTION_URL="postgres://<username>@<host>/<database>?password=<password>" \
    coder server
```

To run as a system service, install with `.deb` (Debian, Ubuntu) or `.rpm` (Fedora, CentOS, RHEL, SUSE):

```bash
# Edit the configuration!
$ sudo vim /etc/coder.d/coder.env
$ sudo service coder restart
```

Reference `coder start --help` for a complete list of flags and environment variables.

### Your First Workspace

In a new terminal, create a new template (eg. Develop in Linux on Google Cloud):

```
$ coder templates init
$ coder templates create
```

Create a new workspace and connect via SSH:

```
$ coder workspaces create my-first-workspace
$ coder ssh my-first-workspace
```

### Modifying Templates

You can edit the Terraform from a sample template:

```sh
$ coder templates init
$ cd gcp-linux/
$ vim main.tf
$ coder templates update gcp-linux
```

## Documentation

Some pages are coming soon. Contributions welcome!

- [About Coder](./about.md#about-coder)
  - [Why remote development](about.md#why-remote-development)
  - [Why Coder](about.md#why-coder)
  - [What Coder is not](about.md#what-coder-is-not)
- [Templates](./templates.md)
  - [Managing templates](./templates.md#managing-templates)
  - [Persistant and ephemeral resources](./templates.md#persistant-and-ephemeral-resources)
  - [Variables](./templates.md#variables)
- Workspaces
- 
- Guides
  - Using the Coder CLI
  - Install Coder on a VM with Caddy + LetsEncrypt
  - Building templates in Coder

## Contributing

Read the [contributing docs](./CONTRIBUTING.md).

