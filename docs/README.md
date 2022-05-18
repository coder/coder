# Coder

[!["GitHub
Discussions"](https://img.shields.io/badge/%20GitHub-%20Discussions-gray.svg?longCache=true&logo=github&colorB=purple)](https://github.com/coder/coder/discussions)
[!["Join us on
Discord"](https://img.shields.io/badge/join-us%20on%20Discord-gray.svg?longCache=true&logo=discord&colorB=purple)](https://discord.gg/coder)
[![Twitter
Follow](https://img.shields.io/twitter/follow/CoderHQ?label=%40CoderHQ&style=social)](https://twitter.com/coderhq)
[![codecov](https://codecov.io/gh/coder/coder/branch/main/graph/badge.svg?token=TNLW3OAP6G)](https://codecov.io/gh/coder/coder)

Provision remote development environments with Terraform.

![Kubernetes workspace in Coder v2](./screenshot.png)

## Highlights

- Automate development environments for Linux, Windows, and macOS
- Start writing code with a single command
- Get started quickly using one of the [examples](../examples) provided

## Installing Coder

Install [the latest release](https://github.com/coder/coder/releases) on a system with
at least 1 CPU core and 2 GB RAM.

To test, start with dev mode (all data is in-memory and is destroyed on exit):

```bash
coder server --dev
```

To run a production deployment with PostgreSQL:

```bash
CODER_PG_CONNECTION_URL="postgres://<username>@<host>/<database>?password=<password>" \
    coder server
```

To run as a system service, install with `.deb` (Debian, Ubuntu) or `.rpm`
(Fedora, CentOS, RHEL, SUSE):

```bash
# Edit the configuration!
sudo vim /etc/coder.d/coder.env
sudo service coder restart
```

Use `coder start --help` to get a complete list of flags and environment
variables.

### Your first workspace

In a new terminal, create a template (e.g., a template to **Develop in Linux on
Google Cloud**):

```bash
coder templates init
coder templates create
```

Create a workspace and connect to it via SSH:

```bash
coder create my-first-workspace
coder ssh my-first-workspace
```

### Modifying templates

You can edit the Terraform template using a sample template:

```sh
coder templates init
cd gcp-linux/
vim main.tf
coder templates update gcp-linux
```

## Documentation

- [About Coder](./about.md#about-coder)
  - [Why remote development](about.md#why-remote-development)
  - [Why Coder](about.md#why-coder)
  - [What Coder is not](about.md#what-coder-is-not)
  - [Comparison: Coder vs. [product]](about.md#comparison)
- [Templates](./templates.md)
  - [Manage templates](./templates.md#manage-templates)
  - [Persistent and ephemeral
    resources](./templates.md#persistent-and-ephemeral-resources)
  - [Parameters](./templates.md#parameters)
- [Workspaces](./workspaces.md)
  - [Create workspaces](./workspaces.md#create-workspaces)
  - [Connect with SSH](./workspaces.md#connect-with-ssh)
  - [Editors and IDEs](./workspaces.md#editors-and-ides)
  - [Workspace lifecycle](./workspaces.md#workspace-lifecycle)
  - [Updating workspaces](./workspaces.md#updating-workspaces)

## Contributing

Read the [contributing docs](./CONTRIBUTING.md).

## Contributors

<!--- Add your row by date (mm/dd/yyyy), most recent date at end of list --->

| Name                | Start Date | First PR Date |           Organization            |                              GitHub User Link |
| ------------------- | :--------: | :-----------: | :-------------------------------: | --------------------------------------------: |
| Grey Barkans        | 01/13/2020 |  03/13/2022   | [Coder](https://github.com/coder) |   [vapurrmaid](https://github.com/vapurrmaid) |
| Ben Potter          | 08/10/2020 |  03/31/2022   | [Coder](https://github.com/coder) |             [bpmct](https://github.com/bpmct) |
| Mathias Fredriksson | 04/25/2022 |  04/25/2022   | [Coder](https://github.com/coder) |       [mafredri](https://github.com/mafredri) |
| Spike Curtis        | 05/02/2022 |  05/06/2022   | [Coder](https://github.com/coder) | [spikecurtis](https://github.com/spikecurtis) |
| Kira Pilot          | 05/09/2022 |  05/09/2022   | [Coder](https://github.com/coder) |   [Kira-Pilot](https://github.com/Kira-Pilot) |
| David Wahler        | 05/09/2022 |  04/05/2022   | [Coder](https://github.com/coder) |         [dwahler](https://github.com/dwahler) |
