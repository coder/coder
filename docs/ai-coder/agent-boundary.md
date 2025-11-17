# Agent Boundary

Agent Boundaries are process-level firewalls that restrict and audit what autonomous programs, such as AI agents, can access and use.

![Screenshot of Agent Boundaries blocking a process](../images/guides/ai-agents/boundary.png)Example of Agent Boundaries blocking a process.

## Supported Agents

Agent Boundaries support the securing of any terminal-based agent, including your own custom agents.

## Features

Agent Boundaries offer network policy enforcement, which blocks domains and HTTP verbs to prevent exfiltration, and writes logs to the workspace.

## Getting Started with Boundary

The easiest way to use Agent Boundaries is through existing Coder modules, such as the [Claude Code module](https://registry.coder.com/modules/coder/claude-code). It can also be ran directly in the terminal by installing the [CLI](https://github.com/coder/boundary).

There are two supported ways to configure Boundary today:

1. **Inline module configuration** – fastest for quick testing.
2. **External `config.yaml`** – best when you need a large allow list or want everyone who launches Boundary manually to share the same config.

### Option 1: Inline module configuration (quick start)

Put every setting directly in the Terraform module when you just want to experiment:

```tf
module "claude-code" {
  source              = "dev.registry.coder.com/coder/claude-code/coder"
  version             = "4.1.0"
  enable_boundary     = true
  boundary_version    = "v0.2.0"
  boundary_log_dir    = "/tmp/boundary_logs"
  boundary_log_level  = "WARN"
  boundary_additional_allowed_urls = ["GET *google.com"]
  boundary_proxy_port = "8087"
}
```

All Boundary knobs live in Terraform, so you can iterate quickly without creating extra files.

### Option 2: Keep policy in `config.yaml` (extensive allow lists)

When you need to maintain a long allow list or share a detailed policy with teammates, keep Terraform minimal and move the rest into `config.yaml`:

```tf
module "claude-code" {
  source              = "dev.registry.coder.com/coder/claude-code/coder"
  version             = "4.1.0"
  enable_boundary     = true
  boundary_version    = "v0.2.0"
}
```

Then create a `config.yaml` file in your template directory with your policy:

```yaml
allowlist:
  - method=GET domain=google.com
log_dir: /tmp/boundary_logs
proxy_port: 8087
log_level: warn
```

Add a `coder_script` resource to mount the configuration file into the workspace filesystem:

```tf
resource "coder_script" "boundary_config_setup" {
  agent_id     = coder_agent.dev.id
  display_name = "Boundary Setup Configuration"
  run_on_start = true

  script = <<-EOF
    #!/bin/sh
    mkdir -p ~/.config/coder_boundary
    echo '${base64encode(file("${path.module}/config.yaml"))}' | base64 -d > ~/.config/coder_boundary/config.yaml
    chmod 600 ~/.config/coder_boundary/config.yaml
  EOF
}
```

Boundary automatically reads `config.yaml` from `~/.config/coder_boundary/` when it starts, so everyone who launches Boundary manually inside the workspace picks up the same configuration without extra flags. This is especially convenient for managing extensive allow lists in version control.

- `boundary_version` defines what version of Boundary is being applied. This is set to `main`, which points to the main branch of `coder/boundary`.
- `boundary_log_dir` is the directory where log files are written to when the workspace spins up.
- `boundary_log_level` defines the verbosity at which requests are logged. Boundary uses the following verbosity levels:
  - `WARN`: logs only requests that have been blocked by Boundary
  - `INFO`: logs all requests at a high level
  - `DEBUG`: logs all requests in detail
- `boundary_additional_allowed_urls`: defines the URLs that the agent can access, in additional to the default URLs required for the agent to work
  - `github.com` means only the specific domain is allowed
  - `*.github.com` means only the subdomains are allowed - the specific domain is excluded
  - `*github.com` means both the specific domain and all subdomains are allowed
  - You can also also filter on methods, hostnames, and paths - for example, `GET,HEAD *github.com/coder`.

You can also run Agent Boundaries directly in your workspace and configure it per template. You can do so by installing the [binary](https://github.com/coder/boundary) into the workspace image or at start-up. You can do so with the following command:

```hcl
curl -fsSL https://raw.githubusercontent.com/coder/boundary/main/install.sh | bash
 ```
