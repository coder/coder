# Backend

This guide is designed to support both Coder engineers and community contributors in understanding our backend systems and getting started with development.

Coder’s backend powers the core infrastructure behind workspace provisioning, access control, and the overall developer experience. As the backbone of our platform, it plays a critical role in enabling reliable and scalable remote development environments.

The purpose of this guide is to help you:

* Understand how the various backend components fit together.
* Navigate the codebase with confidence and adhere to established best practices.
* Contribute meaningful changes - whether you're fixing bugs, implementing features, or reviewing code.

Need help or have questions? Join the conversation on our [Discord server](https://discord.com/invite/coder) — we’re always happy to support contributors.

## Platform Architecture

To understand how the backend fits into the broader system, we recommend reviewing the following resources:

* [General Concepts](../../admin/infrastructure/validated-architectures/index.md#general-concepts): Essential concepts and language used to describe how Coder is structured and operated.

* [Architecture](../../admin/infrastructure/architecture.md): A high-level overview of the infrastructure layout, key services, and how components interact.

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
  * [db2sdk](https://github.com/coder/coder/tree/main/coderd/database/db2sdk): translation between database structures and [codersdk](https://github.com/coder/coder/tree/main/codersdk) objects used by coderd API.
  * [dbauthz](https://github.com/coder/coder/tree/main/coderd/database/dbauthz): AuthZ wrappers for database queries, ideally, every query should verify first if the accessor is eligible to see the query results.
  * [dbfake](https://github.com/coder/coder/tree/main/coderd/database/dbfake): helper functions to quickly prepare the initial database state for testing purposes (e.g. create N healthy workspaces and templates), operates on higher level than [dbgen](https://github.com/coder/coder/tree/main/coderd/database/dbgen)
  * [dbgen](https://github.com/coder/coder/tree/main/coderd/database/dbgen): helper functions to insert raw records to the database store, used for testing purposes
  * [dbmock](https://github.com/coder/coder/tree/main/coderd/database/dbmock): a store wrapper for database queries, useful to verify if the function has been called, used for testing purposes
  * [dbpurge](https://github.com/coder/coder/tree/main/coderd/database/dbpurge): simple wrapper for periodic database cleanup operations
  * [migrations](https://github.com/coder/coder/tree/main/coderd/database/migrations): an ordered list of up/down database migrations, use `./create_migration.sh my_migration_name` to modify the database schema
  * [pubsub](https://github.com/coder/coder/tree/main/coderd/database/pubsub): PubSub implementation using PostgreSQL and in-memory drop-in replacement
  * [queries](https://github.com/coder/coder/tree/main/coderd/database/queries): contains SQL files with queries, `sqlc` compiles them to [Go functions](https://github.com/coder/coder/blob/docs-backend-contrib-guide/coderd/database/queries.sql.go)
  * [sqlc.yaml](https://github.com/coder/coder/tree/main/coderd/database/sqlc.yaml): defines mappings between SQL types and custom Go structures
* [codersdk](https://github.com/coder/coder/tree/main/codersdk): user-facing API entities used by CLI and site to communicate with `coderd` endpoints
* [dogfood](https://github.com/coder/coder/tree/main/dogfood): Terraform definition of the dogfood cluster deployment
* [enterprise](https://github.com/coder/coder/tree/main/enterprise): enterprise-only features, notice similar file structure to repository root (`audit`, `cli`, `cmd`, `coderd`, etc.)
  * [coderd](https://github.com/coder/coder/tree/main/enterprise/coderd)
    * [prebuilds](https://github.com/coder/coder/tree/main/enterprise/coderd/prebuilds): core logic of prebuilt workspaces - reconciliation loop
* [provisioner](https://github.com/coder/coder/tree/main/provisioner): supported implementation of provisioners, Terraform and "echo" (for testing purposes)
* [provisionerd](https://github.com/coder/coder/tree/main/provisionerd): core logic of provisioner runner to interact provisionerd server, depending on a job acquired it calls template import, dry run or a workspace build
* [pty](https://github.com/coder/coder/tree/main/pty): terminal emulation for agent shell
* [support](https://github.com/coder/coder/tree/main/support): compile a support bundle with diagnostics
* [tailnet](https://github.com/coder/coder/tree/main/tailnet): core logic of Tailnet controller to maintain DERP maps, coordinate connections with agents and peers
* [vpn](https://github.com/coder/coder/tree/main/vpn): Coder Desktop (VPN) and tunneling components

## Testing

The Coder backend includes a rich suite of unit and end-to-end tests. A variety of helper utilities are used throughout the codebase to make testing easier, more consistent, and closer to real behavior.

### [clitest](https://github.com/coder/coder/tree/main/cli/clitest)

* Spawns an in-memory `serpent.Command` instance for unit testing
* Configures an authorized `codersdk` client
* Once a `serpent.Invocation` is created, tests can execute commands as if invoked by a real user

### [ptytest](https://github.com/coder/coder/tree/main/pty/ptytest)

* `ptytest` attaches to a `serpent.Invocation` and simulates TTY input/output
* `pty` provides matchers and "write" operations for interacting with pseudo-terminals

### [coderdtest](https://github.com/coder/coder/tree/main/coderd/coderdtest)

* Provides shortcuts to spin up an in-memory `coderd` instance
* Can start an embedded provisioner daemon
* Supports multi-user testing via `CreateFirstUser` and `CreateAnotherUser`
* Includes "busy wait" helpers like `AwaitTemplateVersionJobCompleted`
* [oidctest](https://github.com/coder/coder/tree/main/coderd/coderdtest/oidctest) can start a fake OIDC provider

### [testutil](https://github.com/coder/coder/tree/main/testutil)

* General-purpose testing utilities, including:
  * [chan.go](https://github.com/coder/coder/blob/main/testutil/chan.go): helpers for sending/receiving objects from channels (`TrySend`, `RequireReceive`, etc.)
  * [duration.go](https://github.com/coder/coder/blob/main/testutil/duration.go): set timeouts for test execution
  * [eventually.go](https://github.com/coder/coder/blob/main/testutil/eventually.go): repeatedly poll for a condition using a ticker
  * [port.go](https://github.com/coder/coder/blob/main/testutil/port.go): select a free random port
  * [prometheus.go](https://github.com/coder/coder/blob/main/testutil/prometheus.go): validate Prometheus metrics with expected values
  * [pty.go](https://github.com/coder/coder/blob/main/testutil/pty.go): read output from a terminal until a condition is met

### [dbtestutil](https://github.com/coder/coder/tree/main/coderd/database/dbtestutil)

* Allows choosing between real and in-memory database backends for tests
* `WillUsePostgres` is useful for skipping tests in CI environments that don't run Postgres

### [quartz](https://github.com/coder/quartz/tree/main)

* Provides a mockable clock or ticker interface
* Allows manual time advancement
* Useful for testing time-sensitive or timeout-related logic

## Quiz

Try to find answers to these questions before jumping into implementation work — having a solid understanding of how Coder works will save you time and help you contribute effectively.

1. When you create a template, what does that do exactly?
2. When you create a workspace, what exactly happens?
3. How does the agent get the required information to run?
4. How are provisioner jobs run?

## Recipes

### Adding database migrations and fixtures

#### Database migrations

Database migrations are managed with
[`migrate`](https://github.com/golang-migrate/migrate).

To add new migrations, use the following command:

```shell
./coderd/database/migrations/create_migration.sh my name
/home/coder/src/coder/coderd/database/migrations/000070_my_name.up.sql
/home/coder/src/coder/coderd/database/migrations/000070_my_name.down.sql
```

Then write queries into the generated `.up.sql` and `.down.sql` files and commit
them into the repository. The down script should make a best-effort to retain as
much data as possible.

Run `make gen` to generate models.

#### Database fixtures (for testing migrations)

There are two types of fixtures that are used to test that migrations don't
break existing Coder deployments:

* Partial fixtures
  [`migrations/testdata/fixtures`](../../../coderd/database/migrations/testdata/fixtures)
* Full database dumps
  [`migrations/testdata/full_dumps`](../../../coderd/database/migrations/testdata/full_dumps)

Both types behave like database migrations (they also
[`migrate`](https://github.com/golang-migrate/migrate)). Their behavior mirrors
Coder migrations such that when migration number `000022` is applied, fixture
`000022` is applied afterwards.

Partial fixtures are used to conveniently add data to newly created tables so
that we can ensure that this data is migrated without issue.

Full database dumps are for testing the migration of fully-fledged Coder
deployments. These are usually done for a specific version of Coder and are
often fixed in time. A full database dump may be necessary when testing the
migration of multiple features or complex configurations.

To add a new partial fixture, run the following command:

```shell
./coderd/database/migrations/create_fixture.sh my fixture
/home/coder/src/coder/coderd/database/migrations/testdata/fixtures/000070_my_fixture.up.sql
```

Then add some queries to insert data and commit the file to the repo. See
[`000024_example.up.sql`](../../../coderd/database/migrations/testdata/fixtures/000024_example.up.sql)
for an example.

To create a full dump, run a fully fledged Coder deployment and use it to
generate data in the database. Then shut down the deployment and take a snapshot
of the database.

```shell
mkdir -p coderd/database/migrations/testdata/full_dumps/v0.12.2 && cd $_
pg_dump "postgres://coder@localhost:..." -a --inserts >000069_dump_v0.12.2.up.sql
```

Make sure sensitive data in the dump is desensitized, for instance names,
emails, OAuth tokens and other secrets. Then commit the dump to the project.

To find out what the latest migration for a version of Coder is, use the
following command:

```shell
git ls-files v0.12.2 -- coderd/database/migrations/*.up.sql
```

This helps in naming the dump (e.g. `000069` above).
