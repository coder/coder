---
title: Order startup and shutdown scripts
---

As a template author, you can use the `coder_script_order` data source to declare dependencies between startup or shutdown scripts.
You can order individual `coder_script` resources or all scripts in a child module without adding coordination commands to script bodies.

Splitting workspace initialization into separate `coder_script` resources lets independent tasks run in parallel, which can shorten startup time.
Some scripts still depend on work performed by others.
For example, a script that installs application dependencies needs the repository to be cloned before it starts.
Without declared dependencies, these scripts can run at the same time, causing intermittent startup failures.

## Requirements

Script ordering requires compatible versions of every component:

- The [Coder Terraform provider](https://registry.terraform.io/providers/coder/coder/latest/docs) v2.20.0 or later.
- Coder control plane and provisioners v2.39.0 or later.
- Workspace agents, including dev container sub-agents, v2.39.0 or later.

Older components can ignore ordering declarations and run scripts concurrently without reporting a compatibility error.
Updating the provider alone doesn't enable ordering on older Coder deployments or agents.

## Declare dependencies

Add a `coder_script_order` data source to your template alongside your scripts.
For example, clone a Node.js repository before installing its dependencies:

```tf
resource "coder_script" "clone" {
  agent_id     = coder_agent.main.id
  display_name = "Clone Repository"
  run_on_start = true
  script       = "test -d ~/app/.git || git clone https://github.com/your-org/your-app.git ~/app"
}

resource "coder_script" "install" {
  agent_id     = coder_agent.main.id
  display_name = "Install Dependencies"
  run_on_start = true
  script       = "cd ~/app && npm ci"
}

data "coder_script_order" "startup" {
  rule {
    run   = ["coder_script.install"]
    after = ["coder_script.clone"]
  }
}
```

On startup, **Install Dependencies** waits for **Clone Repository** to succeed, then installs the packages.
The default requirement is `success`.
If cloning fails, Coder skips dependency installation.

Each `coder_script_order` data source contains one or more `rule` blocks:

| Attribute                              | Required | Meaning                                                                                          |
|----------------------------------------|----------|--------------------------------------------------------------------------------------------------|
| `run`                                  | Yes      | A nonempty list of selectors for scripts that wait.                                              |
| `after`                                | Yes      | A nonempty list of selectors for prerequisite scripts.                                           |
| `requires`                             | No       | `success` (default) or `completion`.                                                             |
| [`phase`](#choose-the-lifecycle-phase) | No       | `start` or `stop`. Required for module-only rules that select both startup and shutdown scripts. |

Every script selected by `run` waits for every script selected by `after`.
The order of selectors within either list doesn't determine execution order.
To declare a chain, use separate rules, such as configuration after preparation and tests after configuration.

Coder combines rules from all data sources in the template, including declarations inside modules.
Repeated dependencies with the same requirement are valid.
Declaring the same dependency with conflicting `requires` values is invalid.

A script can succeed before a background service it starts is ready.
If a prerequisite starts a background service, check that the service is ready before the script exits.
Redirect the background service's stdout and stderr, for example `my-server > /tmp/server.log 2>&1 &`.
Otherwise, Coder can treat the prerequisite as failed and skip its `requires = "success"` dependents.

Selectors are quoted [Terraform resource or module addresses](https://developer.hashicorp.com/terraform/cli/state/resource-addressing), not resource IDs or display names.
Selectors match only `coder_script` resources and modules, so you can't order the `startup_script` or `shutdown_script` attributes of `coder_agent`.
Coder must be able to determine all `coder_script_order` input values when it imports your template.
The data source doesn't need Terraform `depends_on`.
Terraform's [`depends_on`](https://developer.hashicorp.com/terraform/language/meta-arguments/depends_on) controls provisioning operations, not script execution inside a workspace.

## Select scripts and modules

Coder resolves selectors relative to the module containing the `coder_script_order` data source:

| Selector string               | Selects                                                                              |
|-------------------------------|--------------------------------------------------------------------------------------|
| `coder_script.install`        | All instances of a script resource in the declaring module.                          |
| `coder_script.install[0]`     | One `count` instance.                                                                |
| `coder_script.install["api"]` | One `for_each` instance.                                                             |
| `module.checkout`             | Scripts across all instances of a direct child module, including nested descendants. |

In Terraform strings, escape quotes around a `for_each` key: `"coder_script.install[\"api\"]"`.
An unindexed selector for a declared script or module can match no instances, such as with `count = 0` or an empty `for_each`.
That selector adds no dependencies.
An unknown resource or module name, or an indexed script instance that doesn't exist, is invalid.

A parent module can select an entire direct child module.
It can't select an individual script inside that module, a nested module path, or one indexed module instance.
For example, `module.checkout.coder_script.clone`, `module.checkout.module.tools`, and `module.checkout[0]` aren't supported selectors.

### Order scripts in modules

You can order modules without knowing their internal script names or changing their source.
For existing `checkout` and `application` module calls, add:

```tf
data "coder_script_order" "modules" {
  rule {
    phase = "start"
    run   = ["module.application"]
    after = ["module.checkout"]
  }
}
```

Every startup script in `application` waits for every startup script in `checkout` to succeed.
Scripts within each module remain concurrent unless other rules order them.
The selected startup scripts must share one workspace agent or dev container sub-agent.

### Wait for multiple prerequisites

You can combine module and script selectors in one rule.
For example, this template clones [Microblog](https://github.com/miguelgrinberg/microblog) and installs its system packages in parallel.
It then installs the application's dependencies after both succeed:

```tf
module "checkout" {
  source   = "registry.coder.com/coder/git-clone/coder"
  version  = "2.0.5"
  agent_id = coder_agent.main.id
  url      = "https://github.com/miguelgrinberg/microblog.git"
}

resource "coder_script" "system_packages" {
  agent_id     = coder_agent.main.id
  display_name = "Install System Packages"
  run_on_start = true
  script       = "sudo apt-get update && sudo apt-get install -y python3-dev python3-venv build-essential"
}

resource "coder_script" "python_dependencies" {
  agent_id     = coder_agent.main.id
  display_name = "Install Python Dependencies"
  run_on_start = true
  script       = <<-EOT
    set -e
    cd ${module.checkout.repo_dir}
    python3 -m venv .venv
    .venv/bin/pip install -r requirements.txt
  EOT
}

data "coder_script_order" "app_setup" {
  rule {
    phase = "start"
    run   = ["coder_script.python_dependencies"]
    after = ["module.checkout", "coder_script.system_packages"]
  }
}
```

The [`git-clone` module](https://registry.coder.com/modules/coder/git-clone) and **Install System Packages** can run concurrently.
**Install Python Dependencies** waits for both to succeed.
If either prerequisite fails, Coder skips **Install Python Dependencies**.

### Order scripts in a dev container

This example builds a Node.js project at `/workspaces/app` inside an existing `coder_devcontainer.app`:

```tf
resource "coder_script" "devcontainer_install" {
  agent_id     = coder_devcontainer.app.subagent_id
  display_name = "Install Dependencies"
  run_on_start = true
  script       = "cd /workspaces/app && npm ci"
}

resource "coder_script" "devcontainer_build" {
  agent_id     = coder_devcontainer.app.subagent_id
  display_name = "Build Application"
  run_on_start = true
  script       = "cd /workspaces/app && npm run build"
}

data "coder_script_order" "devcontainer_startup" {
  rule {
    run   = ["coder_script.devcontainer_build"]
    after = ["coder_script.devcontainer_install"]
  }
}
```

Both scripts run inside the same dev container because they use its `subagent_id` as their `agent_id`.
**Build Application** waits for **Install Dependencies** to succeed.
For setup instructions, refer to [Attach resources to dev containers](../../integrations/devcontainers/integration.md#attach-resources-to-dev-containers).

## Choose the lifecycle phase

Use `phase = "start"` for startup rules or `phase = "stop"` for shutdown rules.
Coder filters module selectors to scripts in that phase.
An explicit script selector must match the rule's phase and isn't filtered.

When you omit `phase`, Coder first infers it from the rule's explicit `coder_script` selectors.
If those selectors match no script instances, Coder infers it from the module selectors when all selected scripts share one phase.
A module-only rule containing both startup and shutdown scripts requires an explicit `phase`.
If all selectors are valid but match no script instances, the rule has no effect.

When an inferred phase filters out module scripts, Coder reports a warning in the provisioner logs.
Setting `phase` explicitly makes that filtering intentional and suppresses the warning.

Every script matched by a selector must set exactly one of `run_on_start` or `run_on_stop`.
For module selectors, this includes scripts that Coder excludes because they belong to the other phase.
If you need to order a script that runs at both startup and shutdown, split it into two `coder_script` resources, one per phase.
Cron-only scripts can't be ordered, and ordering doesn't apply to scheduled cron executions.

## Choose the required outcome

The `requires` attribute controls whether a prerequisite's result lets a dependent script run.
All prerequisites must satisfy their requirements before the dependent starts:

| Prerequisite result   | `requires = "success"` | `requires = "completion"` |
|-----------------------|------------------------|---------------------------|
| Succeeds              | Can run.               | Can run.                  |
| Fails                 | Skips.                 | Can run.                  |
| Times out             | Skips.                 | Can run.                  |
| Is skipped            | Skips.                 | Can run.                  |
| Execution is canceled | Doesn't run.           | Doesn't run.              |

Use `completion` for scripts that must run even if a prerequisite fails, times out, or is skipped, such as cleanup and reporting scripts.
For example, extend the [earlier startup example](#declare-dependencies) with a rule for an existing `coder_script.report` startup script:

```tf
data "coder_script_order" "report" {
  rule {
    run      = ["coder_script.report"]
    after    = ["coder_script.install"]
    requires = "completion"
  }
}
```

This allows `coder_script.report` to run after `coder_script.install` succeeds, fails, times out, or is skipped.

If startup or shutdown execution is canceled, Coder cancels running scripts and doesn't start any remaining scripts.
This applies to both `requires = "success"` and `requires = "completion"`.
In the example above, if startup execution is canceled while `coder_script.install` is running, Coder cancels it and doesn't run `coder_script.report`.

A script's execution timeout, set with the [`timeout`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/script#timeout-1) attribute, starts when its body begins running.
Time spent waiting for prerequisites doesn't consume that timeout.
There is no separate dependency wait timeout, and scripts have no execution timeout by default.
Set execution timeouts on prerequisites to bound how long dependents wait for scripts that don't finish.

## Order shutdown scripts

Shutdown rules use the same dependency direction and outcome requirements as startup rules.
Declare shutdown rules explicitly with `phase = "stop"`.
Startup dependencies don't automatically reverse during shutdown.

Coder resolves shutdown ordering when the workspace starts.
The running agent uses those rules when the workspace stops or is deleted.
To apply updated shutdown rules, start the workspace with the updated template version.

For example, export a development PostgreSQL database before compressing the dump:

```tf
resource "coder_script" "export_database" {
  agent_id     = coder_agent.main.id
  display_name = "Export Database"
  run_on_stop  = true
  timeout      = 30
  script       = "pg_dump app > /tmp/app.sql"
}

resource "coder_script" "compress_dump" {
  agent_id     = coder_agent.main.id
  display_name = "Compress Dump"
  run_on_stop  = true
  timeout      = 30
  script       = "gzip -f /tmp/app.sql"
}

data "coder_script_order" "shutdown" {
  rule {
    phase = "stop"
    run   = ["coder_script.compress_dump"]
    after = ["coder_script.export_database"]
  }
}
```

On shutdown, **Compress Dump** waits for **Export Database** to succeed before compressing `/tmp/app.sql`.

Configure the workspace runtime's termination grace period to exceed the longest shutdown chain.
Allow additional time for script processes to terminate and for the agent to shut down.
For example, add this attribute to the existing `docker_container` resource in a Docker workspace:

```tf
destroy_grace_seconds = 120
```

This gives the container up to 120&nbsp;seconds to stop, including script execution, process termination, and agent shutdown.
Other runtimes require an equivalent grace period, such as [`termination_grace_period_seconds`](https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs/resources/pod_v1#termination_grace_period_seconds-1) in the Kubernetes Terraform provider.
If the runtime terminates the agent before the chain finishes, the remaining scripts won't run.

## Logs and build timeline

While a script waits, its log reports all remaining prerequisites in one entry every 30&nbsp;seconds.
Each entry identifies prerequisites by their full Terraform resource addresses.
If Coder skips a script, its log names the prerequisites that failed, timed out, or were skipped.
These messages use each prerequisite's display name, or its Terraform resource address if no display name is set.
Cancellation logs distinguish scripts canceled before starting from scripts canceled while running.

The [**Build timeline**](../../../tutorials/best-practices/speed-up-templates.md#workspace-build-timeline) shows skipped and canceled startup scripts.
A script that never started has zero execution duration.
The agent logs include output from shutdown scripts.

## Validation

Coder validates script selectors and dependencies when you import a template or start a workspace.
Running `terraform validate` alone doesn't check script selectors or dependencies.

Template import rejects unknown or unsupported selectors, incompatible lifecycle phases, self-dependencies, cycles, and conflicting requirements.
Dependencies across agents or sub-agents don't fail template import, but they fail the workspace start build after Terraform apply.

Ordering supports scripts within one workspace agent or one dev container sub-agent.
Dependencies between agents, between an agent and its sub-agent, or between different sub-agents aren't supported.

## Migrate from `coder exp sync`

For dependencies between whole lifecycle scripts, you can replace script-level sync coordination with `coder_script_order` rules.

Declarative ordering uses the outcome of the whole prerequisite script.
If a script calls `coder exp sync complete` before it finishes, split it into separate `coder_script` resources at that point before migrating.

Calling `coder exp sync complete` from an `EXIT` trap lets dependent scripts run even if the prerequisite script fails.
With `coder_script_order`, the default `requires = "success"` skips dependent scripts after a failure.
Use [`requires = "completion"`](#choose-the-required-outcome) if they need to run even after the prerequisite fails.

The default five-minute wait timeout of `coder exp sync start` doesn't carry over.
If you rely on that limit, set the [`timeout`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/script#timeout-1) attribute on prerequisite `coder_script` resources before migrating.

To migrate:

1. Map each sync unit to its `coder_script` resource or child module selector.
2. Add a rule with the waiting script in `run` and its prerequisites in `after`.
3. Choose `success` or `completion` to match the intended failure behavior.
4. Remove the corresponding sync calls and completion traps from those scripts.
5. Publish the updated template.
6. Restart a test workspace using the updated template version.
7. Verify the execution order and required outcomes in its [script logs](#logs-and-build-timeline).

The experimental [`coder exp sync` workflow](../startup-coordination/usage.md) remains available for coordination inside script bodies.
Its unit status commands don't inspect declarative script dependencies.
Use script logs and the **Build timeline** to observe declarative ordering.

## Learn more

- [`coder_script_order` data source reference](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/script_order)
- [`coder_script` resource reference](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/script)
