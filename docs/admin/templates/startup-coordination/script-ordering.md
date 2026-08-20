# Order workspace scripts declaratively

This guide is for template authors who need to order lifecycle-triggered `coder_script` resources.
Use [`coder exp sync`](./usage.md) instead when you need to coordinate units from inside a running script.

> [!IMPORTANT]
> Declarative script ordering isn't available in a released Coder or `coder/coder` Terraform provider version yet.
> The prototype requires unpublished builds of both components.

The `coder_script_order` data source adds dependencies to existing script executions.
It doesn't enable scripts, cause additional executions, or change cron executions.

## Declare a script dependency

Set the lifecycle trigger on each script, then add a rule that selects the dependent script with `run` and its prerequisite with `after`:

```tf
resource "coder_script" "clone_repo" {
  agent_id     = coder_agent.main.id
  display_name = "Clone repository"
  run_on_start = true
  script       = "git clone https://github.com/coder/coder ~/coder"
}

resource "coder_script" "install_tools" {
  agent_id     = coder_agent.main.id
  display_name = "Install tools"
  run_on_start = true
  script       = "cd ~/coder && make install"
}

data "coder_script_order" "startup" {
  rule {
    run   = ["coder_script.install_tools"]
    after = ["coder_script.clone_repo"]
  }
}
```

The agent starts `install_tools` after `clone_repo` succeeds.
Scripts that no rule references start concurrently as before.

Every script selected by `run` waits for every script selected by `after`.
The order of selectors within either list has no execution meaning.
Multiple scripts in `run` remain concurrent unless another rule orders them.

## Select scripts and modules

Selectors are relative to the module that declares the `coder_script_order` data source.
The following selectors are supported:

| Selector                     | Matches                                                                                          |
|------------------------------|--------------------------------------------------------------------------------------------------|
| `coder_script.<name>`        | Every instance of the named resource, including `count` and `for_each` instances                 |
| `coder_script.<name>[0]`     | One `count` instance                                                                             |
| `coder_script.<name>["key"]` | One `for_each` instance                                                                          |
| `module.<name>`              | Every `coder_script` in the child module and its nested descendants, across all module instances |

For example, select 2 instances from a `for_each` resource by their keys:

```tf
resource "coder_script" "setup" {
  for_each = {
    clone_repo    = "./clone-repo.sh"
    install_tools = "./install-tools.sh"
  }

  agent_id     = coder_agent.main.id
  run_on_start = true
  script       = each.value
}

data "coder_script_order" "startup" {
  rule {
    run   = ["coder_script.setup[\"install_tools\"]"]
    after = ["coder_script.setup[\"clone_repo\"]"]
  }
}
```

A module selector keeps the parent template independent of script names inside a module.
You can use different module selectors in both sides of a rule:

```tf
data "coder_script_order" "startup" {
  rule {
    run   = ["module.dependents"]
    after = ["module.prerequisites"]
  }
}
```

This rule expands each selector recursively.
Every selected script in `module.dependents` waits for every selected script in `module.prerequisites`.

A parent module can't select one specific script inside a child module.
Select the whole child module, or declare a `coder_script_order` data source inside the child module and use relative selectors there.

## Combine rules

Coder combines matching rules from every `coder_script_order` data source into one graph for each agent runtime and lifecycle phase.
Each script runs once per configured lifecycle event and waits for the union of its prerequisites.

For example, both rules contribute to the prerequisites of `start_ide`:

```tf
data "coder_script_order" "startup" {
  rule {
    run   = ["coder_script.start_ide"]
    after = ["coder_script.install_tools"]
  }

  rule {
    run   = ["coder_script.start_ide"]
    after = ["coder_script.configure_dotfiles"]
  }
}
```

The agent starts `start_ide` once both prerequisites satisfy their dependency conditions.
Coder deduplicates repeated selectors and identical dependency edges.
Template validation fails if the same edge has conflicting `requires` values or if the combined rules create a self-dependency or cycle.

## Choose the required outcome

The optional `requires` attribute controls which prerequisite outcomes allow the dependent script to run:

| Value               | Behavior                                                                                                                    |
|---------------------|-----------------------------------------------------------------------------------------------------------------------------|
| `success` (default) | Runs the dependent only when every prerequisite succeeds. A failed, timed-out, or skipped prerequisite skips the dependent. |
| `completion`        | Runs the dependent after every prerequisite reaches any terminal outcome: succeeded, failed, timed out, or skipped.         |

Use `completion` for cleanup, reporting, or recovery work that must run after an attempt regardless of its result:

```tf
rule {
  run      = ["coder_script.report_status"]
  after    = ["coder_script.install_tools"]
  requires = "completion"
}
```

A skipped script remains a terminal dependency-graph node.
A downstream `completion` dependency can proceed from it, while a downstream `success` dependency is also skipped.

If the workspace lifecycle execution is canceled while a script waits, the dependent script is canceled and doesn't start.

## Keep dependencies in one phase and agent runtime

Each rule can contain startup scripts or shutdown scripts, but not both.
Use separate rules to order scripts with `run_on_start = true` and scripts with `run_on_stop = true`.

A selected script must enable exactly 1 of those lifecycle attributes.
Template validation rejects selected cron-only scripts, scripts with neither attribute enabled, and scripts with both attributes enabled.

All scripts selected by a rule must belong to the same agent runtime.
Scripts attached to the same dev container subagent can depend on each other.
Dependencies between different agents, between different dev container subagents, or between a dev container subagent and its parent agent fail template validation.

The legacy `startup_script` and `shutdown_script` attributes on `coder_agent` don't have selectors.
Move their commands into `coder_script` resources before including them in ordering rules.

## Set prerequisite timeouts

A script's `timeout` starts after its dependencies are satisfied and applies only to its own command.
Dependency waiting has no internal deadline.

Set a timeout on each prerequisite that might hang, so its dependents eventually run or skip according to `requires`:

```tf
resource "coder_script" "clone_repo" {
  agent_id     = coder_agent.main.id
  run_on_start = true
  script       = "git clone https://github.com/coder/coder ~/coder"
  timeout      = 300
}
```

The `timeout` value is in seconds.
A value of `0`, the default, means that the script has no timeout.

## Observe ordering

After a script has waited for 30&nbsp;seconds, it writes one summary to its script log every 30&nbsp;seconds.
The summary lists all unmet dependencies, sorted by their full Terraform addresses for deterministic output:

```txt
Waiting for dependencies: "coder_script.clone_repo", "coder_script.prepare_workspace"... (30s)
```

Coder writes one combined summary instead of one log entry per dependency.
The sorted order can differ from the selector order in the template because selector list order has no execution meaning.

When a `success` dependency prevents a script from running, the dependent logs one of these reasons:

```txt
Skipping script: dependency "Clone repository" failed.
Skipping script: dependency "Install tools" was skipped.
```

The build timeline shows a skipped script with the `skipped` status and a zero-duration bar.

## Migrate from `coder exp sync`

Use declarative ordering when each coordinated unit maps to a lifecycle-triggered `coder_script` resource:

1. Move each unit into its own `coder_script` resource if needed.
2. Translate each `coder exp sync want` relationship into a `coder_script_order` rule.
3. Choose `success` or `completion` based on whether failure should skip the dependent.
4. Remove the matching `coder exp sync want`, `start`, and `complete` calls from the script bodies.

Keep `coder exp sync` when a script coordinates smaller internal phases or discovers dependencies while it runs.
The command remains available and doesn't require migration for templates that need those behaviors.

## Check compatibility

No released Coder or `coder/coder` Terraform provider version supports declarative script ordering yet.
The prototype requires unpublished builds of both components.

An older provider rejects `coder_script_order` as an unknown data source.
An older Coder deployment paired with the prototype provider can ignore the evaluated data source and run selected scripts concurrently.
Use a compatible Coder and provider pair before relying on ordering for correctness.
