# Coder

[!["GitHub Discussions"](https://img.shields.io/badge/%20GitHub-%20Discussions-gray.svg?longCache=true&logo=github&colorB=purple)](https://github.com/coder/coder/discussions) [!["Join us on Slack"](https://img.shields.io/badge/join-us%20on%20slack-gray.svg?longCache=true&logo=slack&colorB=brightgreen)](https://coder.com/community) [![Twitter Follow](https://img.shields.io/twitter/follow/CoderHQ?label=%40CoderHQ&style=social)](https://twitter.com/coderhq) [![codecov](https://codecov.io/gh/coder/coder/branch/main/graph/badge.svg?token=TNLW3OAP6G)](https://codecov.io/gh/coder/coder)

Provision remote development environments with Terraform.

### Highlights

- Automate development environments for Linux, Windows, and MacOS in your cloud
- Start writing code with a single command
- Use one of many [examples](./examples) to get started

## Getting Started

Install [the latest release](https://github.com/coder/coder/releases).

To tinker, start with dev-mode (all data is in-memory, and is destroyed on exit):

```bash
$ coder start --dev
```

To run a production deployment with PostgreSQL:

```bash
$ CODER_PG_CONNECTION_URL="postgres://<username>@<host>/<database>?password=<password>" \
    coder start
```

To run as a system service, install with `.deb` or `.rpm`:

```bash
# Edit the configuration!
$ sudo vim /etc/coder.d/coder.env
$ sudo service coder restart
```

### Your First Workspace

In a new terminal, create a new project (eg. Develop in Linux on Google Cloud):

```
$ coder templates init
$ coder templates create
```

Create a new workspace and SSH in:

```
$ coder workspaces create my-first-workspace
$ coder ssh my-first-workspace
```

### Working with Projects

You can edit the Terraform from a sample project:

```sh
$ coder templates init
$ cd gcp-linux/
$ vim main.tf
$ coder templates update gcp-linux
```

## Contributing

Read the [contributing docs](./CONTRIBUTING.md).

