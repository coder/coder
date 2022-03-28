# Coder

[!["GitHub Discussions"](https://img.shields.io/badge/%20GitHub-%20Discussions-gray.svg?longCache=true&logo=github&colorB=purple)](https://github.com/coder/coder/discussions) [!["Join us on Slack"](https://img.shields.io/badge/join-us%20on%20slack-gray.svg?longCache=true&logo=slack&colorB=brightgreen)](https://coder.com/community) [![Twitter Follow](https://img.shields.io/twitter/follow/CoderHQ?label=%40CoderHQ&style=social)](https://twitter.com/coderhq) [![codecov](https://codecov.io/gh/coder/coder/branch/main/graph/badge.svg?token=TNLW3OAP6G)](https://codecov.io/gh/coder/coder)

Provision remote development environments with Terraform.

## Highlights

- Automate development environments for Linux, Windows, and MacOS in your cloud
- Start writing code with a single command
- Use one of many [examples](./examples) to get started

## Getting Started

Install [the latest release](https://github.com/coder/coder/releases).

To tinker, start with dev-mode (all data is in-memory):

```bash
$ coder start --dev
```

To run a production deployment with PostgreSQL:

```bash
$ CODER_PG_CONNECTION_URL="..." coder start
```

To run as a daemon, use the provided service (Linux only):

```bash
$ sudo vim /etc/coder.d/coder.env
$ sudo service coder restart
```

## Development

Code structure is inspired by [Basics of Unix Philosophy](https://homepage.cs.uri.edu/~thenry/resources/unix_art/ch01s06.html) and [Effective Go](https://go.dev/doc/effective_go); these should be read prior to contributing.

Requires Go 1.18+, Node 14+, and GNU Make.

- `make bin` build binaries
- `make install` install binaries to `$GOPATH/bin`
- `make test`
- `make release` dry-run a new release
- `./develop.sh` hot-reloads for frontend development
