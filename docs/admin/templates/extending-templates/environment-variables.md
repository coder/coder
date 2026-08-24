# Environment variables

Use the
[`coder_env`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/env)
resource to inject environment variables into your workspace agents. This is
useful for configuring tools, setting paths, and passing configuration to
development environments.

## Basic usage

```tf
resource "coder_agent" "dev" {
  os   = "linux"
  arch = "amd64"
}

resource "coder_env" "go_path" {
  agent_id = coder_agent.dev.id
  name     = "GOPATH"
  value    = "/home/coder/go"
}
```

Each `coder_env` resource sets a single environment variable on the specified
agent. You can define multiple `coder_env` resources targeting the same agent.

## Merge strategies

When multiple `coder_env` resources define the same variable name, use the
`merge_strategy` attribute to control how values are combined:

| Strategy              | Behavior                                            |
|-----------------------|-----------------------------------------------------|
| `replace` _(default)_ | Last value wins. Backward compatible.               |
| `append`              | Appends to the existing value with `:` separator.   |
| `prepend`             | Prepends to the existing value with `:` separator.  |
| `error`               | Fails the build if the variable is already defined. |

The `append` and `prepend` strategies use `:` as a separator, which matches
the convention for `PATH`-style variables on Unix systems.

### Merges and the host environment

`merge_strategy` only combines values across `coder_env` resources in the
same template. It does not know about the value already set by the
workspace's image or host, such as the base `PATH`. An `append` or `prepend`
builds its value from `coder_env` resources alone, and that value replaces
whatever the host had set for that variable.

To extend a variable that the host already sets, reference it directly in
one of the values (for example, `$PATH`), so the agent expands it against
the real environment at startup. Only one resource in the chain needs the
reference; the rest can use plain values. See the next example.

### Example: Appending to PATH

Multiple `coder_env` resources can each add directories to `PATH`. Reference
`$PATH` in one of them to keep the host's original directories:

```tf
resource "coder_agent" "dev" {
  os   = "linux"
  arch = "amd64"
}

resource "coder_env" "path_cuda" {
  agent_id       = coder_agent.dev.id
  name           = "PATH"
  value          = "$PATH:/usr/local/cuda/bin"
  merge_strategy = "append"
}

resource "coder_env" "path_go" {
  agent_id       = coder_agent.dev.id
  name           = "PATH"
  value          = "/usr/local/go/bin"
  merge_strategy = "append"
}

resource "coder_env" "path_rust_cargo" {
  agent_id       = coder_agent.dev.id
  name           = "PATH"
  value          = "/usr/local/cargo/bin"
  merge_strategy = "append"
}
```

This produces `PATH` with the host's original value, followed by
`/usr/local/cuda/bin:/usr/local/go/bin:/usr/local/cargo/bin`.

### Example: Preventing duplicates

Use `error` to catch accidental duplicate definitions:

```tf
resource "coder_env" "editor" {
  agent_id       = coder_agent.dev.id
  name           = "EDITOR"
  value          = "vim"
  merge_strategy = "error"
}
```

If another `coder_env` resource also sets `EDITOR`, the build fails with
a clear error message.

## Ordering

When multiple `coder_env` resources append or prepend to the same variable,
they are processed in alphabetical order by their
[Terraform resource address](https://developer.hashicorp.com/terraform/cli/state/resource-addressing).
In the previous `PATH` example, `coder_env.path_cuda` is processed first, then `coder_env.path_go`, then `coder_env.path_rust_cargo`, because that is their alphabetical order.
This is why only one resource in the chain needs the `$PATH` reference: the merged value is built as one string before it's expanded, so the reference works no matter where in the chain it appears.

## Agent env override

The `env` block inside a `coder_agent` resource always takes final precedence
over any `coder_env` resources. If both define the same variable, the
`coder_agent` value wins regardless of `merge_strategy`. This override happens
after `coder_env` resources are merged, so `merge_strategy = "error"` does not
trigger when the conflict is with the agent's `env` block — only when two
`coder_env` resources define the same key:

```tf
resource "coder_agent" "dev" {
  os   = "linux"
  arch = "amd64"
  env = {
    PATH = "/usr/local/bin:/usr/bin:/bin"
  }
}

# This value is ignored because coder_agent.dev.env sets PATH directly.
resource "coder_env" "extra_path" {
  agent_id       = coder_agent.dev.id
  name           = "PATH"
  value          = "/home/coder/bin"
  merge_strategy = "append"
}
```

See the
[Coder Terraform provider documentation](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/env)
for the complete `coder_env` reference.
