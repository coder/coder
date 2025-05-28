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
* [General Concepts](../admin/infrastructure/validated-architectures/index.md#general-concepts): Essential concepts and language used to describe how Coder is structured and operated.

* [Architecture](../admin/infrastructure/architecture.md): A high-level overview of the infrastructure layout, key services, and how components interact.

These sections provide the necessary context for navigating and contributing to the backend effectively.

## Tech Stack

Coder's backend is built using a collection of robust, modern Go libraries and internal packages. Familiarity with these technologies will help you navigate the codebase and contribute effectively.

### Core Libraries & Frameworks

* [go-chi/chi](https://github.com/go-chi/chi): lightweight HTTP router for building RESTful APIs in Go
* [golang-migrate/migrate](https://github.com/golang-migrate/migrate): manages database schema migrations across environments
* [coder/terraform-config-inspect](https://github.com/coder/terraform-config-inspect) *(forked)*: used for parsing and analyzing Terraform configurations, forked to include [PR #74](https://github.com/hashicorp/terraform-config-inspect/pull/74)
* [coder/pq](https://github.com/coder/pq) *(forked)*: PostgreSQL driver forked to support rotating authentication tokens via `driver.Connector`
* [coder/tailscale](https://github.com/coder/tailscale) *(forked)*: enables secure, peer-to-peer connectivity, forked to apply internal patches pending upstreaming
* [coder/wireguard-go](https://github.com/coder/wireguard-go) *(forked)*: WireGuard networking implementation, forked to fix a data race and adopt the latest gVisor changes
* [coder/ssh](https://github.com/coder/ssh) *(forked)*: customized SSH server based on `gliderlabs/ssh`, forked to include Tailscale-specific patches and avoid complex subpath dependencies
* [coder/bubbletea](https://github.com/coder/bubbletea) *(forked)*: terminal UI framework for CLI apps, forked to remove an `init()` function that interfered with web terminal output

### Coder libraries

* [coder/terraform-provider-coder](https://github.com/coder/terraform-provider-coder): official Terraform provider for managing Coder resources via infrastructure-as-code
* [coder/websocket](https://github.com/coder/websocket): minimal WebSocket library for real-time communication
* [coder/serpent](https://github.com/coder/serpent): CLI framework built on `cobra`, used for large, complex CLIs
* [coder/guts](https://github.com/coder/guts): generates TypeScript types from Go for shared type definitions
* [coder/wgtunnel](https://github.com/coder/wgtunnel): WireGuard tunnel server for secure backend networking

## Repository Structure

The Coder backend is organized into multiple packages and directories, each with a specific purpose. Here's a high-level overview of the most important ones:

* [agent](https://github.com/coder/coder/tree/main/agent): core logic of a workspace agent, supports DevContainers, remote SSH, startup/shutdown script execution. Protobuf definitions for DRPC communication with `coderd` are kept in [proto](https://github.com/coder/coder/tree/main/agent/proto).
* [cli](https://github.com/coder/coder/tree/main/cli): CLI interface for `coder` command built on [coder/serpent](https://github.com/coder/serpent). Input controls are defined in [cliui](https://github.com/coder/coder/tree/docs-backend-contrib-guide/cli/cliui), and [testdata](https://github.com/coder/coder/tree/docs-backend-contrib-guide/cli/testdata) contains golden files for common CLI calls
* [cmd](https://github.com/coder/coder/tree/main/cmd): entry points for CLI and services, including `coderd`
* [coderd](https://github.com/coder/coder/tree/main/coderd): the main API server implementation with [chi](https://github.com/go-chi/chi) endpoints
  * [audit](https://github.com/coder/coder/tree/main/coderd/audit): audit log logic, defines target resources, actions and extra fields
  * [autobuild](https://github.com/coder/coder/tree/main/coderd/autobuild): core logic of the workspace autobuild executor, periodically evaluates workspaces for next transition actions
  * [httpmw](https://github.com/coder/coder/tree/main/coderd/httpmw): HTTP middlewares mainly used to extract parameters from HTTP requests (e.g. current user, template, workspace, OAuth2 account, etc.) and storing them in the request context
  * [prebuilds](https://github.com/coder/coder/tree/main/coderd/prebuilds): common interfaces for prebuild workspaces, feature implementation is in [enterprise/prebuilds](https://github.com/coder/coder/tree/main/enterprise/coderd/prebuilds)
  * [provisionerdserver](https://github.com/coder/coder/tree/main/coderd/provisionerdserver): DRPC server for [provisionerd](https://github.com/coder/coder/tree/main/provisionerd) instances, used to validate and extract Terraform data and resources, and store them in the database.
  * [rbac](https://github.com/coder/coder/tree/main/coderd/rbac): RBAC engine for `coderd`, including authz layer, role definitions and custom roles. Built on top of [Open Policy Agent](https://github.com/open-policy-agent/opa) and Rego policies.
  * [telemetry](https://github.com/coder/coder/tree/main/coderd/telemetry): records a snapshot with various workspace data for telemetry purposes. Once recorded the reporter sends it to the configured telemetry endpoint.
  * [tracing](https://github.com/coder/coder/tree/main/coderd/tracing): extends telemetry with tracing data consistent with [OpenTelemetry specification](https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/exporter.md)
  * [workspaceapps](https://github.com/coder/coder/tree/main/coderd/workspaceapps): core logic of a secure proxy to expose workspace apps deployed in a workspace
  * [wsbuilder](https://github.com/coder/coder/tree/main/coderd/wsbuilder): wrapper for business logic of creating a workspace build. It encapsulates all database operations required to insert a build record in a transaction.
* [database](https://github.com/coder/coder/tree/main/coderd/database): schema migrations, query logic, in-memory database, etc.
  * [db2sdk](https://github.com/coder/coder/tree/main/coderd/database/db2sdk): translation between database structures and [codersdk](https://github.com/coder/coder/tree/main/coderd/codersdk) objects used by coderd API.
  * [dbauthz](https://github.com/coder/coder/tree/main/coderd/database/dbauthz): AuthZ wrappers for database queries, ideally, every query should verify first if the accessor is eligible to see the query results.
  * [dbfake](https://github.com/coder/coder/tree/main/coderd/database/dbfake): helper functions to quickly prepare the initial database state for testing purposes (e.g. create N healthy workspaces and templates), operates on higher level than [dbgen](https://github.com/coder/coder/tree/main/coderd/database/dbgen)
  * [dbgen](https://github.com/coder/coder/tree/main/coderd/database/dbgen): helper functions to insert raw records to the database store, used for testing purposes
  * [dbmem](https://github.com/coder/coder/tree/main/coderd/database/dbmem): in-memory implementation of the database store, ideally, every real query should have a complimentary Go implementation
  * [dbmock](https://github.com/coder/coder/tree/main/coderd/database/dbmock): a store wrapper for database queries, useful to verify if the function has been called, used for testing purposes
  * [dbpurge](https://github.com/coder/coder/tree/main/coderd/database/dbpurge): simple wrapper for periodic database cleanup operations
  * [migrations](https://github.com/coder/coder/tree/main/coderd/database/migrations): an ordered list of up/down database migrations, use `./create_migration.sh my_migration_name` to modify the database schema
  * [pubsub](https://github.com/coder/coder/tree/main/coderd/database/pubsub): PubSub implementation using PostgreSQL and in-memory drop-in replacement
  * [queries](https://github.com/coder/coder/tree/main/coderd/database/queries): contains SQL files with queries, `sqlc` compiles them to [Go functions](https://github.com/coder/coder/blob/docs-backend-contrib-guide/coderd/database/queries.sql.go)
  * [sqlc.yaml](https://github.com/coder/coder/tree/main/coderd/database/sqlc.yaml): defines mappings between SQL types and custom Go structures
* [dogfood](https://github.com/coder/coder/tree/main/dogfood): Terraform definition of the dogfood cluster deployment
* [enterprise](https://github.com/coder/coder/tree/main/enterprise): enterprise-only features, notice similar file structure to repository root (`audit`, `cli`, `cmd`, `coderd`, etc.)
  * [coderd](https://github.com/coder/coder/tree/main/enterprise/coderd)
    * [prebuilds](https://github.com/coder/coder/tree/main/enterprise/coderd/prebuilds): core logic of prebuilt workspaces - reconciliation loop
* `nix`: Nix utility scripts and definitions
* `provisioner`, `provisionerd`, `provisionersdk`: components for infrastructure provisioning
* `pty`: terminal emulation for remote shells
* `support`: shared internal helpers
* `tailnet`: network stack and identity management
* `vpn`: VPN and tunneling components
