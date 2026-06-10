# Declarative Script Ordering

> [!NOTE]
> This feature is experimental and may change without notice in future releases.

The `coder_script_order` data source declares the execution order of
[`coder_script`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/script)
resources at the template level. Scripts referenced by a rule start only
after the scripts they are ordered `after` have finished, while every
other script keeps running in parallel. Script bodies need no
coordination logic, and registry modules can be ordered without
modifying them.

This is the recommended way to coordinate workspace startup. The
script-level [`coder exp sync`](./usage.md) commands remain available as
the legacy path for cases where ordering must be decided inside a
running script.

## Quick start

Declare a `coder_script_order` data source anywhere in your template and
add one `rule` block per ordering constraint:

```tf
resource "coder_script" "clone" {
  agent_id     = coder_agent.main.id
  display_name = "Clone repository"
  run_on_start = true
  script       = "git clone https://github.com/coder/coder ~/coder"
}

resource "coder_script" "install" {
  agent_id     = coder_agent.main.id
  display_name = "Install dependencies"
  run_on_start = true
  script       = "cd ~/coder && make install"
}

data "coder_script_order" "startup" {
  rule {
    run   = "coder_script.install"
    after = ["coder_script.clone"]
  }
}
```

Neither script contains any synchronization code. The agent starts
`install` once `clone` has succeeded.

## Selectors

`run` and `after` accept Terraform addresses:

| Selector                     | Matches                                                            |
|------------------------------|--------------------------------------------------------------------|
| `coder_script.<name>`        | A single script (all instances when it uses `count` or `for_each`) |
| `coder_script.<name>[<idx>]` | One instance of a script that uses `count` or `for_each`           |
| `module.<name>`              | Every script inside the module, including nested modules           |

Selectors are resolved relative to the module where the data source is
declared. A `coder_script_order` declared inside a module can only
reference scripts in that module's subtree, so modules can ship their
own internal ordering rules without conflicting with the template that
uses them.

Module selectors make registry modules orderable without changes:

```tf
module "git_clone" {
  source   = "registry.coder.com/coder/git-clone/coder"
  version  = "~> 1.0"
  agent_id = coder_agent.main.id
  url      = "https://github.com/coder/coder"
}

data "coder_script_order" "startup" {
  rule {
    run   = "coder_script.run_agent"
    after = ["module.git_clone"]
  }
}
```

## Rule semantics

Every script matched by `run` waits for every script matched by `after`.
The optional `requires` attribute controls what happens when a
dependency does not succeed:

| `requires`          | Behavior                                                                                                         |
|---------------------|------------------------------------------------------------------------------------------------------------------|
| `success` (default) | The dependent script is skipped when a dependency fails, times out, or was itself skipped. Skips are transitive. |
| `completion`        | The dependent script runs once its dependencies reach a terminal state, regardless of outcome.                   |

Skipped scripts log `Skipping script: dependency "<name>" failed` to
their workspace startup logs and report a `skipped` status to the build
timeline. See
[Observing ordering at runtime](#observing-ordering-at-runtime).

Additional semantics:

- Only `run_on_start` scripts can be ordered. Cron and stop scripts are
  unaffected.
- A script's `timeout` applies to its own run, not to the time spent
  waiting for dependencies. Set timeouts on dependencies as a backstop
  so a hung script cannot block its dependents forever.
- Multiple `rule` blocks and multiple `coder_script_order` data sources
  merge into a single dependency graph per agent.

### Multiple rules

Each `rule` block adds edges to the same dependency graph, so chains,
fan-out, and fan-in are declared with one rule per constraint:

```tf
data "coder_script_order" "startup" {
  # Chain: prep, then clone, then install.
  rule {
    run   = "coder_script.clone"
    after = ["coder_script.prep"]
  }

  rule {
    run   = "coder_script.install"
    after = ["coder_script.clone"]
  }

  # Fan-in: the IDE waits for two scripts that run in parallel.
  rule {
    run   = "coder_script.start_ide"
    after = ["coder_script.install", "coder_script.dotfiles"]
  }

  # The report runs once the launcher is terminal, even when an
  # earlier failure skipped the rest of the chain.
  rule {
    run      = "coder_script.report"
    after    = ["coder_script.start_ide"]
    requires = "completion"
  }
}
```

Scripts that no rule references keep starting immediately, in parallel
with the ordered ones. The same graph can also be split across several
`coder_script_order` data sources, for example one per module, and the
results merge per agent.

## Validation

Template builds fail with a descriptive error when:

- A selector matches no script. The error lists the scripts that are in
  scope.
- Rules form a cycle.
- A rule orders scripts that belong to different agents.
- A referenced script does not set `run_on_start = true`.
- A script is ordered after itself.
- Two rules declare the same dependency with different `requires`
  values.

A build warning (not an error) is emitted when an agent mixes
`coder_script_order` rules with scripts that call `coder exp sync`.
Prefer one coordination system per agent.

## Observing ordering at runtime

Ordering activity is visible in the workspace startup logs and in the
build timeline:

- While a script waits for its dependencies, it logs
  `Waiting for "<name>"... (30s)` to its own startup logs every 30
  seconds, naming the dependencies that have not finished yet.
- When a script is skipped, it logs
  `Skipping script: dependency "<name>" failed.` For transitive skips
  the reason is `dependency "<name>" was skipped.`
- Skipped scripts appear in the build timeline (workspace page >
  **Build timeline** > the startup scripts stage) with a `skipped`
  status and a zero-duration bar. Scripts that ran keep their usual
  `success`, `failure`, or `timed out` status.

A waiting script never gives up on its own. If a dependency hangs and
has no `timeout`, every script ordered after it waits indefinitely and
the heartbeat log lines are the only signal. Set a `timeout` on
dependencies as a backstop.

## Compatibility

Both the Coder deployment and the `coder/coder` Terraform provider must
be on releases that include this feature. Older agents and servers
ignore the ordering rules and run all startup scripts in parallel, which
matches the previous behavior.

## Example

See the
[script-ordering example template](https://github.com/coder/coder/tree/main/examples/script-ordering)
for a complete template that orders a registry module between two
template-level scripts and uses `requires = "completion"` for a final
reporting step.
