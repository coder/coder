# Backend

This guide is designed to support both Coder engineers and community contributors in understanding our backend systems and getting started with development.

Coder’s backend powers the core infrastructure behind workspace provisioning, access control, and the overall developer experience. As the backbone of our platform, it plays a critical role in enabling reliable and scalable remote development environments.

The purpose of this guide is to help you:

* Understand how the various backend components fit together.

* Navigate the codebase with confidence and adhere to established best practices.

* Contribute meaningful changes - whether you're fixing bugs, implementing features, or reviewing code.

By aligning on tools, workflows, and conventions, we reduce cognitive overhead, improve collaboration across teams, and accelerate our ability to deliver high-quality software.

Need help or have questions? Join the conversation on our [Discord server](https://discord.com/invite/coder) — we’re always happy to support contributors.

## Quickstart

To get up and running with Coder's backend, make sure you have **Docker installed and running** on your system. Once that's in place, follow these steps:

1. Clone the Coder repository and navigate into the project directory:

```sh
git clone https://github.com/coder/coder.git
cd coder
```

2. Run the development script to spin up the local environment:

```sh
./scripts/develop.sh
```

This will start two processes:
* http://localhost:3000 — the backend API server, used primarily for backend development.
* http://localhost:8080 — the Node.js frontend dev server, useful if you're also touching frontend code.

3. Verify Your Session

Confirm that you're logged in by running:

```sh
./scripts/coder-dev.sh list
```

This should return an empty list of workspaces. If you encounter an error, review the output from the [develop.sh](https://github.com/coder/coder/blob/main/scripts/develop.sh) script for issues.

4. Create a Quick Workspace

A template named docker-amd64 (or docker-arm64 on ARM systems) is created automatically. To spin up a workspace quickly, use:

```sh
./scripts/coder-dev.sh create my-workspace -t docker-amd64
```

## Platform Architecture

To understand how the backend fits into the broader system, we recommend reviewing the following resources:
* [General Concepts](../admin/infrastructure/validated-architectures.md#general-concepts): Essential concepts and language used to describe how Coder is structured and operated.

* [Architecture](../admin/infrastructure/architecture.md): A high-level overview of the infrastructure layout, key services, and how components interact.

These sections provide the necessary context for navigating and contributing to the backend effectively.

## Tech Stack
