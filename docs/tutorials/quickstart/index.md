# Quickstart

Not sure where to begin learning about Coder? This Quickstart guide is designed to get users started working in Coder and to make use of the advantages cloud development has to offer.

The Quickstart guide is split into the following sections:

- [**1: Launch your first workspace**](./launch-workspace.md) guides you through the process of installing the Coder CLI, launching the Coder server, creating a template that serves as a blueprint for your workspace, and spinning up a workspace from that template. In this workspace, you can edit Git repositories and get a basic set of tools for most of the popular programming languages. Most users should start here.
- [**2: Customize workspace startup**](./customize-workspace-startup.md) closes the gaps in the starter template, one change at a time. It includes four guides:
  - [Add a programming language](./add-a-language.md): expose a new language through a parameter and install it at startup.
  - [Install your own command-line tools](./install-command-line-tools.md): install personal command-line tools and make them persist.
  - [Personalize your workspace with dotfiles](./personalize-with-dotfiles.md): pull in a dotfiles repository with a registry module.
  - [Authenticate to GitHub](./authenticate-to-github.md): clone private repositories with an external-auth data source.

If you'd rather skip the tutorial and just install the CLI, then you can visit the [Install guide](/docs/install/index.md#localindividual-installs) for those instructions.

## A 30-second analogy for Coder

Before diving in, the following table breaks down the core concepts that power Coder,
explained through a cooking analogy:

| Component      | What It Is                                                                           | Real-World Analogy             |
|----------------|--------------------------------------------------------------------------------------|--------------------------------|
| **You**        | The engineer/developer/builder working                                               | The head chef cooking the meal |
| **Templates**  | A Terraform blueprint that defines your dev environment (OS, tools, resources)       | Recipe for a meal              |
| **Workspaces** | The actual running environment created from the template                             | The cooked meal                |
| **Users**      | A developer who launches the workspace from a template and does their work inside it | The people eating the meal     |

**Putting it Together:** Coder separates who _defines_ environments from who _uses_ them. Admins create and manage Templates, the recipes, while developers use those Templates to launch Workspaces, the meals.

## Prerequisites

- A machine with 2+ CPU cores and 4GB+ RAM
- Familiarity with running commands in the terminal
- 10-30 minutes of your time, depending on which guides you follow
